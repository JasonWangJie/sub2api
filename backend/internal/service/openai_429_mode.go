package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

type OpenAI429ModeRepository interface {
	StoreOpenAI429ModeState(context.Context, int64, *OpenAI429State) error
}

func (s *OpenAI429ModeService) reconfigure(ctx context.Context, id int64) {
	if s == nil {
		return
	}
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return
	}
	s.setConfiguredEnabled(id, OpenAI429ModeEnabled(a))
	if !SupportsOpenAI429Mode(a) {
		return
	}
	if !OpenAI429ModeEnabled(a) {
		_, _ = s.store.mutate(ctx, a, func(st *OpenAI429State) { st.reset() })
		return
	}
	s.takeOverQuotaPause(ctx, a)
	s.notify(id)
}

func (s *OpenAI429ModeService) takeOverQuotaPause(ctx context.Context, a *Account) {
	clearTemp, clearRate := false, false
	if reason, ok := parseTempUnschedReasonPayload(a.TempUnschedulableReason); ok && reason.Source == AccountSchedulingThresholdReasonSource && reason.Platform == PlatformOpenAI && (reason.Window == "5h" || reason.Window == "7d") {
		clearTemp = true
	}
	_, exhausted, reset, unknown := openAI429Quota(a, time.Now())
	if exhausted && !unknown && reset > 0 && a.RateLimitResetAt != nil && a.RateLimitResetAt.Unix() == time.UnixMilli(reset).Unix() {
		clearRate = true
	}
	if !clearTemp && !clearRate {
		return
	}
	if repo, ok := s.accounts.(interface {
		TakeOverOpenAI429QuotaPause(context.Context, *Account, bool, bool) (bool, error)
	}); ok {
		changed, err := repo.TakeOverOpenAI429QuotaPause(ctx, a, clearTemp, clearRate)
		if err == nil && changed && s.gateway != nil {
			if clearTemp && s.gateway.rateLimitService != nil && s.gateway.rateLimitService.tempUnschedCache != nil {
				if cache, ok := s.gateway.rateLimitService.tempUnschedCache.(interface {
					DeleteTempUnschedIfUnchanged(context.Context, int64, *TempUnschedState) error
				}); ok {
					if a.TempUnschedulableUntil != nil {
						expected := tempUnschedStateFromStoredReason(a.TempUnschedulableReason, a.TempUnschedulableUntil.Unix())
						_ = cache.DeleteTempUnschedIfUnchanged(ctx, a.ID, expected)
					}
				}
			}
		}
	}
}

type OpenAI429ModeService struct {
	store      *openAI429StateStore
	accounts   AccountRepository
	quota      openAI429QuotaQuerier
	gateway    *OpenAIGatewayService
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	queue      chan int64
	running    sync.Map
	startOnce  sync.Once
	configMu   sync.RWMutex
	enabledIDs map[int64]struct{}
	configVer  uint64
}

type openAI429Run struct {
	mu      sync.Mutex
	pending bool
	closing bool
}

type openAI429QuotaQuerier interface {
	QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error)
}

func NewOpenAI429ModeService(cache OpenAI429AtomicCache, accounts AccountRepository, quota openAI429QuotaQuerier, gateway *OpenAIGatewayService) *OpenAI429ModeService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenAI429ModeService{store: &openAI429StateStore{cache: cache}, accounts: accounts, quota: quota, gateway: gateway, ctx: ctx, cancel: cancel, queue: make(chan int64, 1024), enabledIDs: make(map[int64]struct{})}
}

func (s *OpenAI429ModeService) Start() {
	s.startOnce.Do(func() {
		s.scan()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case id := <-s.queue:
					s.startDrive(id)
				case <-ticker.C:
					s.scan()
				}
			}
		}()
	})
}

