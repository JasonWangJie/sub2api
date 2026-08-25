package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseEpusdtNetworksSupportsAliasesAndLegacyCodes(t *testing.T) {
	networks := parseEpusdtNetworks("TRON=TRC20,ethereum=ERC20,bsc,tron=ignored")
	require.Equal(t, []USDTNetwork{
		{Code: "tron", Alias: "TRC20", DisplayName: "TRC20"},
		{Code: "ethereum", Alias: "ERC20", DisplayName: "ERC20"},
		{Code: "bsc", DisplayName: "bsc"},
	}, networks)
	_, err := parseEpusdtNetworksStrict("tron,,ethereum")
	require.Error(t, err)
	_, err = parseEpusdtNetworksStrict("tron=bad\nalias")
	require.Error(t, err)
}

func TestNormalizeEpusdtUnifiedConfig(t *testing.T) {
	config, err := normalizeEpusdtUnifiedConfig(map[string]string{"currency": "usdt", "bonusRate": "1.25"})
	require.NoError(t, err)
	require.Equal(t, epusdtUnifiedConfig{Currency: "USDT", BonusRate: 1.25}, config)
	legacy, err := normalizeEpusdtUnifiedConfig(map[string]string{})
	require.NoError(t, err)
	require.Equal(t, epusdtUnifiedConfig{Currency: "CNY", BonusRate: 0}, legacy)
	cny, err := normalizeEpusdtUnifiedConfig(map[string]string{"currency": "cny"})
	require.NoError(t, err)
	require.Equal(t, epusdtUnifiedConfig{Currency: "CNY", BonusRate: 0}, cny)
	_, err = normalizeEpusdtUnifiedConfig(map[string]string{"currency": "EUR"})
	require.Error(t, err)
	_, err = normalizeEpusdtUnifiedConfig(map[string]string{"bonusRate": "1.234"})
	require.Error(t, err)
}

func TestUSDTCheckoutInfoExcludesGenericRechargeFee(t *testing.T) {
	t.Parallel()

	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient: client,
		settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingPaymentEnabled:  "true",
			SettingRechargeFeeRate: "2.5",
		}},
	}

	info, err := svc.GetUSDTCheckoutInfo(context.Background())
	require.NoError(t, err)
	require.Zero(t, info.FeeRate)
}

func TestUSDTExchangeRateServiceCachesAndUsesStaleValue(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"data":{"amount":"6.720838","base":"USDT","currency":"CNY"}}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	now := time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC)
	svc := &USDTExchangeRateService{client: server.Client(), endpoint: server.URL, now: func() time.Time { return now }}
	first, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, 6.720838, first.Rate)
	now = now.Add(30 * time.Second)
	fresh, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.False(t, fresh.Stale)
	require.Equal(t, first.Rate, fresh.Rate)
	require.Equal(t, 1, calls)
	now = now.Add(2 * time.Minute)
	second, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.True(t, second.Stale)
	require.Equal(t, first.Rate, second.Rate)
	require.Equal(t, 2, calls)
	now = now.Add(9 * time.Minute)
	_, err = svc.Get(context.Background())
	require.Error(t, err)
}

func TestUSDTQuoteExampleRoundsBonusAndBalanceAmount(t *testing.T) {
	rate := 6.720838
	cnyAmount := roundPaymentAmount(20 * rate)
	bonusAmount := roundPaymentAmount(cnyAmount * 1 / 100)
	creditedCNY := roundPaymentAmount(cnyAmount + bonusAmount)
	creditedUSD := calculateCreditedBalance(creditedCNY, 0.1)

	require.Equal(t, 134.42, cnyAmount)
	require.Equal(t, 1.34, bonusAmount)
	require.Equal(t, 135.76, creditedCNY)
	require.Equal(t, 13.58, creditedUSD)
}
