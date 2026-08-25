package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	epusdtHTTPTimeout     = 15 * time.Second
	maxEpusdtResponseSize = 1 << 20
)

// Epusdt implements the current GMPay API exposed by Epusdt.
type Epusdt struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

func NewEpusdt(instanceID string, config map[string]string) (*Epusdt, error) {
	for _, key := range []string{"apiBase", "pid", "secretKey", "networks", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("epusdt config missing required key: %s", key)
		}
	}
	networks, err := normalizeEpusdtNetworks(config["networks"])
	if err != nil {
		return nil, err
	}
	base, err := normalizeEpusdtBase(config["apiBase"])
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"notifyUrl", "returnUrl"} {
		callback, parseErr := url.Parse(strings.TrimSpace(config[key]))
		if parseErr != nil || callback.Scheme != "https" || callback.Host == "" {
			return nil, fmt.Errorf("epusdt %s must be a public HTTPS URL", key)
		}
	}
	cfg := make(map[string]string, len(config)+1)
	for k, v := range config {
		cfg[k] = v
	}
	currency := strings.ToUpper(strings.TrimSpace(config["currency"]))
	if currency == "" {
		currency = payment.DefaultPaymentCurrency
	}
	if currency != payment.DefaultPaymentCurrency && currency != "USDT" {
		return nil, fmt.Errorf("epusdt currency must be CNY or USDT")
	}
	if bonus := strings.TrimSpace(config["bonusRate"]); bonus != "" {
		value, parseErr := strconv.ParseFloat(bonus, 64)
		if parseErr != nil || value < 0 || value > 100 || math.IsNaN(value) || math.IsInf(value, 0) || math.Round(value*100) != value*100 {
			return nil, fmt.Errorf("epusdt bonusRate must be between 0 and 100 with at most 2 decimals")
		}
	}
	cfg["apiBase"], cfg["networks"], cfg["currency"] = base, strings.Join(networks, ","), currency
	return &Epusdt{instanceID: instanceID, config: cfg, httpClient: &http.Client{Timeout: epusdtHTTPTimeout}}, nil
}

func normalizeEpusdtBase(raw string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("epusdt apiBase must be a valid http(s) URL")
	}
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

func normalizeEpusdtNetworks(raw string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		n := strings.ToLower(strings.TrimSpace(parts[0]))
		if n == "" || strings.ContainsAny(n, "\r\n,;&= ") {
			return nil, fmt.Errorf("epusdt networks contain an invalid value")
		}
		if len(parts) == 2 && strings.ContainsAny(strings.TrimSpace(parts[1]), "\r\n,") {
			return nil, fmt.Errorf("epusdt network aliases contain an invalid value")
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		result = append(result, n)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("epusdt networks must contain at least one network")
	}
	return result, nil
}

func (e *Epusdt) Name() string        { return "Epusdt" }
func (e *Epusdt) ProviderKey() string { return payment.TypeEpusdt }
func (e *Epusdt) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeUSDT}
}

func (e *Epusdt) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"pid": strings.TrimSpace(e.config["pid"]), "currency": e.currency()}
}

func (e *Epusdt) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	network := strings.ToLower(strings.TrimSpace(req.Network))
	if network == "" || !epusdtNetworkEnabled(e.config["networks"], network) {
		return nil, fmt.Errorf("epusdt network is not enabled: %s", req.Network)
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(req.Amount), 64)
	if err != nil || amount <= 0 {
		return nil, fmt.Errorf("epusdt amount is invalid")
	}
	params := map[string]any{
		"pid": e.config["pid"], "order_id": req.OrderID, "currency": strings.ToLower(e.currency()),
		"token": "usdt", "network": network, "amount": amount,
		"notify_url":   resolveEpusdtURL(req.NotifyURL, e.config["notifyUrl"]),
		"redirect_url": resolveEpusdtURL(req.ReturnURL, e.config["returnUrl"]), "name": req.Subject,
	}
	params["signature"] = epusdtHMACAny(params, e.config["secretKey"])
	body, status, err := e.postJSON(ctx, e.apiBase()+"/payments/gmpay/v1/order/create-transaction", params)
	if err != nil {
		return nil, fmt.Errorf("epusdt create: %w", err)
	}
	var resp epusdtEnvelope
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("epusdt create parse: %w", err)
	}
	if status < 200 || status >= 300 || resp.StatusCode != 200 {
		return nil, fmt.Errorf("epusdt create rejected: %s", strings.TrimSpace(resp.Message))
	}
	if strings.TrimSpace(resp.Data.TradeID) == "" || strings.TrimSpace(resp.Data.PaymentURL) == "" {
		return nil, fmt.Errorf("epusdt create response missing trade_id or payment_url")
	}
	return &payment.CreatePaymentResponse{TradeNo: resp.Data.TradeID, PayURL: resp.Data.PaymentURL, Currency: e.currency()}, nil
}

