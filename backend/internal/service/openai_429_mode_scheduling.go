package service

import (
	"context"
	"fmt"
	"sort"
	"time"
)

type openAI429ModeExcludedKey struct{}

func WithOpenAI429ModeExcluded(ctx context.Context, excluded bool) context.Context {
	return context.WithValue(ctx, openAI429ModeExcludedKey{}, excluded)
}

func openAI429ModeExcluded(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	excluded, _ := ctx.Value(openAI429ModeExcludedKey{}).(bool)
	return excluded
}

// Runs after group policy construction and before ordinary session stickiness.
// Acquisition uses the original scheduler's concurrency and DB recheck path.
func (s *OpenAIGatewayService) selectOpenAI429Concentrated(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, error) {
	if s == nil || s.openAI429Mode == nil || !s.openAI429Mode.hasEnabledAccounts() || req.Platform != PlatformOpenAI || openAI429ModeExcluded(ctx) || req.RequiredImageCapability != "" || (req.PreviousResponseID != "" && !req.PreviousResponseCanMove) || req.GuardianParentAccountID > 0 || !openAI429TextRequest("/responses", req.RequestedModel, nil) {
		return nil, nil
	}
	accounts, err := s.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return nil, err
	}
	scheduler := &defaultOpenAIAccountScheduler{service: s, stats: newOpenAIAccountRuntimeStats()}
	req.RequirePrivacySet = s.openAIGroupRequiresPrivacySet(ctx, req.GroupID)
	candidates := make([]openAIAccountCandidateScore, 0)
	used := map[int64]float64{}
	now := time.Now()
	for i := range accounts {
		a := &accounts[i]
		if !OpenAI429ModeEnabled(a) {
			continue
		}
		if _, excluded := req.ExcludedIDs[a.ID]; excluded {
			continue
		}
		percent, _, _, _ := openAI429Quota(a, now)
		if req.RequirePrivacySet && !a.IsPrivacySet() {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(a) == 0 {
			continue
		}
		if percent < 90 || !a.IsSchedulable() || !scheduler.isAccountRequestCompatible(ctx, a, req) || !scheduler.isAccountTransportCompatible(a, req.RequiredTransport) || s.isOpenAIAccountRequestRuntimeBlockedForRequest(ctx, a, req.RequestedModel) {
			continue
		}
		used[a.ID] = percent
		candidates = append(candidates, openAIAccountCandidateScore{account: a})
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i].account, candidates[j].account
		if used[a.ID] != used[b.ID] {
			return used[a.ID] > used[b.ID]
		}
		if openAIAccountSchedulingPriority(a) != openAIAccountSchedulingPriority(b) {
			return openAIAccountSchedulingPriority(a) < openAIAccountSchedulingPriority(b)
		}
		return a.ID < b.ID
	})
	if len(candidates) == 0 {
		return nil, nil
	}
	if s.checkChannelPricingRestriction(ctx, req.GroupID, req.RequestedModel) {
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, req.RequestedModel)
	}
	// Do not truncate concentration ordering to the ordinary scheduler's sampled
	// candidate budget: a full account must yield to the next highest usage one.
	result, _, err := scheduler.tryAcquireOpenAISelectionOrderWithBudget(ctx, req, candidates, newOpenAISelectionProbeBudget())
	return result, err
}
