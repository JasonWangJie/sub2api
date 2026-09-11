package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type OpenAI429Phase string

const (
	OpenAI429Normal        OpenAI429Phase = "normal"
	OpenAI429Concentrating OpenAI429Phase = "concentrating"
	OpenAI429Draining      OpenAI429Phase = "draining"
	OpenAI429Probing       OpenAI429Phase = "probing"
	OpenAI429WaitingReal   OpenAI429Phase = "waiting_real"
	OpenAI429RealInFlight  OpenAI429Phase = "real_inflight"
	OpenAI429Stopped       OpenAI429Phase = "stopped"
)

type openAI429Flight struct {
	ExpiresAt int64 `json:"expires_at"`
	RealSlot  bool  `json:"real_slot,omitempty"`
	Probe     bool  `json:"probe,omitempty"`
}

// Only the safe projection of this structure is persisted in account.extra.
// Request payloads, credentials, headers and user identifiers never enter Redis.
type OpenAI429State struct {
	Generation        string                     `json:"generation"`
	Version           int64                      `json:"version"`
	Phase             OpenAI429Phase             `json:"phase"`
	Consecutive429    int                        `json:"consecutive_429"`
	Active            bool                       `json:"active"`
	RecoverAt         int64                      `json:"recover_at,omitempty"`
	UnknownReset      bool                       `json:"unknown_reset,omitempty"`
	ExhaustedWindows  uint8                      `json:"exhausted_windows,omitempty"`
	CycleExpiresAt    int64                      `json:"cycle_expires_at,omitempty"`
	NextQuotaQueryAt  int64                      `json:"next_quota_query_at,omitempty"`
	Model             string                     `json:"model,omitempty"`
	ProbeOwner        string                     `json:"probe_owner,omitempty"`
	ProbeLeaseUntil   int64                      `json:"probe_lease_until,omitempty"`
	RoundStartedAt    int64                      `json:"round_started_at,omitempty"`
	RoundCalls        int                        `json:"round_calls"`
	ProbeTotal        int64                      `json:"probe_total"`
	ProbeDurationMS   int64                      `json:"probe_duration_ms"`
	ProbeInputTokens  int64                      `json:"probe_input_tokens"`
	ProbeOutputTokens int64                      `json:"probe_output_tokens"`
	LastProbeResult   string                     `json:"last_probe_result,omitempty"`
	Flights           map[string]openAI429Flight `json:"flights,omitempty"`
}

type openAI429Outcome struct {
	BudgetExpired    bool
	Ignore           bool
	Success          bool
	RateLimited      bool
	QuotaExhausted   bool
	ResetAt          int64
	UnknownReset     bool
	ExhaustedWindows uint8
	Duration         time.Duration
	Usage            OpenAIUsage
}

var errOpenAI429Admission = errors.New("OpenAI 429 mode: account temporarily unavailable")

func (s *OpenAI429State) expire(now int64) {
	interrupted := false
	for id, flight := range s.Flights {
		if flight.ExpiresAt <= now {
			delete(s.Flights, id)
			interrupted = true
		}
	}
	if s.ProbeOwner != "" && s.ProbeLeaseUntil <= now {
		s.ProbeOwner = ""
		interrupted = true
	}
	if interrupted && s.Active && s.Phase != OpenAI429Stopped {
		s.Consecutive429 = 0
		s.Phase = OpenAI429WaitingReal
	}
	if s.Phase != OpenAI429Stopped && s.CycleExpiresAt > 0 && now >= s.CycleExpiresAt && len(s.Flights) == 0 {
		s.reset()
	}
}

func (s *OpenAI429State) reset() {
	s.Phase = OpenAI429Normal
	s.Active = false
	s.Consecutive429 = 0
	s.RecoverAt, s.CycleExpiresAt, s.NextQuotaQueryAt = 0, 0, 0
	s.UnknownReset = false
	s.ExhaustedWindows = 0
	s.ProbeOwner, s.ProbeLeaseUntil = "", 0
	s.RoundCalls, s.RoundStartedAt = 0, 0
	s.Flights = nil
}

