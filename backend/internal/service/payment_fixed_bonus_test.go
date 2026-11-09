//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func mustQuoteRechargeBonus(t *testing.T, cfg *PaymentConfig, amount float64, currency string) rechargeBonusQuote {
	t.Helper()
	quote, err := quoteRechargeBonus(cfg, amount, currency)
	require.NoError(t, err)
	return quote
}

func TestQuoteRechargeFixedBonus(t *testing.T) {
	tests := []struct {
		name       string
		amount     float64
		multiplier string
		fixed      string
		tiers      string
		enabled    string
		mode       string
		want       rechargeBonusQuote
	}{
		{
			name: "below threshold", amount: 99.99, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
			want:  rechargeBonusQuote{PayBase: 99.99, Credited: 14},
		},
		{
			name: "threshold inclusive", amount: 100, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
			want:  rechargeBonusQuote{PayBase: 100, Credited: 15.4, Bonus: 1.4},
		},
		{
			name: "complete multiples only", amount: 299.99, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
			want:  rechargeBonusQuote{PayBase: 299.99, Credited: 44.8, Bonus: 2.8},
		},
		{
			name: "decimal threshold exact multiple", amount: 0.3, multiplier: "1",
			fixed: `{"enabled":true,"threshold_amount":0.1,"bonus_amount":0.01}`,
			want:  rechargeBonusQuote{PayBase: 0.3, Credited: 0.33, Bonus: 0.03},
		},
		{
			name: "convert entire fixed gift before rounding", amount: 3, multiplier: "0.5",
			fixed: `{"enabled":true,"threshold_amount":1,"bonus_amount":0.01}`,
			want:  rechargeBonusQuote{PayBase: 3, Credited: 1.52, Bonus: 0.02},
		},
		{
			name: "fraction of a multiple cannot round up", amount: 99.99999999999999, multiplier: "1",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
			want:  rechargeBonusQuote{PayBase: 99.99999999999999, Credited: 100},
		},
		{
			name: "fixed wins", amount: 100, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
			tiers: `[{"min_amount":100,"bonus_percent":5}]`,
			want:  rechargeBonusQuote{PayBase: 100, Credited: 15.4, Bonus: 1.4},
		},
		{
			name: "percentage wins", amount: 100, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
			tiers: `[{"min_amount":100,"bonus_percent":20}]`,
			want:  rechargeBonusQuote{PayBase: 100, Credited: 16.8, Bonus: 2.8, Percent: 20},
		},
		{
			name: "percentage wins on a rounded tie", amount: 100, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10.01}`,
			tiers: `[{"min_amount":100,"bonus_percent":10}]`,
			want:  rechargeBonusQuote{PayBase: 100, Credited: 15.4, Bonus: 1.4, Percent: 10},
		},
		{
			name: "explicitly disabled tiers stay disabled", amount: 100, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
			tiers: `[{"min_amount":100,"bonus_percent":20}]`, enabled: "false", mode: "discount",
			want: rechargeBonusQuote{PayBase: 100, Credited: 15.4, Bonus: 1.4},
		},
		{
			name: "discount takes precedence in inconsistent stored settings", amount: 100, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":30}`,
			tiers: `[{"min_amount":100,"bonus_percent":20}]`, mode: "discount",
			want: rechargeBonusQuote{PayBase: 80, Credited: 14, Bonus: 2.8, Percent: 20},
		},
		{
			name: "fixed remains suppressed below an active discount threshold", amount: 100, multiplier: "0.14",
			fixed: `{"enabled":true,"threshold_amount":100,"bonus_amount":30}`,
			tiers: `[{"min_amount":200,"bonus_percent":20}]`, mode: "discount",
			want: rechargeBonusQuote{PayBase: 100, Credited: 14},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{
				SettingBalanceRechargeMult: tc.multiplier, SettingRechargeBonusTiers: tc.tiers,
				SettingRechargeBonusMode: tc.mode, "RECHARGE_FIXED_BONUS": tc.fixed,
			}
			if tc.enabled != "" {
				values["RECHARGE_BONUS_ENABLED"] = tc.enabled
			}
			cfg := (&PaymentConfigService{}).parsePaymentConfig(values)
			require.Equal(t, tc.want, mustQuoteRechargeBonus(t, cfg, tc.amount, "CNY"))
		})
	}
}

