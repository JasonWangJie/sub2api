//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type paymentRechargeCompatUserRepo struct {
	UserRepository
	user *User
}

func (r *paymentRechargeCompatUserRepo) GetByID(_ context.Context, id int64) (*User, error) {
	if r.user.ID != id {
		return nil, fmt.Errorf("unexpected user ID: %d", id)
	}
	return r.user, nil
}

func newPaymentRechargeCompatService(t *testing.T, providerKey, paymentType string, providerConfig map[string]string, settings map[string]string) (*PaymentService, int64) {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("recharge-compat@example.com").
		SetPasswordHash("hash").
		SetUsername("recharge-compat").
		Save(ctx)
	require.NoError(t, err)
	configJSON, err := json.Marshal(providerConfig)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(providerKey).
		SetName("Recharge compatibility provider").
		SetConfig(string(configJSON)).
		SetSupportedTypes(paymentType).
		SetPaymentMode("popup").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	configService := &PaymentConfigService{
		entClient:   client,
		settingRepo: &paymentConfigSettingRepoStub{values: settings},
	}
	return &PaymentService{
		entClient:     client,
		configService: configService,
		loadBalancer:  payment.NewDefaultLoadBalancer(client, nil),
		userRepo: &paymentRechargeCompatUserRepo{user: &User{
			ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive,
		}},
	}, user.ID
}

func TestCreateOrderRechargeTiersPreserveMultiplierAndFee(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       string
		tiers      string
		wantAmount float64
		wantBonus  float64
		wantPay    float64
	}{
		{name: "no tiers", mode: "bonus", wantAmount: 10, wantBonus: 0, wantPay: 102.5},
		{name: "bonus", mode: "bonus", tiers: `[{"min_amount":100,"bonus_percent":20}]`, wantAmount: 12, wantBonus: 2, wantPay: 102.5},
		{name: "discount", mode: "discount", tiers: `[{"min_amount":100,"bonus_percent":20}]`, wantAmount: 10, wantBonus: 2, wantPay: 82},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, userID := newPaymentRechargeCompatService(t, payment.TypeEasyPay, payment.TypeAlipay, map[string]string{
				"pid": "test-merchant", "pkey": "test-key", "apiBase": "https://gateway.example.com",
				"notifyUrl": "https://app.example.com/notify", "returnUrl": "https://app.example.com/return",
			}, map[string]string{
				SettingPaymentEnabled: "true", SettingBalanceRechargeMult: "0.1", SettingRechargeFeeRate: "2.5",
				SettingRechargeBonusMode: tc.mode, SettingRechargeBonusTiers: tc.tiers,
			})
			resp, err := svc.CreateOrder(context.Background(), CreateOrderRequest{
				UserID: userID, Amount: 100, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance,
				SrcHost: "app.example.com",
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantAmount, resp.Amount)
			require.Equal(t, tc.wantBonus, resp.BonusAmount)
			require.Equal(t, tc.wantPay, resp.PayAmount)
			require.Equal(t, 2.5, resp.FeeRate)
			payURL, err := url.Parse(resp.PayURL)
			require.NoError(t, err)
			require.Equal(t, payment.FormatAmountForCurrency(tc.wantPay, "CNY"), payURL.Query().Get("money"))
			order, err := svc.entClient.PaymentOrder.Get(context.Background(), resp.OrderID)
			require.NoError(t, err)
			require.Equal(t, tc.wantAmount, order.Amount)
			require.Equal(t, tc.wantBonus, order.BonusAmount)
			require.Equal(t, tc.wantPay, order.PayAmount)
			require.Equal(t, tc.wantAmount-tc.wantBonus, affiliateRebateBaseAmount(order))
			require.Nil(t, PaymentOrderUSDTQuote(order))
		})
	}
}

func TestCreateUSDTOrderIgnoresOrdinaryRechargeTiers(t *testing.T) {
	for _, currency := range []string{"USDT", "CNY"} {
		for _, mode := range []string{"bonus", "discount"} {
			t.Run(currency+"/"+mode, func(t *testing.T) {
				gatewayAmounts := make(chan float64, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/payments/gmpay/v1/order/create-transaction" {
						t.Errorf("unexpected gateway path: %s", r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
						return
					}
					var body struct {
						Amount float64 `json:"amount"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode payment request: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					gatewayAmounts <- body.Amount
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"status_code":200,"data":{"trade_id":"test-trade","payment_url":"https://gateway.example.com/pay"}}`))
				}))
				defer server.Close()
				svc, userID := newPaymentRechargeCompatService(t, payment.TypeEpusdt, payment.TypeUSDT, map[string]string{
					"pid": "test-merchant", "secretKey": "test-key", "apiBase": server.URL,
					"notifyUrl": "https://app.example.com/notify", "returnUrl": "https://app.example.com/return",
					"networks": "tron=TRC20", "currency": currency, "bonusRate": "1",
				}, map[string]string{
					SettingPaymentEnabled: "true", SettingBalanceRechargeMult: "0.1", SettingRechargeFeeRate: "2.5",
					SettingRechargeBonusMode: mode, SettingRechargeBonusTiers: `[{"min_amount":0,"bonus_percent":20}]`,
				})
				now := time.Now()
				svc.configService.usdtRate = &USDTExchangeRateService{
					hasValue: true, now: func() time.Time { return now },
					current: USDTExchangeRate{Rate: 6.67, Source: usdtRateSourceOKXC2C, At: now},
				}
				resp, err := svc.CreateUSDTOrder(context.Background(), CreateOrderRequest{
					UserID: userID, Amount: 20, Network: "tron", SrcHost: "app.example.com",
				})
				require.NoError(t, err)
				require.Equal(t, 20.0, resp.PayAmount)
				require.Equal(t, 20.0, <-gatewayAmounts)
				require.Zero(t, resp.FeeRate)
				require.Zero(t, resp.BonusAmount)
				require.NotNil(t, resp.USDTQuote)
				require.Equal(t, 6.67, resp.USDTQuote.ExchangeRate)
				require.Equal(t, 1.0, resp.USDTQuote.BonusRate)
				if currency == "USDT" {
					require.Equal(t, 13.47, resp.Amount)
					require.Equal(t, 133.4, resp.USDTQuote.CNYAmount)
					require.Equal(t, 1.33, resp.USDTQuote.BonusAmount)
				} else {
					require.Equal(t, 2.02, resp.Amount)
					require.Equal(t, 20.0, resp.USDTQuote.CNYAmount)
					require.Equal(t, 0.2, resp.USDTQuote.BonusAmount)
				}
				order, err := svc.entClient.PaymentOrder.Get(context.Background(), resp.OrderID)
				require.NoError(t, err)
				require.Equal(t, resp.Amount, order.Amount)
				require.Zero(t, order.BonusAmount)
				require.Equal(t, order.Amount, affiliateRebateBaseAmount(order))
				require.Equal(t, resp.USDTQuote, PaymentOrderUSDTQuote(order))
			})
		}
	}
}