// Keep one driver per account while remembering notifications that arrive
// during a run. Dropping such a notification can otherwise delay the final
// in-flight result until the next periodic scan.
func (s *OpenAI429ModeService) startDrive(id int64) {
	for {
		run := &openAI429Run{}
		actual, loaded := s.running.LoadOrStore(id, run)
		if !loaded {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				for {
					s.drive(id)
					run.mu.Lock()
					if run.pending {
						run.pending = false
						run.mu.Unlock()
						continue
					}
					run.closing = true
					run.mu.Unlock()
					s.running.CompareAndDelete(id, run)
					return
				}
			}()
			return
		}
		run = actual.(*openAI429Run)
		run.mu.Lock()
		if !run.closing {
			run.pending = true
			run.mu.Unlock()
			return
		}
		run.mu.Unlock()
		s.running.CompareAndDelete(id, run)
	}
}

func (s *OpenAI429ModeService) Stop() {
	if s != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *OpenAI429ModeService) notify(id int64) {
	if s == nil {
		return
	}
	select {
	case s.queue <- id:
	default:
	}
}

func (s *OpenAI429ModeService) scan() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	s.configMu.RLock()
	version := s.configVer
	s.configMu.RUnlock()
	accounts, err := s.accounts.FindByExtraField(ctx, OpenAI429ModeEnabledKey, true)
	if err != nil {
		slog.Warn("openai_429_mode_scan_failed", "error", err)
		return
	}
	enabled := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		if OpenAI429ModeEnabled(&accounts[i]) {
			enabled[accounts[i].ID] = struct{}{}
		}
	}
	s.configMu.Lock()
	// A concurrent admin edit is newer than this scan. Preserve that edit and
	// let the next scan reconcile the complete set instead of publishing stale
	// data over the request-side fast path.
	if s.configVer == version {
		s.enabledIDs = enabled
		s.configVer++
	}
	s.configMu.Unlock()
	for id := range enabled {
		s.notify(id)
	}
}

