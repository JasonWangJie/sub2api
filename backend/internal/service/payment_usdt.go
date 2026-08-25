package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// USDTCheckoutInfo is intentionally separate from the ordinary checkout
// response so the purchase page can never discover the USDT method by accident.
type USDTCheckoutInfo struct {
	Enabled                   bool          `json:"enabled"`
	Currency                  string        `json:"currency"`
	MinAmount                 float64       `json:"min_amount"`
	MaxAmount                 float64       `json:"max_amount"`
	DailyLimit                float64       `json:"daily_limit"`
	FeeRate                   float64       `json:"fee_rate"`
	BalanceRechargeMultiplier float64       `json:"balance_recharge_multiplier"`
	BonusRate                 float64       `json:"bonus_rate"`
	ExchangeRate              float64       `json:"exchange_rate"`
	ExchangeRateSource        string        `json:"exchange_rate_source"`
	ExchangeRateAt            string        `json:"exchange_rate_at,omitempty"`
	ExchangeRateStale         bool          `json:"exchange_rate_stale"`
	Networks                  []USDTNetwork `json:"networks"`
}

type USDTNetwork struct {
	Code        string `json:"code"`
	Alias       string `json:"alias,omitempty"`
	DisplayName string `json:"display_name"`
}

type epusdtUnifiedConfig struct {
	Currency  string
	BonusRate float64
}

func (s *PaymentConfigService) GetUSDTCheckoutInfo(ctx context.Context) (*USDTCheckoutInfo, error) {
	cfg, err := s.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	result := &USDTCheckoutInfo{
		Currency: payment.DefaultPaymentCurrency, MinAmount: cfg.MinAmount, MaxAmount: cfg.MaxAmount,
		DailyLimit: cfg.DailyLimit, FeeRate: 0,
		BalanceRechargeMultiplier: cfg.BalanceRechargeMultiplier,
		Networks:                  []USDTNetwork{},
	}
	if !cfg.Enabled || cfg.BalanceDisabled {
		return result, nil
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true), paymentproviderinstance.ProviderKeyEQ(payment.TypeEpusdt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query epusdt instances: %w", err)
	}
	seen := map[string]USDTNetwork{}
	configCurrency := ""
	configBonus := math.NaN()
	matching := make([]*dbent.PaymentProviderInstance, 0, len(instances))
	for _, inst := range instances {
		if !payment.InstanceSupportsType(inst.SupportedTypes, payment.TypeUSDT) {
			continue
		}
		matching = append(matching, inst)
		config, cfgErr := s.decryptConfig(inst.Config)
		if cfgErr != nil {
			continue
		}
		uc, configErr := normalizeEpusdtUnifiedConfig(config)
		if configErr != nil {
			return nil, infraerrors.ServiceUnavailable("USDT_CONFIG_INVALID", "an enabled Epusdt instance has invalid currency or bonusRate").WithCause(configErr)
		}
		if configCurrency == "" {
			configCurrency, configBonus = uc.Currency, uc.BonusRate
		} else if configCurrency != uc.Currency || configBonus != uc.BonusRate {
			return nil, infraerrors.Conflict("USDT_CONFIG_CONFLICT", "enabled USDT provider instances must use the same currency and bonus rate")
		}
		networks, networksErr := parseEpusdtNetworksStrict(config["networks"])
		if networksErr != nil {
			return nil, infraerrors.ServiceUnavailable("USDT_CONFIG_INVALID", "an enabled Epusdt instance has invalid networks").WithCause(networksErr)
		}
		for _, network := range networks {
			if _, exists := seen[network.Code]; !exists {
				seen[network.Code] = network
			}
		}
	}
	for _, network := range seen {
		result.Networks = append(result.Networks, network)
	}
	sort.Slice(result.Networks, func(i, j int) bool { return result.Networks[i].Code < result.Networks[j].Code })
	if configCurrency != "" {
		result.Currency, result.BonusRate = configCurrency, configBonus
	}
	if len(result.Networks) > 0 {
		rate, rateErr := s.usdtExchangeRate(ctx)
		if rateErr != nil {
			return nil, infraerrors.ServiceUnavailable("USDT_RATE_UNAVAILABLE", "USDT exchange rate is unavailable").WithCause(rateErr)
		}
		result.ExchangeRate = rate.Rate
		result.ExchangeRateSource = rate.Source
		result.ExchangeRateAt = rate.At.UTC().Format(time.RFC3339)
		result.ExchangeRateStale = rate.Stale
	}
	result.Enabled = len(result.Networks) > 0
	if len(matching) > 0 {
		limits := pcAggregateMethodLimits(payment.TypeUSDT, matching)
		if limits.SingleMin > result.MinAmount {
			result.MinAmount = limits.SingleMin
		}
		if limits.SingleMax > 0 && (result.MaxAmount == 0 || limits.SingleMax < result.MaxAmount) {
			result.MaxAmount = limits.SingleMax
		}
		if limits.DailyLimit > 0 && (result.DailyLimit == 0 || limits.DailyLimit < result.DailyLimit) {
			result.DailyLimit = limits.DailyLimit
		}
	}
	return result, nil
}

