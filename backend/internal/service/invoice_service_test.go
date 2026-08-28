package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateInvoiceProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile InvoiceProfileData
		wantErr string
	}{
		{name: "trims valid profile", profile: InvoiceProfileData{Email: " billing@example.com ", TaxNumber: " 9132X ", CompanyName: " Example Ltd "}},
		{name: "requires all fields", profile: InvoiceProfileData{Email: "billing@example.com", TaxNumber: "", CompanyName: "Example Ltd"}, wantErr: "INVOICE_PROFILE_REQUIRED"},
		{name: "rejects malformed email", profile: InvoiceProfileData{Email: "not-an-email", TaxNumber: "9132X", CompanyName: "Example Ltd"}, wantErr: "INVALID_INVOICE_EMAIL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateInvoiceProfile(tt.profile)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Equal(t, tt.wantErr, infraErrorReason(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, InvoiceProfileData{Email: "billing@example.com", TaxNumber: "9132X", CompanyName: "Example Ltd"}, got)
		})
	}
}

func TestInvoiceService_EnsureInvoiceEnabledFailsClosedWithoutSettingsService(t *testing.T) {
	service := &InvoiceService{}

	err := service.ensureInvoiceEnabled(context.Background())

	require.Equal(t, "INVOICE_DISABLED", infraErrorReason(err))
}

func TestNormalizeInvoiceSourceRefs(t *testing.T) {
	refs, err := normalizeInvoiceSourceRefs([]InvoiceSourceRef{
		{SourceType: " payment_order ", SourceID: 42},
		{SourceType: InvoiceSourceRedeemCode, SourceID: 7},
	})
	require.NoError(t, err)
	require.Equal(t, []InvoiceSourceRef{
		{SourceType: InvoiceSourcePaymentOrder, SourceID: 42},
		{SourceType: InvoiceSourceRedeemCode, SourceID: 7},
	}, refs)

	_, err = normalizeInvoiceSourceRefs([]InvoiceSourceRef{{SourceType: InvoiceSourcePaymentOrder, SourceID: 42}, {SourceType: InvoiceSourcePaymentOrder, SourceID: 42}})
	require.Equal(t, "DUPLICATE_INVOICE_SOURCE", infraErrorReason(err))

	_, err = normalizeInvoiceSourceRefs([]InvoiceSourceRef{{SourceType: InvoiceSourceAdminGrant, SourceID: 1}})
	require.Equal(t, "INVALID_INVOICE_SOURCE", infraErrorReason(err))
}

func TestIncludeAdminInvoiceRecord(t *testing.T) {
	available := InvoiceRecord{UserID: 42, UserEmail: "billing@example.com", UserName: "Acme", SourceReference: "PAY-100", Selectable: true}
	applied := available
	applied.ApplicationStatus = InvoiceApplicationStatusPending
	rejected := available
	rejected.ApplicationStatus = InvoiceApplicationStatusRejected
	historical := available
	historical.ApplicationStatus = InvoiceRecordStatusHistorical
	historical.Selectable = false

	require.True(t, includeAdminInvoiceRecord(available, "available", "billing"))
	require.True(t, includeAdminInvoiceRecord(available, "all", "42"))
	require.False(t, includeAdminInvoiceRecord(available, "applied", ""))
	require.True(t, includeAdminInvoiceRecord(applied, "applied", "PAY-100"))
	require.True(t, includeAdminInvoiceRecord(rejected, "available", "PAY-100"))
	require.False(t, includeAdminInvoiceRecord(rejected, "applied", ""))
	require.True(t, includeAdminInvoiceRecord(historical, "historical_completed", "acme"))
	require.False(t, includeAdminInvoiceRecord(historical, "available", ""))
}

func TestApplyHistoricalSourceBinding(t *testing.T) {
	service := &InvoiceService{}
	markedAt := time.Now()
	markedBy := int64(7)
	record := service.applySourceBinding(InvoiceRecord{
		SourceType: InvoiceSourcePaymentOrder, SourceID: 9, Selectable: true,
	}, map[string]invoiceApplicationBinding{
		invoiceSourceKey(InvoiceSourcePaymentOrder, 9): {
			Status: InvoiceRecordStatusHistorical, MarkedAt: &markedAt, MarkedBy: &markedBy,
		},
	})

	require.False(t, record.Selectable)
	require.Equal(t, InvoiceRecordStatusHistorical, record.ApplicationStatus)
	require.Equal(t, "HISTORICAL_INVOICE_COMPLETED", record.IneligibleReason)
	require.Equal(t, markedAt, *record.MarkedAt)
	require.Equal(t, markedBy, *record.MarkedBy)
}

func infraErrorReason(err error) string {
	if err == nil {
		return ""
	}
	return infraerrors.Reason(err)
}