func (s *OpenAI429ModeService) setConfiguredEnabled(id int64, enabled bool) {
	if s == nil {
		return
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	if enabled {
		if _, exists := s.enabledIDs[id]; exists {
			return
		}
		s.enabledIDs[id] = struct{}{}
	} else {
		if _, exists := s.enabledIDs[id]; !exists {
			return
		}
		delete(s.enabledIDs, id)
	}
	s.configVer++
}

func (s *OpenAI429ModeService) hasEnabledAccounts() bool {
	if s == nil {
		return false
	}
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return len(s.enabledIDs) > 0
}

func (s *OpenAI429ModeService) configuredEnabled(id int64) bool {
	if s == nil {
		return false
	}
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	_, ok := s.enabledIDs[id]
	return ok
}

func (s *OpenAI429ModeService) change(ctx context.Context, a *Account, fn func(*OpenAI429State)) (*OpenAI429State, error) {
	state, err := s.store.mutate(ctx, a, fn)
	if err != nil {
		return nil, err
	}
	if repo, ok := s.accounts.(OpenAI429ModeRepository); ok {
		projection := *state
		projection.Flights = nil
		if err := repo.StoreOpenAI429ModeState(ctx, a.ID, &projection); err != nil {
			return nil, err
		}
	}
	return state, nil
}

// Each admission uses current database configuration, independently of scheduler
// snapshots. A Redis or persistence failure fails closed for enabled accounts.
func (s *OpenAI429ModeService) begin(ctx context.Context, a *Account, model string) (*openAI429Ticket, error) {
	if !OpenAI429ModeEnabled(a) && (s == nil || !s.configuredEnabled(a.ID)) {
		return nil, nil
	}
	ticket, _, err := s.beginFresh(ctx, a, model)
	return ticket, err
}

func (s *OpenAI429ModeService) beginFresh(ctx context.Context, a *Account, model string) (*openAI429Ticket, bool, error) {
	if s == nil {
		if OpenAI429ModeEnabled(a) {
			return nil, true, errOpenAI429Admission
		}
		return nil, false, nil
	}
	if !SupportsOpenAI429Mode(a) {
		return nil, false, nil
	}
	fresh, err := s.accounts.GetByID(ctx, a.ID)
	if err != nil {
		return nil, OpenAI429ModeEnabled(a), fmt.Errorf("%w: %v", errOpenAI429Admission, err)
	}
	enabled := OpenAI429ModeEnabled(fresh)
	s.setConfiguredEnabled(a.ID, enabled)
	if !enabled {
		return nil, false, nil
	}
	if !fresh.IsSchedulable() {
		return nil, true, errOpenAI429Admission
	}
	id := uuid.NewString()
	allowed := false
	state, err := s.change(ctx, fresh, func(state *OpenAI429State) {
		allowed = false
		state.expire(time.Now().UnixMilli())
		state.observeQuota(fresh, time.Now())
		allowed = state.admit(id, model, time.Now().UnixMilli())
	})
	if err != nil {
		return nil, true, fmt.Errorf("%w: %v", errOpenAI429Admission, err)
	}
	if !allowed {
		s.notify(a.ID)
		return nil, true, errOpenAI429Admission
	}
	ticket := &openAI429Ticket{service: s, accountID: a.ID, generation: state.Generation, id: id, started: time.Now(), done: make(chan struct{})}
	go ticket.keepAlive()
	return ticket, true, nil
}

type openAI429Ticket struct {
	service    *OpenAI429ModeService
	accountID  int64
	generation string
	id         string
	started    time.Time
	once       sync.Once
	done       chan struct{}
}

func (t *openAI429Ticket) keepAlive() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-t.service.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(t.service.ctx, 5*time.Second)
			a, err := t.service.accounts.GetByID(ctx, t.accountID)
			if err == nil && OpenAI429ModeEnabled(a) && a.GetExtraString(OpenAI429ModeGenerationKey) == t.generation {
				_, _ = t.service.store.mutate(ctx, a, func(state *OpenAI429State) {
					if f, ok := state.Flights[t.id]; ok {
						f.ExpiresAt = time.Now().Add(2 * time.Minute).UnixMilli()
						state.Flights[t.id] = f
					}
				})
			}
			cancel()
		}
	}
}

func (t *openAI429Ticket) finish(outcome openAI429Outcome) {
	if t == nil {
		return
	}
	t.once.Do(func() {
		close(t.done)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a, err := t.service.accounts.GetByID(ctx, t.accountID)
		if err != nil || !OpenAI429ModeEnabled(a) || a.GetExtraString(OpenAI429ModeGenerationKey) != t.generation {
			return
		}
		outcome.Duration = time.Since(t.started)
		_, err = t.service.change(ctx, a, func(state *OpenAI429State) { state.finish(t.id, outcome, time.Now().UnixMilli()) })
		if err != nil {
			slog.Warn("openai_429_mode_result_failed", "account_id", t.accountID, "error", err)
			return
		}
		t.service.notify(t.accountID)
	})
}

func (s *OpenAI429ModeService) blocked(ctx context.Context, a *Account) bool {
	if !OpenAI429ModeEnabled(a) {
		return false
	}
	if s == nil {
		return true
	}
	state, err := s.store.mutate(ctx, a, func(state *OpenAI429State) {
		state.expire(time.Now().UnixMilli())
		state.observeQuota(a, time.Now())
	})
	if err != nil {
		return true
	}
	switch state.Phase {
	case OpenAI429Normal, OpenAI429Concentrating:
		return false
	case OpenAI429WaitingReal:
		return len(state.Flights) > 0 || state.ProbeOwner != ""
	default:
		s.notify(a.ID)
		return true
	}
}

func (s *OpenAI429ModeService) handles429(ctx context.Context, a *Account, headers map[string][]string, body []byte) bool {
	if !OpenAI429ModeEnabled(a) || openAI429ModeExcluded(ctx) || isOpenAIImageRateLimitError(429, body) {
		return false
	}
	if openAI429Classify(429, headers, body).QuotaExhausted {
		return true
	}
	if s == nil {
		return true
	}
	state, err := s.store.mutate(ctx, a, func(*OpenAI429State) {})
	return err != nil || state.Active
}