func parseEpusdtNetworks(raw string) []USDTNetwork {
	result, _ := parseEpusdtNetworksStrict(raw)
	return result
}

func parseEpusdtNetworksStrict(raw string) ([]USDTNetwork, error) {
	seen := map[string]struct{}{}
	result := make([]USDTNetwork, 0)
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		code := strings.ToLower(strings.TrimSpace(parts[0]))
		if code == "" {
			return nil, fmt.Errorf("Epusdt networks contain an empty network code")
		}
		if strings.ContainsAny(code, "\r\n,;&= ") {
			return nil, fmt.Errorf("Epusdt networks contain an invalid network code")
		}
		alias := ""
		if len(parts) == 2 {
			alias = strings.TrimSpace(parts[1])
			if strings.ContainsAny(alias, "\r\n,") {
				return nil, fmt.Errorf("Epusdt network aliases contain an invalid value")
			}
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, USDTNetwork{Code: code, Alias: alias, DisplayName: firstNonEmpty(alias, code)})
	}
	return result, nil
}

func normalizeEpusdtUnifiedConfig(config map[string]string) (epusdtUnifiedConfig, error) {
	currency := strings.ToUpper(strings.TrimSpace(config["currency"]))
	if currency == "" {
		currency = payment.DefaultPaymentCurrency
	}
	if currency != payment.DefaultPaymentCurrency && currency != "USDT" {
		return epusdtUnifiedConfig{}, fmt.Errorf("Epusdt currency must be CNY or USDT")
	}
	bonus := 0.0
	if raw := strings.TrimSpace(config["bonusRate"]); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 || math.Round(value*100) != value*100 {
			return epusdtUnifiedConfig{}, fmt.Errorf("Epusdt bonusRate must be between 0 and 100 with at most 2 decimals")
		}
		bonus = value
	}
	return epusdtUnifiedConfig{Currency: currency, BonusRate: bonus}, nil
}

func validateUSDTNetwork(network string, available []USDTNetwork) string {
	network = strings.ToLower(strings.TrimSpace(network))
	for _, item := range available {
		if strings.EqualFold(network, item.Code) {
			return item.Code
		}
	}
	return ""
}

func (s *PaymentConfigService) RequireUSDTNetwork(ctx context.Context, network string) (string, error) {
	info, err := s.GetUSDTCheckoutInfo(ctx)
	if err != nil {
		return "", err
	}
	network = validateUSDTNetwork(network, info.Networks)
	if !info.Enabled || network == "" {
		return "", infraerrors.ServiceUnavailable("USDT_UNAVAILABLE", "the selected USDT network is unavailable")
	}
	return network, nil
}
