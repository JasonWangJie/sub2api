package service

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyBillingChargeMultiplierToCostMovesSystemMultiplierToComponents(t *testing.T) {
	t.Parallel()

	cost := &CostBreakdown{
		InputCost:         1,
		ImageInputCost:    2,
		OutputCost:        3,
		ImageOutputCost:   4,
		CacheCreationCost: 5,
		CacheReadCost:     6,
		TotalCost:         22,
		ActualCost:        44,
		BillingMode:       string(BillingModeToken),
	}

	applied := applyBillingChargeMultiplierToCost(cost, 1.5, 2)

	require.InDelta(t, 1.5, applied, 1e-12)
	require.InDelta(t, 1.5, cost.InputCost, 1e-12)
	require.InDelta(t, 2.0, cost.ImageInputCost, 1e-12)
	require.InDelta(t, 4.5, cost.OutputCost, 1e-12)
	require.InDelta(t, 4.0, cost.ImageOutputCost, 1e-12)
	require.InDelta(t, 5.0, cost.CacheCreationCost, 1e-12)
	require.InDelta(t, 9.0, cost.CacheReadCost, 1e-12)
	// Cache creation, image token costs, and the 1-unit surcharge stay unchanged.
	require.InDelta(t, 27, cost.TotalCost, 1e-12)
	// The original ordinary multiplier is 2x; the system coefficient is not
	// applied to ActualCost as a separate final-stage multiplier anymore.
	require.InDelta(t, 54, cost.ActualCost, 1e-12)

	applied = applyBillingChargeMultiplierToCost(cost, 0, 2)
	require.InDelta(t, 1, applied, 1e-12)
	require.InDelta(t, 27, cost.TotalCost, 1e-12, "invalid multiplier falls back to 1")
}

func TestApplyBillingChargeMultiplierToCostPreservesExcludedSurchargeCharge(t *testing.T) {
	t.Parallel()

	// The token component uses a 2x peak rate while the 1-unit search
	// surcharge was charged at its separate 0.5x base rate.
	cost := &CostBreakdown{
		InputCost:   10,
		TotalCost:   11,
		ActualCost:  20.5,
		BillingMode: string(BillingModeToken),
	}

	applyBillingChargeMultiplierToCost(cost, 1.5, 2)

	require.InDelta(t, 15, cost.InputCost, 1e-12)
	require.InDelta(t, 16, cost.TotalCost, 1e-12)
	require.InDelta(t, 30.5, cost.ActualCost, 1e-12)
}

func TestShouldApplyBillingChargeMultiplierExcludesMediaGeneration(t *testing.T) {
	t.Parallel()

	require.True(t, shouldApplyBillingChargeMultiplier(&CostBreakdown{InputCost: 1, BillingMode: string(BillingModeToken)}, 0, false))
	require.False(t, shouldApplyBillingChargeMultiplier(nil, 0, false))
	require.False(t, shouldApplyBillingChargeMultiplier(&CostBreakdown{InputCost: 1, BillingMode: string(BillingModeToken)}, 1, false))
	require.False(t, shouldApplyBillingChargeMultiplier(&CostBreakdown{BillingMode: string(BillingModeToken)}, 0, false))
	require.False(t, shouldApplyBillingChargeMultiplier(&CostBreakdown{InputCost: 1, BillingMode: string(BillingModePerRequest)}, 0, false))
	require.False(t, shouldApplyBillingChargeMultiplier(&CostBreakdown{BillingMode: string(BillingModeImage)}, 0, false))
	require.False(t, shouldApplyBillingChargeMultiplier(&CostBreakdown{BillingMode: string(BillingModeVideo)}, 0, false))
	require.False(t, shouldApplyBillingChargeMultiplier(&CostBreakdown{InputCost: 1, BillingMode: string(BillingModeToken)}, 0, true))
}

func TestParseBillingChargeMultiplier(t *testing.T) {
	t.Parallel()

	require.Equal(t, 1.0, parseBillingChargeMultiplier(""))
	require.Equal(t, 1.0, parseBillingChargeMultiplier("abc"))
	require.Equal(t, 1.0, parseBillingChargeMultiplier("0"))
	require.Equal(t, 1.1, parseBillingChargeMultiplier("1.1"))
	require.Equal(t, 10.0, parseBillingChargeMultiplier("99"))
}

func TestParseBillingChargeMultiplierUpdateRejectsInvalidPayloads(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "abc", "0", "-1", "10.01", "NaN", "+Inf"} {
		_, ok := parseBillingChargeMultiplierUpdate(raw)
		require.False(t, ok, raw)
	}
	value, ok := parseBillingChargeMultiplierUpdate("1.25")
	require.True(t, ok)
	require.Equal(t, 1.25, value)
}

