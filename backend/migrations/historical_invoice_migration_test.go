package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoricalInvoiceMigrationCreatesImmutableSourceLedger(t *testing.T) {
	sql, err := FS.ReadFile("229_ZJ_historical_invoice_marks.sql")
	require.NoError(t, err)
	text := string(sql)
	require.Contains(t, text, "CREATE TABLE IF NOT EXISTS invoice_historical_marks")
	require.Contains(t, text, "UNIQUE (source_type, source_id)")
	require.Contains(t, text, "marked_by BIGINT NOT NULL")
	require.Contains(t, text, "source_type IN ('payment_order', 'redeem_code')")
}