func (s *OpenAI429State) observeQuota(a *Account, now time.Time) {
	used, exhausted, reset, unknown := openAI429Quota(a, now)
	if exhausted && !s.Active {
		s.Active, s.Phase = true, OpenAI429Draining
	}
	if exhausted {
		for i, window := range []string{"5h", "7d"} {
			if reset, err := parseTime(fmt.Sprint(a.Extra["codex_"+window+"_reset_at"])); err == nil && !reset.After(now) {
				continue
			}
			if percent, ok := resolveAccountExtraNumber(a.Extra, "codex_"+window+"_used_percent"); ok && percent >= 100 {
				s.ExhaustedWindows |= 1 << i
			}
		}
		if reset > s.RecoverAt {
			s.RecoverAt = reset
		}
		firstUnknown := unknown && !s.UnknownReset
		s.UnknownReset = s.UnknownReset || unknown
		if reset > s.CycleExpiresAt {
			s.CycleExpiresAt = reset
		}
		if firstUnknown || (unknown && s.CycleExpiresAt == 0) {
			s.CycleExpiresAt = now.Add(7 * 24 * time.Hour).UnixMilli()
		}
	}
	if !s.Active {
		s.Phase = OpenAI429Normal
		if used >= 90 {
			s.Phase = OpenAI429Concentrating
		}
	}
	if s.Active && s.Model == "" && len(s.Flights) == 0 && s.Phase == OpenAI429Draining {
		s.Phase = OpenAI429WaitingReal
	}
}

func (s *OpenAI429State) admit(id, model string, now int64) bool {
	s.expire(now)
	if s.Flights == nil {
		s.Flights = make(map[string]openAI429Flight)
	}
	flight := openAI429Flight{ExpiresAt: now + int64((2*time.Minute)/time.Millisecond)}
	switch s.Phase {
	case OpenAI429Normal, OpenAI429Concentrating:
	case OpenAI429WaitingReal:
		if len(s.Flights) != 0 || s.ProbeOwner != "" {
			return false
		}
		flight.RealSlot = true
		s.Phase = OpenAI429RealInFlight
	default:
		return false
	}
	if model != "" {
		s.Model = model
	}
	s.Flights[id] = flight
	return true
}

func (s *OpenAI429State) finish(id string, outcome openAI429Outcome, now int64) bool {
	s.expire(now)
	flight, exists := s.Flights[id]
	if !exists {
		return false
	} // Duplicate or fenced late response.
	delete(s.Flights, id)
	if outcome.Ignore {
		if flight.RealSlot || flight.Probe {
			s.Phase = OpenAI429WaitingReal
			s.ProbeOwner = ""
		}
		if s.Phase == OpenAI429Draining && len(s.Flights) == 0 && s.Model == "" {
			s.Phase = OpenAI429WaitingReal
		}
		return true
	}
	if outcome.BudgetExpired {
		if flight.Probe {
			s.ProbeTotal++
			s.ProbeDurationMS += outcome.Duration.Milliseconds()
			s.LastProbeResult = "budget_exhausted"
		}
		s.Phase, s.ProbeOwner, s.ProbeLeaseUntil = OpenAI429WaitingReal, "", 0
		return true
	}
	if flight.Probe {
		s.ProbeTotal++
		s.ProbeDurationMS += outcome.Duration.Milliseconds()
		s.ProbeInputTokens += int64(outcome.Usage.InputTokens)
		s.ProbeOutputTokens += int64(outcome.Usage.OutputTokens)
		s.LastProbeResult = "interrupted"
		if outcome.RateLimited {
			s.LastProbeResult = "429"
		} else if outcome.Success {
			s.LastProbeResult = "success"
		}
	}
	if s.Phase == OpenAI429Stopped {
		return true
	}
	if outcome.QuotaExhausted {
		s.Active = true
		s.ExhaustedWindows |= outcome.ExhaustedWindows
		if outcome.ResetAt > s.RecoverAt {
			s.RecoverAt = outcome.ResetAt
		}
		firstUnknown := !s.UnknownReset && (outcome.UnknownReset || s.RecoverAt == 0)
		s.UnknownReset = s.UnknownReset || outcome.UnknownReset || s.RecoverAt == 0
		if s.RecoverAt > s.CycleExpiresAt {
			s.CycleExpiresAt = s.RecoverAt
		}
		if firstUnknown || s.CycleExpiresAt == 0 {
			s.CycleExpiresAt = now + int64(7*24*time.Hour/time.Millisecond)
		}
	}
	if !s.Active {
		return true
	}
	if outcome.RateLimited {
		s.Consecutive429++
	} else {
		s.Consecutive429 = 0
	}
	if s.Consecutive429 >= 10 && !flight.Probe && len(s.Flights) == 0 {
		s.Phase = OpenAI429Stopped
		s.ProbeOwner, s.ProbeLeaseUntil = "", 0
		s.NextQuotaQueryAt = now + int64(5*time.Minute/time.Millisecond)
		return true
	}
	if !outcome.Success && !outcome.RateLimited {
		s.Phase = OpenAI429WaitingReal
		s.ProbeOwner, s.ProbeLeaseUntil = "", 0
		return true
	}
	if flight.RealSlot {
		s.RoundCalls, s.RoundStartedAt = 0, 0
	}
	if len(s.Flights) > 0 {
		s.Phase = OpenAI429Draining
		return true
	}
	if s.Consecutive429 >= 9 {
		s.Phase = OpenAI429WaitingReal
		s.ProbeOwner, s.ProbeLeaseUntil = "", 0
	} else if flight.Probe {
		s.Phase = OpenAI429Probing
	} else {
		s.Phase = OpenAI429Draining
	}
	return true
}

