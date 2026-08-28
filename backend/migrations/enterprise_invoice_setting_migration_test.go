package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnterpriseInvoiceSettingMigrationDefaultsToDisabled(t *testing.T) {
	sql, err := FS.ReadFile("230_ZJ_enterprise_invoice_setting.sql")
	require.NoError(t, err)
	text := string(sql)
	require.Contains(t, text, "enterprise_invoice_enabled")
	require.Contains(t, text, "'false'")
	require.Contains(t, text, "ON CONFLICT (key) DO NOTHING")
}
