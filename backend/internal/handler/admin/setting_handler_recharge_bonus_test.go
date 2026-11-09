package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_RechargeBonusSwitchesRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{service.SettingKeyPromoCodeEnabled: "false"}}
	settings := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(settings, nil, nil, nil, service.NewPaymentConfigService(nil, repo, nil), nil, nil)

	fixed := map[string]any{"enabled": true, "threshold_amount": 100.5, "bonus_amount": 15.25}
	rec := doUpdateSettings(t, handler, map[string]any{
		"payment_recharge_bonus_enabled": false,
		"payment_recharge_bonus_mode":    "discount",
		"payment_recharge_bonus_tiers":   []map[string]any{{"min_amount": 100, "bonus_percent": 20}},
		"payment_recharge_fixed_bonus":   fixed,
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertRules := func(body []byte) {
		t.Helper()
		var resp response.Response
		require.NoError(t, json.Unmarshal(body, &resp))
		data := resp.Data.(map[string]any)
		require.Equal(t, false, data["payment_recharge_bonus_enabled"])
		require.Equal(t, "discount", data["payment_recharge_bonus_mode"])
		require.Equal(t, []any{map[string]any{"min_amount": 100.0, "bonus_percent": 20.0}}, data["payment_recharge_bonus_tiers"])
		require.Equal(t, fixed, data["payment_recharge_fixed_bonus"])
	}
	assertRules(rec.Body.Bytes())

	rec = doUpdateSettings(t, handler, map[string]any{
		"payment_recharge_bonus_enabled": true,
		"promo_code_enabled":             true,
	}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	var rejection response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rejection))
	require.Equal(t, "RECHARGE_BONUS_MODE_CONFLICT", rejection.Reason)
	require.Equal(t, "false", repo.values[service.SettingKeyPromoCodeEnabled])

	rec = httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	handler.GetSettings(c)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertRules(rec.Body.Bytes())
}