func TestValidateBillingChargeMultiplier(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateBillingChargeMultiplier(1))
	require.NoError(t, validateBillingChargeMultiplier(1.1))
	require.NoError(t, validateBillingChargeMultiplier(0.01))
	require.Error(t, validateBillingChargeMultiplier(0))
	require.Error(t, validateBillingChargeMultiplier(-1))
	require.Error(t, validateBillingChargeMultiplier(10.01))
}

type billingChargeMultiplierSettingRepo struct {
	value        string
	allGroupsRaw string
	groupIDsRaw  string
	err          error
	hits         atomic.Int64
}

func (r *billingChargeMultiplierSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (r *billingChargeMultiplierSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	r.hits.Add(1)
	if r.err != nil {
		return "", r.err
	}
	if key != SettingKeyBillingChargeMultiplier {
		return "", ErrSettingNotFound
	}
	return r.value, nil
}
func (r *billingChargeMultiplierSettingRepo) Set(ctx context.Context, key, value string) error {
	return nil
}
func (r *billingChargeMultiplierSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	r.hits.Add(1)
	if r.err != nil {
		return nil, r.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		switch key {
		case SettingKeyBillingChargeMultiplier:
			out[key] = r.value
		case SettingKeyBillingChargeMultiplierAllGroups:
			if r.allGroupsRaw != "" {
				out[key] = r.allGroupsRaw
			}
		case SettingKeyBillingChargeMultiplierGroupIDs:
			if r.groupIDsRaw != "" {
				out[key] = r.groupIDsRaw
			}
		}
	}
	return out, nil
}
func (r *billingChargeMultiplierSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
}
func (r *billingChargeMultiplierSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *billingChargeMultiplierSettingRepo) Delete(ctx context.Context, key string) error {
	return nil
}

func TestGetBillingChargeMultiplier_HotPathUsesCache(t *testing.T) {
	repo := &billingChargeMultiplierSettingRepo{value: "1.25"}
	svc := &SettingService{settingRepo: repo}
	require.Equal(t, 1.25, svc.WarmBillingChargeMultiplier(context.Background()))
	hitsAfterWarm := repo.hits.Load()

	for i := 0; i < 20; i++ {
		require.Equal(t, 1.25, svc.GetBillingChargeMultiplier(context.Background()))
	}
	require.Equal(t, hitsAfterWarm, repo.hits.Load(), "fresh cache must not hit DB")
}

func TestGetBillingChargeMultiplier_ColdCacheLoadsBeforeReturning(t *testing.T) {
	repo := &billingChargeMultiplierSettingRepo{value: "1.5"}
	svc := &SettingService{settingRepo: repo}

	got := svc.GetBillingChargeMultiplier(context.Background())
	require.Equal(t, 1.5, got)
	require.Equal(t, int64(1), repo.hits.Load())
}

func TestGetBillingChargeMultiplierForGroup_DefaultsToAllGroups(t *testing.T) {
	repo := &billingChargeMultiplierSettingRepo{value: "1.5"}
	svc := &SettingService{settingRepo: repo}

	require.Equal(t, 1.5, svc.GetBillingChargeMultiplierForGroup(context.Background(), nil))
	groupID := int64(42)
	require.Equal(t, 1.5, svc.GetBillingChargeMultiplierForGroup(context.Background(), &groupID))
}

func TestGetBillingChargeMultiplierForGroup_SelectedGroupsOnly(t *testing.T) {
	repo := &billingChargeMultiplierSettingRepo{
		value:        "1.5",
		allGroupsRaw: "false",
		groupIDsRaw:  `[9, 7, 9, 0, -1]`,
	}
	svc := &SettingService{settingRepo: repo}

	selected := int64(7)
	unselected := int64(8)
	require.Equal(t, 1.5, svc.GetBillingChargeMultiplierForGroup(context.Background(), &selected))
	require.Equal(t, 1.0, svc.GetBillingChargeMultiplierForGroup(context.Background(), &unselected))
	require.Equal(t, 1.0, svc.GetBillingChargeMultiplierForGroup(context.Background(), nil))
}

func TestGetBillingChargeMultiplierForGroup_ExplicitEmptyScopeDisablesMultiplier(t *testing.T) {
	repo := &billingChargeMultiplierSettingRepo{
		value:        "1.5",
		allGroupsRaw: "false",
		groupIDsRaw:  `[]`,
	}
	svc := &SettingService{settingRepo: repo}
	groupID := int64(7)

	require.Equal(t, 1.0, svc.GetBillingChargeMultiplierForGroup(context.Background(), &groupID))
}

func TestGetBillingChargeMultiplierForGroup_InvalidScopeFallsBackToAllGroups(t *testing.T) {
	repo := &billingChargeMultiplierSettingRepo{
		value:        "1.5",
		allGroupsRaw: "false",
		groupIDsRaw:  `invalid`,
	}
	svc := &SettingService{settingRepo: repo}
	groupID := int64(7)

	require.Equal(t, 1.5, svc.GetBillingChargeMultiplierForGroup(context.Background(), &groupID))
}

func TestStoreBillingChargeMultiplierCache_WriteThrough(t *testing.T) {
	svc := &SettingService{}
	svc.storeBillingChargeMultiplierCache(1.1)
	require.Equal(t, 1.1, svc.GetBillingChargeMultiplier(context.Background()))
}

type blockingBillingChargeMultiplierRepo struct {
	*billingChargeMultiplierSettingRepo
	started chan struct{}
	release chan struct{}
}

func (r *blockingBillingChargeMultiplierRepo) GetValue(ctx context.Context, key string) (string, error) {
	close(r.started)
	select {
	case <-r.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return r.billingChargeMultiplierSettingRepo.GetValue(ctx, key)
}

func (r *blockingBillingChargeMultiplierRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	close(r.started)
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.billingChargeMultiplierSettingRepo.GetMultiple(ctx, keys)
}

func TestBillingChargeMultiplier_StaleRefreshCannotOverwriteAdminUpdate(t *testing.T) {
	repo := &blockingBillingChargeMultiplierRepo{
		billingChargeMultiplierSettingRepo: &billingChargeMultiplierSettingRepo{value: "1.25"},
		started:                            make(chan struct{}),
		release:                            make(chan struct{}),
	}
	svc := &SettingService{settingRepo: repo}
	done := make(chan float64, 1)
	go func() { done <- svc.GetBillingChargeMultiplier(context.Background()) }()
	<-repo.started
	svc.storeBillingChargeMultiplierCache(1.75)
	close(repo.release)

	require.Equal(t, 1.75, <-done)
	require.Equal(t, 1.75, svc.GetBillingChargeMultiplier(context.Background()))
}

type billingSettingInvalidationStub struct {
	updates   chan string
	handled   chan struct{}
	started   chan struct{}
	published chan string
}

func (s *billingSettingInvalidationStub) PublishBillingChargeMultiplier(ctx context.Context, value string) error {
	select {
	case s.published <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *billingSettingInvalidationStub) SubscribeBillingChargeMultiplier(ctx context.Context, handler func(string)) error {
	close(s.started)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case value := <-s.updates:
			handler(value)
			s.handled <- struct{}{}
		}
	}
}

func TestBillingChargeMultiplierSubscriberUpdatesCacheAndIgnoresInvalidPayload(t *testing.T) {
	invalidation := &billingSettingInvalidationStub{
		updates: make(chan string), handled: make(chan struct{}),
		started: make(chan struct{}), published: make(chan string, 1),
	}
	svc := &SettingService{billingSettingInvalidation: invalidation}
	svc.storeBillingChargeMultiplierCache(1.25)
	svc.StartBillingSettingInvalidationSubscriber(context.Background())
	<-invalidation.started

	invalidation.updates <- "invalid"
	<-invalidation.handled
	require.Equal(t, 1.25, svc.GetBillingChargeMultiplier(context.Background()))

	invalidation.updates <- "1.75"
	<-invalidation.handled
	require.Equal(t, 1.75, svc.GetBillingChargeMultiplier(context.Background()))

	invalidation.updates <- `{"multiplier":1.5,"all_groups":false,"group_ids":[7]}`
	<-invalidation.handled
	selected := int64(7)
	unselected := int64(8)
	require.Equal(t, 1.5, svc.GetBillingChargeMultiplierForGroup(context.Background(), &selected))
	require.Equal(t, 1.0, svc.GetBillingChargeMultiplierForGroup(context.Background(), &unselected))
	svc.StopBillingSettingInvalidationSubscriber()
}

func TestPublishBillingChargePolicyUsesNormalizedPayload(t *testing.T) {
	invalidation := &billingSettingInvalidationStub{published: make(chan string, 1)}
	svc := &SettingService{billingSettingInvalidation: invalidation}
	svc.publishBillingChargePolicy(context.Background(), 1.25, false, []int64{9, 7, 9, -1})
	require.JSONEq(t, `{"multiplier":1.25,"all_groups":false,"group_ids":[7,9]}`, <-invalidation.published)
}