func (s *OpenAI429State) claimProbe(owner string, now int64) bool {
	s.expire(now)
	if !s.Active || s.Model == "" || len(s.Flights) > 0 || s.Phase != OpenAI429Draining || s.ProbeOwner != "" {
		return false
	}
	if s.Consecutive429 >= 9 {
		s.Phase = OpenAI429WaitingReal
		return false
	}
	s.Phase, s.ProbeOwner = OpenAI429Probing, owner
	s.ProbeLeaseUntil = now + int64(40*time.Second/time.Millisecond)
	s.RoundStartedAt, s.RoundCalls = now, 0
	return true
}

func (s *OpenAI429State) nextProbe(owner, id string, now int64) bool {
	if s.ProbeOwner != owner || s.ProbeLeaseUntil <= now || s.Phase != OpenAI429Probing || len(s.Flights) != 0 {
		return false
	}
	if s.Consecutive429 >= 9 || s.RoundCalls >= 30 || now-s.RoundStartedAt >= int64(30*time.Second/time.Millisecond) {
		s.Phase, s.ProbeOwner, s.ProbeLeaseUntil = OpenAI429WaitingReal, "", 0
		return false
	}
	s.RoundCalls++
	if s.Flights == nil {
		s.Flights = make(map[string]openAI429Flight)
	}
	s.Flights[id] = openAI429Flight{ExpiresAt: s.ProbeLeaseUntil, Probe: true}
	return true
}

type OpenAI429AtomicCache interface {
	Get(context.Context, string) (string, error)
	CompareAndSwap(context.Context, string, string, string) (bool, error)
}

type openAI429StateStore struct{ cache OpenAI429AtomicCache }

func (s *openAI429StateStore) mutate(ctx context.Context, a *Account, fn func(*OpenAI429State)) (*OpenAI429State, error) {
	if s == nil || s.cache == nil {
		return nil, errors.New("OpenAI 429 mode requires Redis")
	}
	key := fmt.Sprintf("openai:429-mode:{%d}", a.ID)
	for attempt := 0; attempt < 64; attempt++ {
		raw, err := s.cache.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		state := OpenAI429State{}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &state); err != nil {
				return nil, err
			}
		}
		generation := a.GetExtraString(OpenAI429ModeGenerationKey)
		if state.Generation > generation {
			return nil, errOpenAI429Admission
		}
		if raw == "" || state.Generation != generation {
			state = OpenAI429State{Generation: generation, Phase: OpenAI429Normal}
			if snapshot, ok := a.Extra[OpenAI429ModeStateKey]; ok {
				data, _ := json.Marshal(snapshot)
				var saved OpenAI429State
				if json.Unmarshal(data, &saved) == nil && saved.Generation == generation {
					state = saved
					// A missing Redis key cannot resurrect an in-flight permission
					// from the durable projection, which deliberately has no tickets.
					if state.Phase == OpenAI429RealInFlight || state.Phase == OpenAI429Probing || state.Phase == OpenAI429Draining {
						state.Consecutive429 = 0
						state.Phase = OpenAI429WaitingReal
						state.ProbeOwner, state.ProbeLeaseUntil = "", 0
					}
				}
			}
		}
		fn(&state)
		state.Version++
		encoded, err := json.Marshal(&state)
		if err != nil {
			return nil, err
		}
		changed, err := s.cache.CompareAndSwap(ctx, key, raw, string(encoded))
		if err != nil {
			return nil, err
		}
		if changed {
			return &state, nil
		}
	}
	return nil, errors.New("OpenAI 429 mode state contention")
}