func (e *Epusdt) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	body, status, err := e.get(ctx, e.apiBase()+"/pay/check-status/"+url.PathEscape(strings.TrimSpace(tradeNo)))
	if err != nil {
		return nil, fmt.Errorf("epusdt query: %w", err)
	}
	var resp epusdtEnvelope
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("epusdt query parse: %w", err)
	}
	if status < 200 || status >= 300 || resp.StatusCode != 200 {
		return nil, fmt.Errorf("epusdt query rejected: %s", strings.TrimSpace(resp.Message))
	}
	state := strconv.Itoa(resp.Data.Status)
	result := payment.ProviderStatusPending
	if resp.Data.Status == 2 {
		result = payment.ProviderStatusPaid
	} else if resp.Data.Status == 3 {
		result = payment.ProviderStatusFailed
	}
	return &payment.QueryOrderResponse{TradeNo: resp.Data.TradeID, Status: result, Amount: resp.Data.Amount, Metadata: map[string]string{"status": state, "currency": e.currency(), "pid": e.config["pid"]}}, nil
}

func (e *Epusdt) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	dec := json.NewDecoder(strings.NewReader(rawBody))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("epusdt callback JSON: %w", err)
	}
	signature, _ := payload["signature"].(string)
	if signature == "" || !hmac.Equal([]byte(strings.ToLower(signature)), []byte(epusdtHMACAny(payload, e.config["secretKey"]))) {
		return nil, fmt.Errorf("epusdt callback signature mismatch")
	}
	if strings.TrimSpace(stringValue(payload["pid"])) != strings.TrimSpace(e.config["pid"]) {
		return nil, fmt.Errorf("epusdt callback pid mismatch")
	}
	if stringValue(payload["status"]) != "2" {
		return nil, fmt.Errorf("epusdt callback is not successful")
	}
	if strings.TrimSpace(stringValue(payload["trade_id"])) == "" || strings.TrimSpace(stringValue(payload["order_id"])) == "" {
		return nil, fmt.Errorf("epusdt callback identifiers are missing")
	}
	if token := strings.ToLower(strings.TrimSpace(stringValue(payload["token"]))); token != "usdt" {
		return nil, fmt.Errorf("epusdt callback token mismatch")
	}
	amount, err := strconv.ParseFloat(stringValue(payload["amount"]), 64)
	if err != nil || amount <= 0 {
		return nil, fmt.Errorf("epusdt callback amount is invalid")
	}
	network := strings.ToLower(strings.TrimSpace(stringValue(payload["network"])))
	return &payment.PaymentNotification{
		TradeNo: stringValue(payload["trade_id"]), OrderID: stringValue(payload["order_id"]), Amount: amount,
		Status: payment.NotificationStatusSuccess, RawData: rawBody,
		Metadata: map[string]string{"pid": stringValue(payload["pid"]), "currency": e.currency(), "network": network, "token": strings.ToLower(stringValue(payload["token"]))},
	}, nil
}

func (e *Epusdt) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("epusdt does not support refunds")
}

func (e *Epusdt) apiBase() string { return strings.TrimRight(e.config["apiBase"], "/") }
func (e *Epusdt) currency() string {
	currency := strings.ToUpper(strings.TrimSpace(e.config["currency"]))
	if currency == "USDT" {
		return currency
	}
	return payment.DefaultPaymentCurrency
}
func resolveEpusdtURL(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
func epusdtNetworkEnabled(raw, wanted string) bool {
	for _, n := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(n), wanted) {
			return true
		}
	}
	return false
}

type epusdtData struct {
	TradeID    string  `json:"trade_id"`
	PaymentURL string  `json:"payment_url"`
	Status     int     `json:"status"`
	Amount     float64 `json:"amount"`
}
type epusdtEnvelope struct {
	StatusCode int        `json:"status_code"`
	Message    string     `json:"message"`
	Data       epusdtData `json:"data"`
}

func (e *Epusdt) postJSON(ctx context.Context, endpoint string, params map[string]any) ([]byte, int, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	return e.do(req)
}
func (e *Epusdt) get(ctx context.Context, endpoint string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	return e.do(req)
}
func (e *Epusdt) do(req *http.Request) ([]byte, int, error) {
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEpusdtResponseSize))
	return body, resp.StatusCode, err
}

func epusdtHMAC(params map[string]string, secret string) string {
	anyParams := make(map[string]any, len(params))
	for k, v := range params {
		anyParams[k] = v
	}
	return epusdtHMACAny(anyParams, secret)
}
func epusdtHMACAny(params map[string]any, secret string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "signature" || value == nil || stringValue(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+stringValue(params[key]))
	}
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(strings.Join(parts, "&")))
	return hex.EncodeToString(h.Sum(nil))
}
func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return string(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprint(v)
	}
}
