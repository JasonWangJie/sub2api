package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// USDTCheckoutInfo is intentionally separate from the ordinary checkout
// response so the purchase page can never discover the USDT method by accident.
type USDTCheckoutInfo struct {
	Enabled                   bool     `json:"enabled"`
	Currency                  string   `json:"currency"`
	MinAmount                 float64  `json:"min_amount"`
	MaxAmount                 float64  `json:"max_amount"`
	DailyLimit                float64  `json:"daily_limit"`
	FeeRate                   float64  `json:"fee_rate"`
	BalanceRechargeMultiplier float64  `json:"balance_recharge_multiplier"`
	Networks                  []string `json:"networks"`
}

func (s *PaymentConfigService) GetUSDTCheckoutInfo(ctx context.Context) (*USDTCheckoutInfo, error) {
	cfg, err := s.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	result := &USDTCheckoutInfo{
		Currency: payment.DefaultPaymentCurrency, MinAmount: cfg.MinAmount, MaxAmount: cfg.MaxAmount,
		DailyLimit: cfg.DailyLimit, FeeRate: cfg.RechargeFeeRate,
		BalanceRechargeMultiplier: cfg.BalanceRechargeMultiplier,
		Networks:                  []string{},
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
	seen := map[string]struct{}{}
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
		for _, raw := range strings.Split(config["networks"], ",") {
			network := strings.ToLower(strings.TrimSpace(raw))
			if network != "" {
				seen[network] = struct{}{}
			}
		}
	}
	for network := range seen {
		result.Networks = append(result.Networks, network)
	}
	sort.Strings(result.Networks)
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

func validateUSDTNetwork(network string, available []string) string {
	network = strings.ToLower(strings.TrimSpace(network))
	for _, item := range available {
		if strings.EqualFold(network, item) {
			return strings.ToLower(strings.TrimSpace(item))
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
