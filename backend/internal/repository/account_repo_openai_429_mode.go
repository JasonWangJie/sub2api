package repository

import (
	"context"
	"encoding/json"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Configuration generation and state version fence delayed workers and
// out-of-order projections. The outbox and durable stop share one transaction.
func (r *accountRepository) StoreOpenAI429ModeState(ctx context.Context, id int64, state *service.OpenAI429State) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	result, err := tx.Client().ExecContext(ctx, `UPDATE accounts
		SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{openai_429_mode_state}', $2::jsonb, true)
		WHERE id = $1 AND deleted_at IS NULL
		AND extra -> 'openai_429_mode_enabled' = 'true'::jsonb
		AND extra ->> 'openai_429_mode_generation' = $3
		AND (extra -> 'openai_429_mode_state' ->> 'generation' IS DISTINCT FROM $3
		OR COALESCE((extra -> 'openai_429_mode_state' ->> 'version')::bigint, 0) < $4)`, id, string(data), state.Generation, state.Version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		if err := enqueueSchedulerOutbox(ctx, tx.Client(), service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if n > 0 {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return nil
}

// Called under the existing account row lock, so a full editor save cannot
// overwrite a newer coordinator result with a stale extra snapshot.
func preserveOpenAI429ManagedExtra(ctx context.Context, client *dbent.Client, account *service.Account, extra map[string]any) error {
	if _, present := extra[service.OpenAI429ModeGenerationKey]; !present {
		return nil
	}
	rows, err := client.QueryContext(ctx, `SELECT extra ->> 'openai_429_mode_generation', extra -> 'openai_429_mode_state' FROM accounts WHERE id = $1`, account.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return rows.Err()
	}
	var generation *string
	var snapshot []byte
	if err := rows.Scan(&generation, &snapshot); err != nil {
		return err
	}
	if incoming, ok := extra[service.OpenAI429ModeGenerationKey].(string); ok && generation != nil && incoming < *generation {
		return errors.New("account configuration changed while editing; reload the account before saving")
	}
	delete(extra, service.OpenAI429ModeStateKey)
	if enabled, _ := extra[service.OpenAI429ModeEnabledKey].(bool); enabled && generation != nil && *generation == extra[service.OpenAI429ModeGenerationKey] && len(snapshot) > 0 {
		var value any
		if err := json.Unmarshal(snapshot, &value); err != nil {
			return err
		}
		extra[service.OpenAI429ModeStateKey] = value
	}
	return rows.Err()
}

func openAI429BulkExtraExpression(base, parameter string) string {
	return "CASE WHEN COALESCE(extra -> 'openai_429_mode_enabled', 'false'::jsonb) IS DISTINCT FROM " + parameter + " -> 'openai_429_mode_enabled'" +
		" OR (" + parameter + " -> 'openai_429_mode_enabled' = 'true'::jsonb AND NULLIF(extra ->> 'openai_429_mode_generation', '') IS NULL)" +
		" THEN (" + base + ") - 'openai_429_mode_state'" +
		" ELSE (((" + base + ") - 'openai_429_mode_generation' - 'openai_429_mode_state') || jsonb_strip_nulls(jsonb_build_object(" +
		"'openai_429_mode_generation', extra -> 'openai_429_mode_generation', 'openai_429_mode_state', extra -> 'openai_429_mode_state'))) END"
}

func (r *accountRepository) TakeOverOpenAI429QuotaPause(ctx context.Context, a *service.Account, clearTemp, clearRate bool) (bool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Client().ExecContext(ctx, `UPDATE accounts SET
		temp_unschedulable_until = CASE WHEN $3 AND temp_unschedulable_reason = $4 THEN NULL ELSE temp_unschedulable_until END,
		temp_unschedulable_reason = CASE WHEN $3 AND temp_unschedulable_reason = $4 THEN NULL ELSE temp_unschedulable_reason END,
		rate_limit_reset_at = CASE WHEN $5 AND rate_limit_reset_at = $6 THEN NULL ELSE rate_limit_reset_at END,
		rate_limited_at = CASE WHEN $5 AND rate_limit_reset_at = $6 THEN NULL ELSE rate_limited_at END
		WHERE id = $1 AND deleted_at IS NULL AND extra -> 'openai_429_mode_enabled' = 'true'::jsonb
		AND extra ->> 'openai_429_mode_generation' = $2
		AND (($3 AND temp_unschedulable_reason = $4) OR ($5 AND rate_limit_reset_at = $6))`, a.ID, a.GetExtraString(service.OpenAI429ModeGenerationKey), clearTemp, a.TempUnschedulableReason, clearRate, a.RateLimitResetAt)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		if err := enqueueSchedulerOutbox(ctx, tx.Client(), service.SchedulerOutboxEventAccountChanged, &a.ID, nil, nil); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if n > 0 {
		r.syncSchedulerAccountSnapshot(ctx, a.ID)
	}
	return n > 0, nil
}
