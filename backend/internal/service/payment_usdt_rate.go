package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// OKX C2C express: side=buy is the user buy price (pay CNY for USDT).
	defaultOKXC2CExpressURL = "https://www.okx.com/v4/c2c/express/price?crypto=USDT&fiat=CNY&side=buy"
	// OKX C2C books: side=sell means merchant sells USDT (user buy price). Used as fallback.
	defaultOKXC2CBooksURL  = "https://www.okx.com/v3/c2c/tradingOrders/books?quoteCurrency=CNY&baseCurrency=USDT&side=sell&paymentMethod=all&userType=all&showTrade=false&receivingAds=false&showFollow=false&showAlreadyTraded=false&isAbleFilter=false"
	usdtRateSourceOKXC2C   = "OKX C2C"
	usdtRateFreshFor       = time.Minute
	usdtRateStaleFor       = 10 * time.Minute
	usdtRateRetryFor       = 15 * time.Second
	usdtRateRequestTimeout = 6 * time.Second
	usdtRateRefreshTimeout = 10 * time.Second
	// Fail closed on clearly implausible CNY/USDT quotes. A recent valid quote
	// also limits sudden source-side jumps before they can affect user balances.
	minUSDTExchangeRate         = 4.0
	maxUSDTExchangeRate         = 12.0
	maxUSDTExchangeRateChange   = 0.05
	maxUSDTExchangeResponseSize = 1 << 20
	okxHTTPUserAgent            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36"
)

// USDTExchangeRate is a server-side quote returned to the checkout page.
type USDTExchangeRate struct {
	Rate   float64   `json:"exchange_rate"`
	Source string    `json:"exchange_rate_source"`
	At     time.Time `json:"exchange_rate_at"`
	Stale  bool      `json:"exchange_rate_stale"`
}

type okxC2CExpressResponse struct {
	Code         json.Number `json:"code"`
	ErrorCode    string      `json:"error_code"`
	ErrorMessage string      `json:"error_message"`
	Msg          string      `json:"msg"`
	Data         struct {
		Price string `json:"price"`
	} `json:"data"`
}

type okxC2CTradingOrdersResponse struct {
	Code json.Number `json:"code"`
	Data struct {
		Sell []okxC2COrder `json:"sell"`
	} `json:"data"`
	Msg string `json:"msg"`
}

type okxC2COrder struct {
	Price         string `json:"price"`
	Side          string `json:"side"`
	BaseCurrency  string `json:"baseCurrency"`
	QuoteCurrency string `json:"quoteCurrency"`
}

// USDTExchangeRateService fetches and caches the OKX C2C USDT/CNY buy rate.
// The fields are intentionally injectable in tests without changing the public API.
type USDTExchangeRateService struct {
	client         *http.Client
	endpoint       string // primary (express); empty = default
	fallbackURL    string // books; empty = default
	now            func() time.Time
	refreshTimeout time.Duration
	refresh        singleflight.Group

	mu                 sync.Mutex
	current            USDTExchangeRate
	hasValue           bool
	lastRefreshAttempt time.Time
}

func NewUSDTExchangeRateService() *USDTExchangeRateService {
	return &USDTExchangeRateService{
		client:         &http.Client{Timeout: usdtRateRequestTimeout},
		endpoint:       defaultOKXC2CExpressURL,
		fallbackURL:    defaultOKXC2CBooksURL,
		now:            time.Now,
		refreshTimeout: usdtRateRefreshTimeout,
	}
}

