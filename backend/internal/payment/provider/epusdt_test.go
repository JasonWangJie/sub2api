package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestEpusdtGMPayCreateAndCallback(t *testing.T) {
	const secret = "secret-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payments/gmpay/v1/order/create-transaction":
			var params map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&params))
			require.Equal(t, "usdt", params["token"])
			require.Equal(t, "tron", params["network"])
			require.Equal(t, epusdtHMACAny(map[string]any{
				"pid": params["pid"], "order_id": params["order_id"], "currency": params["currency"],
				"token": params["token"], "network": params["network"], "amount": params["amount"],
				"notify_url": params["notify_url"], "redirect_url": params["redirect_url"], "name": params["name"],
			}, secret), params["signature"])
			_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":"trade-1","payment_url":"https://cashier.example/trade-1"}}`))
		case "/pay/check-status/trade-1":
			_, _ = w.Write([]byte(`{"status_code":200,"data":{"trade_id":"trade-1","status":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov, err := NewEpusdt("1", map[string]string{
		"apiBase": server.URL, "pid": "1000", "secretKey": secret, "networks": "TRON, tron",
		"notifyUrl": "https://merchant.example/api/v1/payment/webhook/epusdt", "returnUrl": "https://merchant.example/payment/result",
	})
	require.NoError(t, err)
	created, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "sub2_order", Amount: "100.00", PaymentType: payment.TypeUSDT, Subject: "Balance", Network: "tron"})
	require.NoError(t, err)
	require.Equal(t, "trade-1", created.TradeNo)
	require.Equal(t, "https://cashier.example/trade-1", created.PayURL)

	queried, err := prov.QueryOrder(context.Background(), "trade-1")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, queried.Status)

	callback := map[string]any{"pid": "1000", "trade_id": "trade-1", "order_id": "sub2_order", "amount": 100, "actual_amount": 14.29, "token": "USDT", "status": 2}
	callback["signature"] = epusdtHMACAny(callback, secret)
	raw, _ := json.Marshal(callback)
	notification, err := prov.VerifyNotification(context.Background(), string(raw), nil)
	require.NoError(t, err)
	require.Equal(t, "sub2_order", notification.OrderID)
	require.Equal(t, 100.0, notification.Amount)

	callback["signature"] = "bad"
	raw, _ = json.Marshal(callback)
	_, err = prov.VerifyNotification(context.Background(), string(raw), nil)
	require.Error(t, err)
}

func TestEpusdtUSDTModeCarriesCurrencyThroughProviderLifecycle(t *testing.T) {
	const secret = "usdt-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payments/gmpay/v1/order/create-transaction":
			var params map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&params))
			require.Equal(t, "usdt", params["currency"])
			require.Equal(t, "tron", params["network"])
			require.Equal(t, epusdtHMACAny(map[string]any{
				"pid": params["pid"], "order_id": params["order_id"], "currency": params["currency"],
				"token": params["token"], "network": params["network"], "amount": params["amount"],
				"notify_url": params["notify_url"], "redirect_url": params["redirect_url"], "name": params["name"],
			}, secret), params["signature"])
			_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":"usdt-trade","payment_url":"https://cashier.example/usdt-trade"}}`))
		case "/pay/check-status/usdt-trade":
			_, _ = w.Write([]byte(`{"status_code":200,"data":{"trade_id":"usdt-trade","status":2,"amount":20}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov, err := NewEpusdt("usdt-1", map[string]string{
		"apiBase": server.URL, "pid": "2000", "secretKey": secret, "currency": "USDT", "networks": "TRON=TRC20",
		"notifyUrl": "https://merchant.example/api/v1/payment/webhook/epusdt", "returnUrl": "https://merchant.example/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"pid": "2000", "currency": "USDT"}, prov.MerchantIdentityMetadata())

	created, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_usdt", Amount: "20.00", PaymentType: payment.TypeUSDT, Subject: "Balance", Network: "TRON",
	})
	require.NoError(t, err)
	require.Equal(t, "USDT", created.Currency)

	queried, err := prov.QueryOrder(context.Background(), "usdt-trade")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, queried.Status)
	require.Equal(t, "USDT", queried.Metadata["currency"])

	callback := map[string]any{"pid": "2000", "trade_id": "usdt-trade", "order_id": "sub2_usdt", "amount": 20, "token": "USDT", "network": "tron", "status": 2}
	callback["signature"] = epusdtHMACAny(callback, secret)
	raw, err := json.Marshal(callback)
	require.NoError(t, err)
	notification, err := prov.VerifyNotification(context.Background(), string(raw), nil)
	require.NoError(t, err)
	require.Equal(t, 20.0, notification.Amount)
	require.Equal(t, "USDT", notification.Metadata["currency"])
	require.Equal(t, "tron", notification.Metadata["network"])
}
