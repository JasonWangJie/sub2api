package service

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	defaultBillingChargeMultiplier    = 1.0
	maxBillingChargeMultiplier        = 10.0
	billingChargeMultiplierCacheTTL   = 60 * time.Second
	billingChargeMultiplierErrorTTL   = 10 * time.Second
	billingChargeMultiplierDBTimeout  = 2 * time.Second
	billingChargeMultiplierPublishTTL = 2 * time.Second
	billingChargeMultiplierRefreshKey = "billing_charge_multiplier"
)

type cachedBillingChargeMultiplier struct {
	value     float64
	expiresAt int64
}

// applyBillingChargeMultiplier scales ActualCost after group/user/peak multipliers.
// Invalid multipliers fall back to 1 so billing never breaks on misconfiguration.
func applyBillingChargeMultiplier(actualCost, multiplier float64) float64 {
	if actualCost <= 0 {
		return actualCost
	}
	m := normalizeBillingChargeMultiplier(multiplier)
	if m == 1 {
		return actualCost
	}
	return actualCost * m
}

func normalizeBillingChargeMultiplier(value float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return defaultBillingChargeMultiplier
	}
	if value > maxBillingChargeMultiplier {
		return maxBillingChargeMultiplier
	}
	return value
}

func parseBillingChargeMultiplier(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return defaultBillingChargeMultiplier
	}
	return normalizeBillingChargeMultiplier(value)
}

func parseBillingChargeMultiplierUpdate(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || validateBillingChargeMultiplier(value) != nil {
		return 0, false
	}
	return value, true
}

func validateBillingChargeMultiplier(value float64) error {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return infraerrors.BadRequest(
			"INVALID_BILLING_CHARGE_MULTIPLIER",
			"billing charge multiplier must be a finite number greater than 0",
		)
	}
	if value > maxBillingChargeMultiplier {
		return infraerrors.BadRequest(
			"INVALID_BILLING_CHARGE_MULTIPLIER",
			"billing charge multiplier must be less than or equal to 10",
		)
	}
	return nil
}

// GetBillingChargeMultiplier returns the system charge multiplier for the
// billing hot path. A cold or expired cache performs one synchronous,
// singleflight-coalesced read so a process restart cannot silently bill with 1.
func (s *SettingService) GetBillingChargeMultiplier(ctx context.Context) float64 {
	if s == nil {
		return defaultBillingChargeMultiplier
	}
	cached, _ := s.billingChargeMultiplierCache.Load().(*cachedBillingChargeMultiplier)
	now := time.Now().UnixNano()
	if cached != nil && now < cached.expiresAt {
		return cached.value
	}
	_, _, _ = s.billingChargeMultiplierSF.Do(billingChargeMultiplierRefreshKey, func() (any, error) {
		if current, _ := s.billingChargeMultiplierCache.Load().(*cachedBillingChargeMultiplier); current != nil && time.Now().UnixNano() < current.expiresAt {
			return current, nil
		}
		return s.refreshBillingChargeMultiplier(ctx), nil
	})
	if current, _ := s.billingChargeMultiplierCache.Load().(*cachedBillingChargeMultiplier); current != nil {
		return current.value
	}
	return defaultBillingChargeMultiplier
}

// WarmBillingChargeMultiplier synchronously loads the multiplier into cache.
func (s *SettingService) WarmBillingChargeMultiplier(ctx context.Context) float64 {
	if s == nil {
		return defaultBillingChargeMultiplier
	}
	return s.GetBillingChargeMultiplier(ctx)
}

func (s *SettingService) storeBillingChargeMultiplierCache(value float64) {
	if s == nil {
		return
	}
	s.billingChargeMultiplierMu.Lock()
	defer s.billingChargeMultiplierMu.Unlock()
	s.billingChargeMultiplierGeneration.Add(1)
	s.billingChargeMultiplierCache.Store(&cachedBillingChargeMultiplier{
		value:     normalizeBillingChargeMultiplier(value),
		expiresAt: time.Now().Add(billingChargeMultiplierCacheTTL).UnixNano(),
	})
}