func (s *USDTExchangeRateService) Get(ctx context.Context) (USDTExchangeRate, error) {
	if s == nil {
		return USDTExchangeRate{}, fmt.Errorf("USDT exchange rate service is unavailable")
	}
	now := s.currentTime()

	s.mu.Lock()
	if s.hasValue && usdtRateAge(now, s.current.At) < usdtRateFreshFor {
		value := s.current
		s.mu.Unlock()
		return value, nil
	}
	previous := s.current
	hasPrevious := s.hasValue
	lastRefreshAttempt := s.lastRefreshAttempt
	s.mu.Unlock()

	staleUsable := hasPrevious && usdtRateAge(now, previous.At) <= usdtRateStaleFor
	shouldRefresh := !staleUsable || lastRefreshAttempt.IsZero() || usdtRateAge(now, lastRefreshAttempt) >= usdtRateRetryFor
	var result <-chan singleflight.Result
	if shouldRefresh {
		result = s.refresh.DoChan("USDT/CNY", func() (any, error) {
			return s.refreshRate()
		})
	}
	if staleUsable {
		previous.Stale = true
		return previous, nil
	}

	select {
	case <-ctx.Done():
		return USDTExchangeRate{}, fmt.Errorf("USDT exchange rate unavailable: %w", ctx.Err())
	case refreshed := <-result:
		if refreshed.Err != nil {
			return USDTExchangeRate{}, fmt.Errorf("USDT exchange rate unavailable: %w", refreshed.Err)
		}
		value, ok := refreshed.Val.(USDTExchangeRate)
		if !ok {
			return USDTExchangeRate{}, fmt.Errorf("USDT exchange rate unavailable: invalid refresh result")
		}
		return value, nil
	}
}

func (s *USDTExchangeRateService) refreshRate() (USDTExchangeRate, error) {
	now := s.currentTime()
	s.mu.Lock()
	if s.hasValue && usdtRateAge(now, s.current.At) < usdtRateFreshFor {
		value := s.current
		s.mu.Unlock()
		return value, nil
	}
	client := s.client
	endpoint := s.endpoint
	fallback := s.fallbackURL
	previous := s.current
	hasPrevious := s.hasValue
	refreshTimeout := s.refreshTimeout
	s.lastRefreshAttempt = now
	s.mu.Unlock()

	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultOKXC2CExpressURL
	}
	if refreshTimeout <= 0 {
		refreshTimeout = usdtRateRefreshTimeout
	}
	refreshCtx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	rate, err := fetchOKXC2CUSDTBuy(refreshCtx, client, endpoint, fallback)
	if err != nil {
		return USDTExchangeRate{}, err
	}
	if hasPrevious && usdtRateAge(now, previous.At) <= usdtRateStaleFor {
		if err := validateUSDTExchangeRateChange(rate, previous.Rate); err != nil {
			return USDTExchangeRate{}, err
		}
	}

	value := USDTExchangeRate{Rate: rate, Source: usdtRateSourceOKXC2C, At: s.currentTime()}
	s.mu.Lock()
	s.current, s.hasValue = value, true
	s.mu.Unlock()
	return value, nil
}

func (s *USDTExchangeRateService) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func usdtRateAge(now, at time.Time) time.Duration {
	age := now.Sub(at)
	if age < 0 {
		return 0
	}
	return age
}

func fetchOKXC2CUSDTBuy(ctx context.Context, client *http.Client, endpoint, fallback string) (float64, error) {
	rate, err := fetchOKXC2CExpressPrice(ctx, client, endpoint)
	if err == nil {
		return rate, nil
	}
	primaryErr := err

	if strings.TrimSpace(fallback) == "" {
		fallback = defaultOKXC2CBooksURL
	}
	// When tests inject a single httptest endpoint, skip the real OKX fallback.
	if fallback == endpoint {
		return 0, primaryErr
	}
	rate, err = fetchOKXC2CBooksBuy(ctx, client, fallback)
	if err == nil {
		return rate, nil
	}
	return 0, fmt.Errorf("express: %v; books: %w", primaryErr, err)
}

