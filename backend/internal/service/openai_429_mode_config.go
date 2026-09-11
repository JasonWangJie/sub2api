package service

import (
	"fmt"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const (
	OpenAI429ModeEnabledKey    = "openai_429_mode_enabled"
	OpenAI429ModeStateKey      = "openai_429_mode_state"
	OpenAI429ModeGenerationKey = "openai_429_mode_generation"
	openAI429SnapshotTTL       = 5 * time.Minute
)

func SupportsOpenAI429Mode(a *Account) bool {
	return a != nil && a.IsOpenAIOAuthLike() && !a.IsShadow() && (a.QuotaDimension == "" || a.QuotaDimension == "global")
}

func OpenAI429ModeEnabled(a *Account) bool {
	if !SupportsOpenAI429Mode(a) {
		return false
	}
	enabled, _ := a.Extra[OpenAI429ModeEnabledKey].(bool)
	return enabled
}

func normalizeOpenAI429ModeExtra(a *Account, extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	out := cloneOpenAIAutoResetExtra(extra)
	delete(out, OpenAI429ModeStateKey)
	delete(out, OpenAI429ModeGenerationKey)
	if value, exists := out[OpenAI429ModeEnabledKey]; exists {
		if _, ok := value.(bool); !ok {
			return nil, infraerrors.BadRequest("OPENAI_429_MODE_BOOLEAN_REQUIRED", "openai_429_mode_enabled must be a boolean")
		}
		if !SupportsOpenAI429Mode(a) {
			return nil, infraerrors.BadRequest("OPENAI_429_MODE_ACCOUNT_UNSUPPORTED", fmt.Sprintf("account %d (%s) does not support OpenAI 429 mode", a.ID, a.Name))
		}
	} else if value, exists := a.Extra[OpenAI429ModeEnabledKey]; exists {
		out[OpenAI429ModeEnabledKey] = value
	}
	before := OpenAI429ModeEnabled(a)
	after, _ := out[OpenAI429ModeEnabledKey].(bool)
	if after && !SupportsOpenAI429Mode(a) {
		return nil, infraerrors.BadRequest("OPENAI_429_MODE_ACCOUNT_UNSUPPORTED", fmt.Sprintf("disable OpenAI 429 mode before changing account %d type", a.ID))
	}
	if before != after || (after && a.GetExtraString(OpenAI429ModeGenerationKey) == "") {
		out[OpenAI429ModeGenerationKey] = newOpenAI429Generation()
	} else {
		if v, ok := a.Extra[OpenAI429ModeGenerationKey]; ok {
			out[OpenAI429ModeGenerationKey] = v
		}
		if after {
			if v, ok := a.Extra[OpenAI429ModeStateKey]; ok {
				out[OpenAI429ModeStateKey] = v
			}
		}
	}
	return out, nil
}

func newOpenAI429Generation() string {
	return fmt.Sprintf("%020d-%s", time.Now().UnixNano(), uuid.NewString())
}

// Only fresh, finite snapshots from the ordinary text pool can activate mode.
func openAI429Quota(a *Account, now time.Time) (used float64, exhausted bool, resetAt int64, unknownReset bool) {
	if !OpenAI429ModeEnabled(a) {
		return
	}
	updated, err := parseTime(fmt.Sprint(a.Extra["codex_usage_updated_at"]))
	if err != nil || updated.After(now.Add(time.Minute)) || now.Sub(updated) >= openAI429SnapshotTTL {
		return
	}
	for _, window := range []string{"5h", "7d"} {
		percent, ok := resolveAccountExtraNumber(a.Extra, "codex_"+window+"_used_percent")
		if !ok || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 {
			continue
		}
		reset, err := parseTime(fmt.Sprint(a.Extra["codex_"+window+"_reset_at"]))
		if err == nil && !reset.After(now) {
			continue
		}
		if percent > used {
			used = percent
		}
		if percent >= 100 {
			exhausted = true
			if err != nil {
				unknownReset = true
			} else if reset.UnixMilli() > resetAt {
				resetAt = reset.UnixMilli()
			}
		}
	}
	return
}