func TestUpdatePaymentConfigFixedBonus(t *testing.T) {
	ctx := context.Background()
	update := func(t *testing.T, svc *PaymentConfigService, body string) error {
		t.Helper()
		var req UpdatePaymentConfigRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		return svc.UpdatePaymentConfig(ctx, req)
	}

	t.Run("persists both switches and preserves omitted fields", func(t *testing.T) {
		tiers := `[{"min_amount":100,"bonus_percent":20}]`
		repo := &paymentConfigSettingRepoStub{values: map[string]string{SettingRechargeBonusTiers: tiers}}
		svc := &PaymentConfigService{settingRepo: repo}
		require.NoError(t, update(t, svc, `{"recharge_bonus_enabled":false,"recharge_fixed_bonus":{"enabled":true,"threshold_amount":100.25,"bonus_amount":10.05}}`))
		require.Equal(t, map[string]string{
			"RECHARGE_BONUS_ENABLED": "false",
			"RECHARGE_FIXED_BONUS":   `{"enabled":true,"threshold_amount":100.25,"bonus_amount":10.05}`,
		}, repo.updates)
		require.NoError(t, update(t, svc, `{"recharge_fee_rate":2.5}`))
		require.Equal(t, map[string]string{SettingRechargeFeeRate: "2.50"}, repo.updates)
		require.Equal(t, tiers, repo.values[SettingRechargeBonusTiers])
		require.NoError(t, update(t, svc, `{"recharge_fixed_bonus":{"enabled":false}}`))
		require.Equal(t, `{"enabled":false,"threshold_amount":100.25,"bonus_amount":10.05}`, repo.values["RECHARGE_FIXED_BONUS"])
	})

	t.Run("allows an unconfigured disabled rule", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{}
		svc := &PaymentConfigService{settingRepo: repo}
		require.NoError(t, update(t, svc, `{"recharge_fixed_bonus":{"enabled":false,"threshold_amount":0,"bonus_amount":0}}`))
		require.JSONEq(t, `{"enabled":false,"threshold_amount":0,"bonus_amount":0}`, repo.values["RECHARGE_FIXED_BONUS"])
	})

	t.Run("rejects enabled zero negative and overprecision amounts", func(t *testing.T) {
		for _, body := range []string{
			`{"recharge_fixed_bonus":{"enabled":true,"threshold_amount":0,"bonus_amount":1}}`,
			`{"recharge_fixed_bonus":{"enabled":true,"threshold_amount":10,"bonus_amount":-1}}`,
			`{"recharge_fixed_bonus":{"enabled":true,"threshold_amount":10.001,"bonus_amount":1}}`,
			`{"recharge_fixed_bonus":{"enabled":true,"threshold_amount":10,"bonus_amount":0.101}}`,
		} {
			repo := &paymentConfigSettingRepoStub{}
			svc := &PaymentConfigService{settingRepo: repo}
			err := update(t, svc, body)
			require.Equal(t, "INVALID_RECHARGE_FIXED_BONUS", infraerrors.Reason(err), body)
			require.Nil(t, repo.updates)
		}
	})

	t.Run("rejects a conflict with inferred legacy discount", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{
			SettingRechargeBonusMode: RechargeBonusModeDiscount, SettingRechargeBonusTiers: `[{"min_amount":100,"bonus_percent":10}]`,
		}}
		svc := &PaymentConfigService{settingRepo: repo}
		body := `{"recharge_fee_rate":2.5,"recharge_fixed_bonus":{"enabled":true,"threshold_amount":100,"bonus_amount":10}}`
		err := update(t, svc, body)
		require.Equal(t, "RECHARGE_BONUS_MODE_CONFLICT", infraerrors.Reason(err))
		require.Nil(t, repo.updates)
		require.NoError(t, update(t, svc, `{"recharge_bonus_enabled":false,"recharge_fixed_bonus":{"enabled":true,"threshold_amount":100,"bonus_amount":10}}`))
		err = update(t, svc, `{"recharge_bonus_enabled":true}`)
		require.Equal(t, "RECHARGE_BONUS_MODE_CONFLICT", infraerrors.Reason(err))
		require.Equal(t, "false", repo.values["RECHARGE_BONUS_ENABLED"])
	})
}

func TestQuoteRechargeFixedBonusPublishedExamples(t *testing.T) {
	cfg := (&PaymentConfigService{}).parsePaymentConfig(map[string]string{
		SettingRechargeBonusTiers: `[{"min_amount":100,"bonus_percent":15}]`,
		"RECHARGE_FIXED_BONUS":    `{"enabled":true,"threshold_amount":500,"bonus_amount":200}`,
	})
	for _, tc := range []struct {
		pay, credit float64
	}{
		{99, 99},
		{100, 115},
		{500, 700},
		{900, 1100},
		{1000, 1400},
	} {
		quote := mustQuoteRechargeBonus(t, cfg, tc.pay, "CNY")
		require.Equal(t, tc.pay, quote.PayBase)
		require.Equal(t, tc.credit, quote.Credited, "payment %v", tc.pay)
	}
}