func fetchOKXC2CExpressPrice(ctx context.Context, client *http.Client, endpoint string) (float64, error) {
	body, err := getOKXJSON(ctx, client, endpoint)
	if err != nil {
		return 0, err
	}
	var payload okxC2CExpressResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("invalid OKX C2C express response: %w", err)
	}
	if !okxResponseOK(payload.Code, payload.ErrorCode) {
		msg := firstNonEmpty(strings.TrimSpace(payload.ErrorMessage), strings.TrimSpace(payload.Msg), "unknown error")
		return 0, fmt.Errorf("OKX C2C express error: %s", msg)
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(payload.Data.Price), 64)
	if err != nil {
		return 0, fmt.Errorf("OKX C2C express returned an invalid USDT/CNY price")
	}
	if err := validateUSDTExchangeRate(rate); err != nil {
		return 0, fmt.Errorf("OKX C2C express returned an invalid USDT/CNY price: %w", err)
	}
	return rate, nil
}

func fetchOKXC2CBooksBuy(ctx context.Context, client *http.Client, endpoint string) (float64, error) {
	body, err := getOKXJSON(ctx, client, endpoint)
	if err != nil {
		return 0, err
	}
	var payload okxC2CTradingOrdersResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("invalid OKX C2C books response: %w", err)
	}
	if !okxResponseOK(payload.Code, "") {
		msg := strings.TrimSpace(payload.Msg)
		if msg == "" {
			msg = "unknown error"
		}
		return 0, fmt.Errorf("OKX C2C books error: %s", msg)
	}
	if len(payload.Data.Sell) == 0 {
		return 0, fmt.Errorf("OKX C2C books returned no merchant sell orders")
	}
	return bestOKXC2CBuyPrice(payload.Data.Sell)
}

func getOKXJSON(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", okxHTTPUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OKX returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUSDTExchangeResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxUSDTExchangeResponseSize {
		return nil, fmt.Errorf("OKX response exceeds %d bytes", maxUSDTExchangeResponseSize)
	}
	return body, nil
}

func okxResponseOK(code json.Number, errorCode string) bool {
	errorCode = strings.TrimSpace(errorCode)
	if errorCode != "" && errorCode != "0" {
		return false
	}
	raw := strings.TrimSpace(code.String())
	return raw == "0"
}

// bestOKXC2CBuyPrice picks the lowest valid merchant sell quote (cheapest user USDT buy).
func bestOKXC2CBuyPrice(orders []okxC2COrder) (float64, error) {
	best := 0.0
	for _, order := range orders {
		if !strings.EqualFold(strings.TrimSpace(order.Side), "sell") ||
			!strings.EqualFold(strings.TrimSpace(order.BaseCurrency), "USDT") ||
			!strings.EqualFold(strings.TrimSpace(order.QuoteCurrency), "CNY") {
			continue
		}
		rate, err := strconv.ParseFloat(strings.TrimSpace(order.Price), 64)
		if err != nil || validateUSDTExchangeRate(rate) != nil {
			continue
		}
		if best == 0 || rate < best {
			best = rate
		}
	}
	if best <= 0 {
		return 0, fmt.Errorf("OKX C2C returned no valid USDT/CNY buy price")
	}
	return best, nil
}

func validateUSDTExchangeRate(rate float64) error {
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return fmt.Errorf("rate must be finite")
	}
	if rate < minUSDTExchangeRate || rate > maxUSDTExchangeRate {
		return fmt.Errorf("rate %.6f is outside the allowed range %.2f-%.2f", rate, minUSDTExchangeRate, maxUSDTExchangeRate)
	}
	return nil
}

func validateUSDTExchangeRateChange(rate, previous float64) error {
	if err := validateUSDTExchangeRate(rate); err != nil {
		return err
	}
	if err := validateUSDTExchangeRate(previous); err != nil {
		return nil
	}
	change := math.Abs(rate-previous) / previous
	if change > maxUSDTExchangeRateChange {
		return fmt.Errorf("rate changed %.2f%% from the previous quote, exceeding the %.2f%% limit", change*100, maxUSDTExchangeRateChange*100)
	}
	return nil
}
