//go:build unit

package admin

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type conflictingRechargeSettingsRepo struct {
	service.SettingRepository
	injected bool
}

func (r *conflictingRechargeSettingsRepo) SetMultiple(ctx context.Context, values map[string]string) error {
	if err := r.SettingRepository.SetMultiple(ctx, values); err != nil {
		return err
	}
	if _, ok := values[service.SettingKeyPromoCodeEnabled]; ok && !r.injected {
		r.injected = true
		return r.SettingRepository.Set(ctx, service.SettingRechargeFixedBonus,
			`{"enabled":true,"threshold_amount":100,"bonus_amount":10}`)
	}
	return nil
}

func TestSettingHandlerRechargeConflictRollsBackGeneralSettings(t *testing.T) {
	db, err := sql.Open("sqlite", "file:setting_recharge_transaction?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	baseRepo := repository.NewSettingRepository(client)
	require.NoError(t, baseRepo.Set(ctx, service.SettingKeyPromoCodeEnabled, "false"))
	require.NoError(t, baseRepo.Set(ctx, service.SettingRechargeBonusEnabled, "false"))
	require.NoError(t, baseRepo.Set(ctx, service.SettingRechargeBonusMode, service.RechargeBonusModeDiscount))
	require.NoError(t, baseRepo.Set(ctx, service.SettingRechargeBonusTiers,
		`[{"min_amount":100,"bonus_percent":20}]`))
	repo := &conflictingRechargeSettingsRepo{SettingRepository: baseRepo}
	settings := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	paymentConfig := service.NewPaymentConfigService(client, repo, nil)
	handler := NewSettingHandler(settings, nil, nil, nil, paymentConfig, nil, nil)

	response := doUpdateSettings(t, handler, map[string]any{
		"payment_recharge_bonus_enabled": true,
		"promo_code_enabled":             true,
	}, nil)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "RECHARGE_BONUS_MODE_CONFLICT")
	require.True(t, repo.injected)
	stored, err := baseRepo.GetAll(ctx)
	require.NoError(t, err)
	require.Equal(t, "false", stored[service.SettingKeyPromoCodeEnabled])
	require.Equal(t, "false", stored[service.SettingRechargeBonusEnabled])
	require.Empty(t, stored[service.SettingRechargeFixedBonus])
}
