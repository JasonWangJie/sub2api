package service

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
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
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"code":0,"data":{"price":"6.67"},"error_code":"0","msg":""}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	now := time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC)
	svc := &USDTExchangeRateService{
		client:      server.Client(),
		endpoint:    server.URL,
		fallbackURL: server.URL, // same as endpoint → skip live OKX fallback in unit tests
		now:         func() time.Time { return now },
	}
	first, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, 6.67, first.Rate)
	require.Equal(t, usdtRateSourceOKXC2C, first.Source)
	now = now.Add(30 * time.Second)
	fresh, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.False(t, fresh.Stale)
	require.Equal(t, first.Rate, fresh.Rate)
	require.Equal(t, int32(1), calls.Load())
	now = now.Add(2 * time.Minute)
	second, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.True(t, second.Stale)
	require.Equal(t, first.Rate, second.Rate)
	require.Eventually(t, func() bool { return calls.Load() >= 2 }, time.Second, 5*time.Millisecond)
	again, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.True(t, again.Stale)
	require.Never(t, func() bool { return calls.Load() > 2 }, 50*time.Millisecond, 5*time.Millisecond)
	now = now.Add(9 * time.Minute)
	_, err = svc.Get(context.Background())
	require.Error(t, err)
}

func TestBestOKXC2CBuyPricePicksLowestValid(t *testing.T) {
	rate, err := bestOKXC2CBuyPrice([]okxC2COrder{
		okxMerchantSellOrder("6.69"),
		okxMerchantSellOrder("0"),
		okxMerchantSellOrder("bad"),
		okxMerchantSellOrder("NaN"),
		okxMerchantSellOrder("+Inf"),
		okxMerchantSellOrder("12.01"),
		okxMerchantSellOrder("6.67"),
		okxMerchantSellOrder("6.68"),
		{Price: "1.01", Side: "buy", BaseCurrency: "USDT", QuoteCurrency: "CNY"},
		{Price: "1.01", Side: "sell", BaseCurrency: "BTC", QuoteCurrency: "CNY"},
	})
	require.NoError(t, err)
	require.Equal(t, 6.67, rate)

	_, err = bestOKXC2CBuyPrice([]okxC2COrder{okxMerchantSellOrder("0"), okxMerchantSellOrder("x")})
	require.Error(t, err)
}

func TestValidateUSDTExchangeRateRejectsUnsafeValues(t *testing.T) {
	for _, rate := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, 3.99, 12.01} {
		require.Error(t, validateUSDTExchangeRate(rate), "rate=%v", rate)
	}
	for _, rate := range []float64{4, 6.67, 12} {
		require.NoError(t, validateUSDTExchangeRate(rate), "rate=%v", rate)
	}
	require.NoError(t, validateUSDTExchangeRateChange(7.0, 6.67))
	require.Error(t, validateUSDTExchangeRateChange(7.1, 6.67))
}

func TestFetchOKXC2CBooksRejectsOppositeSide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"sell":[],"buy":[{"price":"6.60","side":"buy","baseCurrency":"usdt","quoteCurrency":"cny"}]}}`))
	}))
	defer server.Close()

	_, err := fetchOKXC2CBooksBuy(context.Background(), server.Client(), server.URL)
	require.ErrorContains(t, err, "no merchant sell orders")
}

func TestFetchOKXC2CUSDTBuyFallsBackToVerifiedSellBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/express":
			w.WriteHeader(http.StatusBadGateway)
		case "/books":
			_, _ = w.Write([]byte(`{"code":0,"data":{"sell":[{"price":"6.68","side":"sell","baseCurrency":"usdt","quoteCurrency":"cny"}],"buy":[{"price":"1.01","side":"buy","baseCurrency":"usdt","quoteCurrency":"cny"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	rate, err := fetchOKXC2CUSDTBuy(context.Background(), server.Client(), server.URL+"/express", server.URL+"/books")
	require.NoError(t, err)
	require.Equal(t, 6.68, rate)
}

func TestUSDTExchangeRateServiceCapsWholeRefreshAndCoalescesCallers(t *testing.T) {
	t.Run("whole refresh timeout", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.URL.Path == "/express" {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			<-r.Context().Done()
		}))
		defer server.Close()

		svc := &USDTExchangeRateService{
			client:         server.Client(),
			endpoint:       server.URL + "/express",
			fallbackURL:    server.URL + "/books",
			refreshTimeout: 50 * time.Millisecond,
		}
		started := time.Now()
		_, err := svc.Get(context.Background())
		require.Error(t, err)
		require.Less(t, time.Since(started), 500*time.Millisecond)
		require.Equal(t, int32(2), calls.Load())
	})

	t.Run("concurrent callers share one refresh", func(t *testing.T) {
		var calls atomic.Int32
		entered := make(chan struct{})
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				close(entered)
			}
			<-release
			_, _ = w.Write([]byte(`{"code":0,"data":{"price":"6.67"},"error_code":"0"}`))
		}))
		defer server.Close()

		svc := &USDTExchangeRateService{
			client:         server.Client(),
			endpoint:       server.URL,
			fallbackURL:    server.URL,
			refreshTimeout: time.Second,
		}
		const workers = 8
		start := make(chan struct{})
		results := make(chan error, workers)
		var ready sync.WaitGroup
		ready.Add(workers)
		for range workers {
			go func() {
				ready.Done()
				<-start
				_, err := svc.Get(context.Background())
				results <- err
			}()
		}
		ready.Wait()
		close(start)
		<-entered
		close(release)
		for range workers {
			require.NoError(t, <-results)
		}
		require.Equal(t, int32(1), calls.Load())
	})
}

func okxMerchantSellOrder(price string) okxC2COrder {
	return okxC2COrder{Price: price, Side: "sell", BaseCurrency: "USDT", QuoteCurrency: "CNY"}
}

func TestLiveOKXC2CUSDTBuy(t *testing.T) {
	if os.Getenv("LIVE_OKX_C2C") == "" {
		t.Skip("set LIVE_OKX_C2C=1 to hit OKX")
	}
	svc := NewUSDTExchangeRateService()
	rate, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, rate.Rate, minUSDTExchangeRate)
	require.LessOrEqual(t, rate.Rate, maxUSDTExchangeRate)
	require.Equal(t, usdtRateSourceOKXC2C, rate.Source)
	t.Logf("OKX C2C buy rate=%.6f at=%s", rate.Rate, rate.At.Format(time.RFC3339))
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