func (s *OpenAI429ModeService) Reset(ctx context.Context, id int64) error {
	if s == nil || !s.configuredEnabled(id) {
		return nil
	}
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !SupportsOpenAI429Mode(a) {
		s.setConfiguredEnabled(id, false)
		return nil
	}
	if !OpenAI429ModeEnabled(a) {
		s.setConfiguredEnabled(id, false)
		return nil
	}
	generation := newOpenAI429Generation()
	if err := s.accounts.UpdateExtra(ctx, id, map[string]any{OpenAI429ModeGenerationKey: generation, OpenAI429ModeStateKey: nil}); err != nil {
		return err
	}
	a.Extra = cloneOpenAIAutoResetExtra(a.Extra)
	a.Extra[OpenAI429ModeGenerationKey] = generation
	delete(a.Extra, OpenAI429ModeStateKey)
	_, err = s.change(ctx, a, func(state *OpenAI429State) { state.reset() })
	return err
}

func (s *OpenAI429ModeService) drive(id int64) {
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil || !OpenAI429ModeEnabled(a) {
		return
	}
	s.takeOverQuotaPause(ctx, a)
	state, err := s.change(ctx, a, func(state *OpenAI429State) {
		state.expire(time.Now().UnixMilli())
		state.observeQuota(a, time.Now())
	})
	if err != nil {
		slog.Warn("openai_429_mode_drive_failed", "account_id", id, "error", err)
		return
	}
	if state.Phase == OpenAI429Stopped {
		s.recover(ctx, a, state)
		return
	}
	if state.Phase != OpenAI429Draining || !state.Active || state.Model == "" || len(state.Flights) > 0 {
		return
	}
	// Original slots also cover requests sent before this switch was enabled.
	// Wait for their handlers to process results and release concurrency before
	// starting the automatic round. Waiting does not consume its 30 second budget.
	if s.gateway != nil && s.gateway.concurrencyService != nil {
		drainCtx, stopDrain := context.WithTimeout(ctx, 10*time.Second)
		defer stopDrain()
		for {
			counts, err := s.gateway.concurrencyService.GetAccountConcurrencyBatch(drainCtx, []int64{id})
			if err != nil || drainCtx.Err() != nil {
				return
			}
			if counts[id] == 0 {
				break
			}
			select {
			case <-drainCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	a, err = s.accounts.GetByID(ctx, id)
	if err != nil || !OpenAI429ModeEnabled(a) || a.GetExtraString(OpenAI429ModeGenerationKey) != state.Generation {
		return
	}
	owner := uuid.NewString()
	claimed := false
	state, err = s.change(ctx, a, func(st *OpenAI429State) {
		claimed = st.claimProbe(owner, time.Now().UnixMilli())
	})
	if err != nil || !claimed {
		return
	}
	deadline := time.UnixMilli(state.RoundStartedAt).Add(30 * time.Second)
	roundCtx, roundCancel := context.WithDeadline(ctx, deadline)
	defer roundCancel()
	// This watcher cancels an in-progress probe on disable, including changes
	// made on another instance. The generation also fences every late result.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-roundCtx.Done():
				return
			case <-ticker.C:
				fresh, err := s.accounts.GetByID(roundCtx, id)
				if err != nil || !OpenAI429ModeEnabled(fresh) || fresh.GetExtraString(OpenAI429ModeGenerationKey) != state.Generation {
					roundCancel()
					return
				}
			}
		}
	}()
	for {
		fresh, err := s.accounts.GetByID(ctx, id)
		if err != nil || !OpenAI429ModeEnabled(fresh) || fresh.GetExtraString(OpenAI429ModeGenerationKey) != state.Generation {
			return
		}
		probeID := uuid.NewString()
		allowed := false
		current, err := s.change(ctx, fresh, func(st *OpenAI429State) {
			allowed = st.nextProbe(owner, probeID, time.Now().UnixMilli())
		})
		if err != nil || !allowed {
			return
		}
		started := time.Now()
		outcome := openAI429Outcome{}
		if fresh.IsSchedulable() && s.gateway != nil && roundCtx.Err() == nil {
			outcome = s.gateway.runOpenAI429Probe(roundCtx, fresh, current.Model)
		}
		outcome.Duration = time.Since(started)
		if !outcome.Success && !outcome.RateLimited && !time.Now().Before(deadline) {
			outcome.BudgetExpired = true
		}
		fresh, err = s.accounts.GetByID(ctx, id)
		if err != nil || !OpenAI429ModeEnabled(fresh) || fresh.GetExtraString(OpenAI429ModeGenerationKey) != state.Generation {
			return
		}
		_, err = s.change(ctx, fresh, func(st *OpenAI429State) { st.finish(probeID, outcome, time.Now().UnixMilli()) })
		slog.Info("openai_429_mode_probe", "account_id", id, "duration_ms", outcome.Duration.Milliseconds(), "success", outcome.Success, "rate_limited", outcome.RateLimited, "input_tokens", outcome.Usage.InputTokens, "output_tokens", outcome.Usage.OutputTokens)
		if err != nil || (!outcome.Success && !outcome.RateLimited) {
			return
		}
	}
}