func (s *SettingService) refreshBillingChargeMultiplier(ctx context.Context) *cachedBillingChargeMultiplier {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	generation := s.billingChargeMultiplierGeneration.Load()
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), billingChargeMultiplierDBTimeout)
	defer cancel()

	value := defaultBillingChargeMultiplier
	ttl := billingChargeMultiplierCacheTTL
	raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyBillingChargeMultiplier)
	if err == nil {
		value = parseBillingChargeMultiplier(raw)
	} else if !errors.Is(err, ErrSettingNotFound) {
		if prior, _ := s.billingChargeMultiplierCache.Load().(*cachedBillingChargeMultiplier); prior != nil {
			value = prior.value
		}
		ttl = billingChargeMultiplierErrorTTL
	}
	entry := &cachedBillingChargeMultiplier{
		value:     value,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	}
	s.billingChargeMultiplierMu.Lock()
	defer s.billingChargeMultiplierMu.Unlock()
	if generation != s.billingChargeMultiplierGeneration.Load() {
		current, _ := s.billingChargeMultiplierCache.Load().(*cachedBillingChargeMultiplier)
		return current
	}
	s.billingChargeMultiplierCache.Store(entry)
	return entry
}

func (s *SettingService) SetBillingSettingInvalidation(invalidation BillingSettingInvalidation) {
	if s != nil {
		s.billingSettingInvalidation = invalidation
	}
}

func (s *SettingService) publishBillingChargeMultiplier(ctx context.Context, value float64) {
	if s == nil || s.billingSettingInvalidation == nil {
		return
	}
	raw := strconv.FormatFloat(normalizeBillingChargeMultiplier(value), 'f', -1, 64)
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), billingChargeMultiplierPublishTTL)
	defer cancel()
	if err := s.billingSettingInvalidation.PublishBillingChargeMultiplier(publishCtx, raw); err != nil {
		slog.Warn("failed to publish billing charge multiplier update", "error", err)
	}
}

// StartBillingSettingInvalidationSubscriber keeps the process-local cache in
// sync after another instance commits an admin settings update.
func (s *SettingService) StartBillingSettingInvalidationSubscriber(ctx context.Context) {
	if s == nil || s.billingSettingInvalidation == nil {
		return
	}
	s.billingInvalidationStart.Do(func() {
		subscriberCtx, cancel := context.WithCancel(ctx)
		s.billingInvalidationCancel = cancel
		s.billingInvalidationWG.Add(1)
		go func() {
			defer s.billingInvalidationWG.Done()
			backoff := time.Second
			for {
				err := s.billingSettingInvalidation.SubscribeBillingChargeMultiplier(subscriberCtx, func(raw string) {
					if value, ok := parseBillingChargeMultiplierUpdate(raw); ok {
						s.storeBillingChargeMultiplierCache(value)
					}
				})
				if subscriberCtx.Err() != nil {
					return
				}
				if err == nil {
					err = errors.New("billing setting invalidation subscription closed")
				}
				slog.Warn("billing setting invalidation subscriber failed; retrying", "error", err, "retry_in", backoff)
				timer := time.NewTimer(backoff)
				select {
				case <-subscriberCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if backoff < 30*time.Second {
					backoff *= 2
					if backoff > 30*time.Second {
						backoff = 30 * time.Second
					}
				}
			}
		}()
	})
}

func (s *SettingService) StopBillingSettingInvalidationSubscriber() {
	if s == nil {
		return
	}
	s.billingInvalidationStop.Do(func() {
		if s.billingInvalidationCancel != nil {
			s.billingInvalidationCancel()
		}
		s.billingInvalidationWG.Wait()
	})
}

// ResolveBillingChargeMultiplier is a nil-safe helper for gateway services.
func ResolveBillingChargeMultiplier(settingService *SettingService, ctx context.Context) float64 {
	if settingService == nil {
		return defaultBillingChargeMultiplier
	}
	return settingService.GetBillingChargeMultiplier(ctx)
}
