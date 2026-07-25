package service

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyBillingChargeMultiplier(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 1.0, applyBillingChargeMultiplier(1.0, 1), 1e-12)
	require.InDelta(t, 1.1, applyBillingChargeMultiplier(1.0, 1.1), 1e-12)
	require.InDelta(t, 0.9, applyBillingChargeMultiplier(1.0, 0.9), 1e-12)
	require.Zero(t, applyBillingChargeMultiplier(0, 1.1))
	require.InDelta(t, 1.0, applyBillingChargeMultiplier(1.0, 0), 1e-12)
	require.InDelta(t, 1.0, applyBillingChargeMultiplier(1.0, -1), 1e-12)
	require.InDelta(t, 10.0, applyBillingChargeMultiplier(1.0, 11), 1e-12)
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
	value string
	err   error
	hits  atomic.Int64
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
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := r.GetValue(ctx, key)
		if err == nil {
			out[key] = value
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
	svc.StopBillingSettingInvalidationSubscriber()
}

func TestPublishBillingChargeMultiplierUsesNormalizedPayload(t *testing.T) {
	invalidation := &billingSettingInvalidationStub{published: make(chan string, 1)}
	svc := &SettingService{billingSettingInvalidation: invalidation}
	svc.publishBillingChargeMultiplier(context.Background(), 1.25)
	require.Equal(t, "1.25", <-invalidation.published)
}
