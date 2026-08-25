package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultCoinbaseUSDTSpotURL  = "https://api.coinbase.com/v2/prices/USDT-CNY/spot"
	usdtRateFreshFor            = time.Minute
	usdtRateStaleFor            = 10 * time.Minute
	maxUSDTExchangeResponseSize = 64 << 10
)

// USDTExchangeRate is a server-side quote returned to the checkout page.
type USDTExchangeRate struct {
	Rate   float64   `json:"exchange_rate"`
	Source string    `json:"exchange_rate_source"`
	At     time.Time `json:"exchange_rate_at"`
	Stale  bool      `json:"exchange_rate_stale"`
}

type coinbaseUSDTSpotResponse struct {
	Data struct {
		Amount   string `json:"amount"`
		Base     string `json:"base"`
		Currency string `json:"currency"`
	} `json:"data"`
}

// USDTExchangeRateService fetches and caches the Coinbase USDT/CNY spot rate.
// The fields are intentionally injectable in tests without changing the public API.
type USDTExchangeRateService struct {
	client   *http.Client
	endpoint string
	now      func() time.Time

	mu       sync.Mutex
	current  USDTExchangeRate
	hasValue bool
}

func NewUSDTExchangeRateService() *USDTExchangeRateService {
	return &USDTExchangeRateService{
		client:   &http.Client{Timeout: 10 * time.Second},
		endpoint: defaultCoinbaseUSDTSpotURL,
		now:      time.Now,
	}
}

func (s *USDTExchangeRateService) Get(ctx context.Context) (USDTExchangeRate, error) {
	if s == nil {
		return USDTExchangeRate{}, fmt.Errorf("USDT exchange rate service is unavailable")
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}

	s.mu.Lock()
	if s.hasValue && now.Sub(s.current.At) < usdtRateFreshFor {
		value := s.current
		s.mu.Unlock()
		return value, nil
	}
	client := s.client
	endpoint := s.endpoint
	previous := s.current
	hasPrevious := s.hasValue
	s.mu.Unlock()

	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultCoinbaseUSDTSpotURL
	}
	rate, err := fetchCoinbaseUSDTSpot(ctx, client, endpoint)
	if err == nil {
		value := USDTExchangeRate{Rate: rate, Source: "Coinbase", At: now}
		s.mu.Lock()
		s.current, s.hasValue = value, true
		s.mu.Unlock()
		return value, nil
	}

	if hasPrevious && now.Sub(previous.At) <= usdtRateStaleFor {
		previous.Stale = true
		return previous, nil
	}
	return USDTExchangeRate{}, fmt.Errorf("USDT exchange rate unavailable: %w", err)
}

func fetchCoinbaseUSDTSpot(ctx context.Context, client *http.Client, endpoint string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("Coinbase returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUSDTExchangeResponseSize))
	if err != nil {
		return 0, err
	}
	var payload coinbaseUSDTSpotResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("invalid Coinbase response: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Data.Base), "USDT") || !strings.EqualFold(strings.TrimSpace(payload.Data.Currency), "CNY") {
		return 0, fmt.Errorf("Coinbase response is not USDT/CNY")
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(payload.Data.Amount), 64)
	if err != nil || rate <= 0 {
		return 0, fmt.Errorf("Coinbase returned an invalid USDT/CNY rate")
	}
	return rate, nil
}