func (s *OpenAI429ModeService) recover(ctx context.Context, a *Account, state *OpenAI429State) {
	now := time.Now().UnixMilli()
	if state.RecoverAt > 0 && !state.UnknownReset {
		if now < state.RecoverAt {
			return
		}
		_, _ = s.change(ctx, a, func(st *OpenAI429State) {
			if st.Phase == OpenAI429Stopped && st.RecoverAt <= now && !st.UnknownReset {
				st.reset()
			}
		})
		return
	}
	query := false
	_, err := s.change(ctx, a, func(st *OpenAI429State) {
		query = st.Phase == OpenAI429Stopped && st.NextQuotaQueryAt <= now
		if query {
			st.NextQuotaQueryAt = now + int64(5*time.Minute/time.Millisecond)
		}
	})
	if err != nil || !query || s.quota == nil {
		return
	}
	usage, err := s.quota.QueryUsage(ctx, a.ID)
	if err != nil || usage == nil || usage.RateLimit == nil {
		return
	}
	rate := usage.RateLimit
	if !rate.Allowed || rate.LimitReached || (rate.PrimaryWindow == nil && rate.SecondaryWindow == nil) {
		return
	}
	updates := buildOpenAIAutoResetUsageUpdates(usage, time.Now())
	for i, window := range []string{"5h", "7d"} {
		percent, known := resolveAccountExtraNumber(updates, "codex_"+window+"_used_percent")
		if (known && percent >= 100) || (!known && state.ExhaustedWindows&(1<<i) != 0) {
			return
		}
	}
	fresh, err := s.accounts.GetByID(ctx, a.ID)
	if err != nil || !OpenAI429ModeEnabled(fresh) || fresh.GetExtraString(OpenAI429ModeGenerationKey) != state.Generation {
		return
	}
	if err := s.accounts.UpdateExtra(ctx, a.ID, updates); err != nil {
		return
	}
	_, _ = s.change(ctx, fresh, func(st *OpenAI429State) {
		if st.Phase == OpenAI429Stopped {
			st.reset()
		}
	})
}

func openAI429StateFromExtra(a *Account) *OpenAI429State {
	if a == nil {
		return nil
	}
	data, err := json.Marshal(a.Extra[OpenAI429ModeStateKey])
	if err != nil {
		return nil
	}
	var state OpenAI429State
	if json.Unmarshal(data, &state) != nil || state.Generation != a.GetExtraString(OpenAI429ModeGenerationKey) {
		return nil
	}
	return &state
}
