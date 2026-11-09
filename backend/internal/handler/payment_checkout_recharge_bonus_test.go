//go:build unit

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestGetCheckoutInfoIncludesRechargePromotions(t *testing.T) {
	db, err := sql.Open("sqlite", "file:checkout_recharge_promotions?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	settings := repository.NewSettingRepository(client)
	require.NoError(t, settings.SetMultiple(context.Background(), map[string]string{
		service.SettingRechargeBonusTiers: `[{"min_amount":100,"bonus_percent":15}]`,
		service.SettingRechargeFixedBonus: `{"enabled":true,"threshold_amount":500,"bonus_amount":200}`,
	}))
	config := service.NewPaymentConfigService(client, settings, nil)
	h := NewPaymentHandler(nil, config)
	getPaymentData := func(path string, handler gin.HandlerFunc) map[string]json.RawMessage {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
		handler(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var payload struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload.Data
	}

	checkout := getPaymentData("/api/v1/payment/checkout-info", h.GetCheckoutInfo)
	require.JSONEq(t, `true`, string(checkout["recharge_bonus_enabled"]))
	require.JSONEq(t, `[{"min_amount":100,"bonus_percent":15}]`, string(checkout["recharge_bonus_tiers"]))
	require.JSONEq(t, `{"enabled":true,"threshold_amount":500,"bonus_amount":200}`, string(checkout["recharge_fixed_bonus"]))

	require.NoError(t, settings.Set(context.Background(), service.SettingRechargeBonusEnabled, "false"))
	for _, data := range []map[string]json.RawMessage{
		getPaymentData("/api/v1/payment/checkout-info", h.GetCheckoutInfo),
		getPaymentData("/api/v1/payment/config", h.GetPaymentConfig),
	} {
		require.JSONEq(t, `false`, string(data["recharge_bonus_enabled"]))
		require.JSONEq(t, `[]`, string(data["recharge_bonus_tiers"]))
		require.JSONEq(t, `{"enabled":true,"threshold_amount":500,"bonus_amount":200}`, string(data["recharge_fixed_bonus"]))
	}
}
