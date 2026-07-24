package service

import (
	"context"
	"errors"
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

// GetBillingChargeMultiplier returns the system charge multiplier for the billing
// hot path. It never blocks on the database: fresh cache hits return immediately,
// stale values are served while a background refresh runs (stale-while-revalidate).
func (s *SettingService) GetBillingChargeMultiplier(ctx context.Context) float64 {
	if s == nil {
		return defaultBillingChargeMultiplier
	}
	cached, _ := s.billingChargeMultiplierCache.Load().(*cachedBillingChargeMultiplier)
	now := time.Now().UnixNano()
	if cached != nil && now < cached.expiresAt {
		return cached.value
	}
	s.billingChargeMultiplierSF.DoChan(billingChargeMultiplierRefreshKey, func() (any, error) {
		s.refreshBillingChargeMultiplier(context.Background())
		return nil, nil
	})
	if cached != nil {
		return cached.value
	}
	return defaultBillingChargeMultiplier
}

// WarmBillingChargeMultiplier synchronously loads the multiplier into cache.
func (s *SettingService) WarmBillingChargeMultiplier(ctx context.Context) float64 {
	if s == nil {
		return defaultBillingChargeMultiplier
	}
	s.refreshBillingChargeMultiplier(ctx)
	cached, _ := s.billingChargeMultiplierCache.Load().(*cachedBillingChargeMultiplier)
	if cached == nil {
		return defaultBillingChargeMultiplier
	}
	return cached.value
}

func (s *SettingService) storeBillingChargeMultiplierCache(value float64) {
	if s == nil {
		return
	}
	s.billingChargeMultiplierCache.Store(&cachedBillingChargeMultiplier{
		value:     normalizeBillingChargeMultiplier(value),
		expiresAt: time.Now().Add(billingChargeMultiplierCacheTTL).UnixNano(),
	})
}

func (s *SettingService) refreshBillingChargeMultiplier(ctx context.Context) {
	if s == nil || s.settingRepo == nil {
		return
	}
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
	s.billingChargeMultiplierCache.Store(&cachedBillingChargeMultiplier{
		value:     value,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}

// ResolveBillingChargeMultiplier is a nil-safe helper for gateway services.
func ResolveBillingChargeMultiplier(settingService *SettingService, ctx context.Context) float64 {
	if settingService == nil {
		return defaultBillingChargeMultiplier
	}
	return settingService.GetBillingChargeMultiplier(ctx)
}
