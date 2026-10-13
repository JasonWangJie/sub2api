//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMigration242AdminRechargeHistoricalInvoices(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	var userID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, username)
        VALUES ('migration242@example.com', 'hash', 'Migration 242') RETURNING id`).Scan(&userID))
	_, err := tx.ExecContext(ctx, `ALTER TABLE invoice_historical_marks
        DROP CONSTRAINT invoice_historical_marks_source_type_check;
        ALTER TABLE invoice_historical_marks ADD CONSTRAINT invoice_historical_marks_source_type_check
        CHECK (source_type IN ('payment_order', 'redeem_code'))`)
	require.NoError(t, err)
	insert := `INSERT INTO invoice_historical_marks
        (user_id, source_type, source_id, source_reference, amount, marked_by)
        VALUES ($1, $2, $3, 'RECHARGE-242', 100, $1)`
	for _, sourceType := range []string{"payment_order", "redeem_code"} {
		_, err := tx.ExecContext(ctx, insert, userID, sourceType, 242)
		require.NoError(t, err)
	}
	expectInvoiceLedgerConstraint(t, tx, "23514", insert, userID, "admin_grant", 242)

	migrationSQL, err := dbmigrations.FS.ReadFile("242_ZJ_allow_admin_recharge_invoices.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	var count int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice_historical_marks WHERE user_id = $1`, userID).Scan(&count))
	require.Equal(t, 2, count, "upgrade must preserve existing historical marks")
	_, err = tx.ExecContext(ctx, insert, userID, "admin_grant", 242)
	require.NoError(t, err, "new admin recharge source must be accepted")

	// Replaying the forward migration preserves all sources and uniqueness.
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	expectInvoiceLedgerConstraint(t, tx, "23505", insert, userID, "admin_grant", 242)
	expectInvoiceLedgerConstraint(t, tx, "23514", insert, userID, "affiliate_transfer", 243)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice_historical_marks WHERE user_id = $1`, userID).Scan(&count))
	require.Equal(t, 3, count)
}

func expectInvoiceLedgerConstraint(t *testing.T, tx *sql.Tx, code string, query string, args ...any) {
	t.Helper()
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `SAVEPOINT invoice_ledger_constraint`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, query, args...)
	var pgError *pq.Error
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, code, string(pgError.Code))
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT invoice_ledger_constraint`)
	require.NoError(t, err)
}