func TestCreateRechargeOrderRejectsUnrepresentableFixedBonus(t *testing.T) {
	tests := map[string]string{
		"bonus overflows float64": `{"enabled":true,"threshold_amount":0.01,"bonus_amount":1e308}`,
		"total overflows balance": `{"enabled":true,"threshold_amount":100,"bonus_amount":999999999950}`,
	}
	for name, fixed := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			svc, userID := newPaymentRechargeCompatService(t, payment.TypeEasyPay, payment.TypeAlipay, map[string]string{
				"pid": "test-merchant", "pkey": "test-key", "apiBase": "https://gateway.example.com",
				"notifyUrl": "https://app.example.com/notify", "returnUrl": "https://app.example.com/return",
			}, map[string]string{
				SettingPaymentEnabled:  "true",
				"RECHARGE_FIXED_BONUS": fixed,
			})
			_, err := svc.CreateOrder(ctx, CreateOrderRequest{
				UserID: userID, Amount: 100, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance,
				SrcHost: "app.example.com",
			})
			require.Equal(t, "INVALID_RECHARGE_BONUS_AMOUNT", infraerrors.Reason(err))
			count, err := svc.entClient.PaymentOrder.Query().Count(ctx)
			require.NoError(t, err)
			require.Zero(t, count)
		})
	}
}

func TestCreateAndFulfillRechargeOrderWithFrozenFixedBonus(t *testing.T) {
	ctx := context.Background()
	svc, userID := newPaymentRechargeCompatService(t, payment.TypeEasyPay, payment.TypeAlipay, map[string]string{
		"pid": "test-merchant", "pkey": "test-key", "apiBase": "https://gateway.example.com",
		"notifyUrl": "https://app.example.com/notify", "returnUrl": "https://app.example.com/return",
	}, map[string]string{
		SettingPaymentEnabled: "true", SettingBalanceRechargeMult: "0.14", SettingRechargeFeeRate: "2.5",
		"RECHARGE_FIXED_BONUS": `{"enabled":true,"threshold_amount":100,"bonus_amount":10}`,
	})
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, svc.entClient)
	req := CreateOrderRequest{
		UserID: userID, Amount: 99, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance,
		SrcHost: "app.example.com",
	}
	below, err := svc.CreateOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 101.48, below.PayAmount)
	require.Zero(t, below.BonusAmount, "fees must not qualify an otherwise below-threshold order")

	req.Amount = 250
	resp, err := svc.CreateOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 256.25, resp.PayAmount)
	require.Equal(t, 37.8, resp.Amount)
	require.Equal(t, 2.8, resp.BonusAmount)
	order, err := svc.entClient.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, 256.25, order.PayAmount)
	require.Equal(t, 37.8, order.Amount)
	require.Equal(t, 2.8, order.BonusAmount)
	require.Equal(t, 35.0, affiliateRebateBaseAmount(order))

	var update UpdatePaymentConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{"recharge_fixed_bonus":{"enabled":false,"threshold_amount":100,"bonus_amount":10}}`), &update))
	require.NoError(t, svc.configService.UpdatePaymentConfig(ctx, update))
	newOrder, err := svc.CreateOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 35.0, newOrder.Amount)
	require.Zero(t, newOrder.BonusAmount)
	redeemRepo := &paymentFulfillmentRedeemRepo{}
	userRepo := &mockUserRepo{getByIDUser: &User{ID: userID}}
	credited := 0.0
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, userID, id)
		credited += amount
		return nil
	}
	svc.redeemService = NewRedeemService(redeemRepo, userRepo, nil, nil, nil, svc.entClient, nil, nil)
	notification := &payment.PaymentNotification{
		TradeNo: "easypay-fixed-bonus", OrderID: order.OutTradeNo, Amount: 256.25, Status: payment.NotificationStatusSuccess,
	}
	require.NoError(t, svc.HandlePaymentNotification(ctx, notification, payment.TypeEasyPay))
	require.NoError(t, svc.HandlePaymentNotification(ctx, notification, payment.TypeEasyPay))
	order, err = svc.entClient.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, order.Status)
	require.Equal(t, 37.8, credited)
	require.Len(t, redeemRepo.useCalls, 1)
}
