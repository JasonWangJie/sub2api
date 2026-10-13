package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func newInvoiceRechargeTestService(t *testing.T) (*InvoiceService, *dbent.User) {
	t.Helper()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.ExecContext(context.Background(), `CREATE TABLE user_affiliate_ledger (
        id INTEGER PRIMARY KEY, user_id BIGINT, amount REAL, action TEXT, created_at TIMESTAMP
    )`)
	require.NoError(t, err)
	account, err := client.User.Create().SetEmail("invoice@example.com").SetUsername("Invoice User").SetPasswordHash("hash").Save(context.Background())
	require.NoError(t, err)
	settings := NewSettingService(&paymentConfigSettingRepoStub{values: map[string]string{SettingKeyEnterpriseInvoiceEnabled: "true"}}, nil)
	return NewInvoiceService(client, settings), account
}

func createInvoiceRechargeTestCode(t *testing.T, svc *InvoiceService, userID int64, codeType string, value float64, code string) *dbent.RedeemCode {
	t.Helper()
	item, err := svc.entClient.RedeemCode.Create().SetCode(code).SetType(codeType).SetValue(value).
		SetStatus(domain.StatusUsed).SetUsedBy(userID).SetUsedAt(time.Now()).Save(context.Background())
	require.NoError(t, err)
	return item
}

func TestInvoiceService_AdminRechargeLists(t *testing.T) {
	ctx := context.Background()
	svc, account := newInvoiceRechargeTestService(t)
	codes := make([]*dbent.RedeemCode, 0)
	for index, amount := range []float64{-100, 0, 50, 50, 99.99, 99.99999999, 100, 100.01} {
		codes = append(codes, createInvoiceRechargeTestCode(t, svc, account.ID, domain.AdjustmentTypeAdminBalance, amount, fmt.Sprintf("ADMIN-%d", index)))
	}
	card := createInvoiceRechargeTestCode(t, svc, account.ID, domain.RedeemTypeBalance, 50, "CARD-50")
	createInvoiceRechargeTestCode(t, svc, account.ID, domain.RedeemTypeBalance, 900, "PAYMENT-LINK")
	_, err := svc.entClient.PaymentOrder.Create().SetUserID(account.ID).SetUserEmail(account.Email).SetUserName(account.Username).
		SetAmount(900).SetPayAmount(900).SetFeeRate(0).SetRechargeCode("PAYMENT-LINK").SetOutTradeNo("PAY-900").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("trade-pay-900").SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("localhost").Save(ctx)
	require.NoError(t, err)

	userRecords, total, err := svc.ListUserRecords(ctx, account.ID, 1, 100)
	require.NoError(t, err)
	require.EqualValues(t, 8, total, "payment-linked card must not be counted twice")
	historicalRecords, historicalTotal, err := svc.ListHistoricalRecords(ctx, InvoiceRecordListParams{UserID: account.ID, PageSize: 100})
	require.NoError(t, err)
	require.Equal(t, total, historicalTotal)
	for _, records := range [][]InvoiceRecord{userRecords, historicalRecords} {
		byKey := make(map[string]InvoiceRecord)
		for _, record := range records {
			byKey[invoiceSourceKey(record.SourceType, record.SourceID)] = record
		}
		for _, code := range codes {
			record, exists := byKey[invoiceSourceKey(InvoiceSourceAdminGrant, code.ID)]
			if code.Value <= 0 {
				require.False(t, exists)
				continue
			}
			require.True(t, exists)
			require.Equal(t, code.Value, record.Amount)
			require.Equal(t, code.Value >= 100, record.Selectable)
			if code.Value < 100 {
				require.Equal(t, "ADMIN_RECHARGE_MINIMUM_NOT_MET", record.IneligibleReason)
			}
		}
		require.True(t, byKey[invoiceSourceKey(InvoiceSourceRedeemCode, card.ID)].Selectable)
	}
	available, availableTotal, err := svc.ListHistoricalRecords(ctx, InvoiceRecordListParams{UserID: account.ID, InvoiceStatus: "available", PageSize: 100})
	require.NoError(t, err)
	require.EqualValues(t, 4, availableTotal)
	for _, record := range available {
		require.True(t, record.Selectable)
	}
	page, pageTotal, err := svc.ListUserRecords(ctx, account.ID, 2, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.Equal(t, total, pageTotal)
}

func TestInvoiceService_AdminRechargeSourceValidation(t *testing.T) {
	ctx := context.Background()
	svc, account := newInvoiceRechargeTestService(t)
	admin := createInvoiceRechargeTestCode(t, svc, account.ID, domain.AdjustmentTypeAdminBalance, 100, "ADMIN-100")
	card := createInvoiceRechargeTestCode(t, svc, account.ID, domain.RedeemTypeBalance, 900, "CARD-900")
	low1 := createInvoiceRechargeTestCode(t, svc, account.ID, domain.AdjustmentTypeAdminBalance, 50, "ADMIN-LOW-1")
	low2 := createInvoiceRechargeTestCode(t, svc, account.ID, domain.AdjustmentTypeAdminBalance, 50, "ADMIN-LOW-2")
	negative := createInvoiceRechargeTestCode(t, svc, account.ID, domain.AdjustmentTypeAdminBalance, -100, "ADMIN-NEGATIVE")
	zero := createInvoiceRechargeTestCode(t, svc, account.ID, domain.AdjustmentTypeAdminBalance, 0, "ADMIN-ZERO")
	unused, err := svc.entClient.RedeemCode.Create().SetCode("ADMIN-UNUSED").SetType(domain.AdjustmentTypeAdminBalance).SetValue(100).Save(ctx)
	require.NoError(t, err)
	adminRef := InvoiceSourceRef{SourceType: InvoiceSourceAdminGrant, SourceID: admin.ID}
	cardRef := InvoiceSourceRef{SourceType: InvoiceSourceRedeemCode, SourceID: card.ID}
	items, total, err := svc.resolveSelectedInvoiceItems(ctx, account.ID, []InvoiceSourceRef{adminRef, cardRef})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "1000.00", total.StringFixed(2))
	require.Equal(t, InvoiceSourceAdminGrant, items[0].SourceType)
	resolved, err := svc.resolveHistoricalInvoiceSources(ctx, svc.entClient, []InvoiceSourceRef{adminRef})
	require.NoError(t, err)
	require.Equal(t, account.ID, resolved[0].UserID)

	for _, tc := range []struct {
		name string
		refs []InvoiceSourceRef
	}{
		{"admin disguised as card", []InvoiceSourceRef{{SourceType: InvoiceSourceRedeemCode, SourceID: admin.ID}}},
		{"card disguised as admin", []InvoiceSourceRef{{SourceType: InvoiceSourceAdminGrant, SourceID: card.ID}}},
		{"negative adjustment", []InvoiceSourceRef{{SourceType: InvoiceSourceAdminGrant, SourceID: negative.ID}}},
		{"zero adjustment", []InvoiceSourceRef{{SourceType: InvoiceSourceAdminGrant, SourceID: zero.ID}}},
		{"no recipient", []InvoiceSourceRef{{SourceType: InvoiceSourceAdminGrant, SourceID: unused.ID}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.resolveSelectedInvoiceItems(ctx, account.ID, tc.refs)
			require.Equal(t, "INVOICE_SOURCE_NOT_AVAILABLE", infraErrorReason(err))
			_, err = svc.resolveHistoricalInvoiceSources(ctx, svc.entClient, tc.refs)
			require.Equal(t, "INVOICE_SOURCE_NOT_AVAILABLE", infraErrorReason(err))
		})
	}
	_, _, err = svc.resolveSelectedInvoiceItems(ctx, account.ID+1, []InvoiceSourceRef{adminRef})
	require.Equal(t, "INVOICE_SOURCE_NOT_AVAILABLE", infraErrorReason(err), "users cannot invoice another user's admin recharge")
	lowRefs := []InvoiceSourceRef{cardRef, {SourceType: InvoiceSourceAdminGrant, SourceID: low1.ID}, {SourceType: InvoiceSourceAdminGrant, SourceID: low2.ID}}
	_, _, err = svc.resolveSelectedInvoiceItems(ctx, account.ID, lowRefs)
	require.Equal(t, "ADMIN_RECHARGE_MINIMUM_NOT_MET", infraErrorReason(err), "small adjustments cannot be combined to bypass the per-record threshold")
	_, err = svc.resolveHistoricalInvoiceSources(ctx, svc.entClient, lowRefs)
	require.Equal(t, "ADMIN_RECHARGE_MINIMUM_NOT_MET", infraErrorReason(err))
}

func TestInvoiceService_AdminRechargeBindings(t *testing.T) {
	for _, status := range []string{InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted, InvoiceApplicationStatusRejected, InvoiceRecordStatusHistorical} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			svc, account := newInvoiceRechargeTestService(t)
			code := createInvoiceRechargeTestCode(t, svc, account.ID, domain.AdjustmentTypeAdminBalance, 100, "ADMIN-100")
			ref := InvoiceSourceRef{SourceType: InvoiceSourceAdminGrant, SourceID: code.ID}
			if status == InvoiceRecordStatusHistorical {
				_, err := svc.entClient.InvoiceHistoricalMark.Create().SetUserID(account.ID).SetSourceType(ref.SourceType).SetSourceID(ref.SourceID).
					SetSourceReference(code.Code).SetAmount(code.Value).SetMarkedBy(account.ID).Save(ctx)
				require.NoError(t, err)
			} else {
				app, err := svc.entClient.InvoiceApplication.Create().SetUserID(account.ID).SetApplicationNo("INV-TEST").
					SetEmail(account.Email).SetTaxNumber("TAX").SetCompanyName("Example").SetTotalAmount(100).SetStatus(status).Save(ctx)
				require.NoError(t, err)
				_, err = svc.entClient.InvoiceApplicationItem.Create().SetApplicationID(app.ID).SetSourceType(ref.SourceType).
					SetSourceID(ref.SourceID).SetSourceReference(code.Code).SetAmount(code.Value).Save(ctx)
				require.NoError(t, err)
			}
			userRecords, _, err := svc.ListUserRecords(ctx, account.ID, 1, 20)
			require.NoError(t, err)
			adminRecords, _, err := svc.ListHistoricalRecords(ctx, InvoiceRecordListParams{UserID: account.ID})
			require.NoError(t, err)
			for _, records := range [][]InvoiceRecord{userRecords, adminRecords} {
				require.Len(t, records, 1)
				require.Equal(t, status == InvoiceApplicationStatusRejected, records[0].Selectable)
				if status != InvoiceApplicationStatusRejected {
					require.Equal(t, status, records[0].ApplicationStatus)
				}
			}
			_, _, userErr := svc.resolveSelectedInvoiceItems(ctx, account.ID, []InvoiceSourceRef{ref})
			_, adminErr := svc.resolveHistoricalInvoiceSources(ctx, svc.entClient, []InvoiceSourceRef{ref})
			if status == InvoiceApplicationStatusRejected {
				require.NoError(t, userErr)
				require.NoError(t, adminErr)
			} else {
				require.Equal(t, "INVOICE_SOURCE_ALREADY_APPLIED", infraErrorReason(userErr))
				require.Equal(t, "INVOICE_SOURCE_ALREADY_APPLIED", infraErrorReason(adminErr))
			}
		})
	}
}

// PostgreSQL transaction SQL is mocked here; the SQLite tests above execute
// actual eligibility, ownership, source-type, and invoice-state queries.
func TestInvoiceService_AdminRechargeTransactions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		historical bool
		mixed      bool
		amount     float64
		wantErr    string
	}{
		{name: "mixed application reaches 1000", mixed: true, amount: 100},
		{name: "application retains total minimum", amount: 100, wantErr: "INVOICE_MINIMUM_NOT_MET"},
		{name: "mixed application rejects small admin recharge", mixed: true, amount: 99.99, wantErr: "ADMIN_RECHARGE_MINIMUM_NOT_MET"},
		{name: "historical backfill accepts 100", historical: true, amount: 100},
		{name: "mixed historical backfill rejects small admin recharge", historical: true, mixed: true, amount: 99.99999999, wantErr: "ADMIN_RECHARGE_MINIMUM_NOT_MET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			settings := NewSettingService(&paymentConfigSettingRepoStub{values: map[string]string{SettingKeyEnterpriseInvoiceEnabled: "true"}}, nil)
			svc := NewInvoiceService(client, settings)
			refs := []InvoiceSourceRef{{SourceType: InvoiceSourceAdminGrant, SourceID: 1}}
			if tc.mixed {
				refs = append(refs, InvoiceSourceRef{SourceType: InvoiceSourcePaymentOrder, SourceID: 2})
			}
			mock.ExpectBegin()
			keys := make([]string, 0, len(refs))
			for _, ref := range refs {
				keys = append(keys, invoiceSourceKey(ref.SourceType, ref.SourceID))
			}
			sort.Strings(keys)
			for _, key := range keys {
				mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtext($1))`)).WithArgs(key).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if tc.mixed {
				mock.ExpectQuery(`SELECT .* FROM "payment_orders"`).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "amount", "out_trade_no"}).AddRow(2, 42, 900, "PAY-900"))
			}
			mock.ExpectQuery(`SELECT .* FROM "redeem_codes"`).WillReturnRows(sqlmock.NewRows([]string{"id", "type", "value", "used_by", "code"}).AddRow(1, domain.AdjustmentTypeAdminBalance, tc.amount, 42, "ADMIN-100"))
			if tc.amount >= 100 {
				for range refs {
					mock.ExpectQuery(`SELECT .* FROM "invoice_application_items"`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
					mock.ExpectQuery(`SELECT .* FROM "invoice_historical_marks"`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
				}
			}
			if tc.wantErr != "" {
				mock.ExpectRollback()
			} else {
				if tc.historical {
					mock.ExpectQuery(`INSERT INTO "invoice_historical_marks"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				} else {
					mock.ExpectQuery(`INSERT INTO "invoice_profiles"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
					mock.ExpectQuery(`INSERT INTO "invoice_applications"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
					for index := range refs {
						mock.ExpectQuery(`INSERT INTO "invoice_application_items"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(index + 1))
					}
				}
				mock.ExpectCommit()
			}
			if tc.historical {
				count, err := svc.MarkHistoricalRecords(context.Background(), 7, refs)
				require.Equal(t, tc.wantErr, infraErrorReason(err), "%v", err)
				if tc.wantErr == "" {
					require.Equal(t, 1, count)
				} else {
					require.Zero(t, count)
				}
			} else {
				app, err := svc.CreateApplication(context.Background(), 42, CreateInvoiceApplicationInput{
					InvoiceProfileData: InvoiceProfileData{Email: "invoice@example.com", TaxNumber: "TAX", CompanyName: "Example"}, Sources: refs,
				})
				require.Equal(t, tc.wantErr, infraErrorReason(err), "%v", err)
				if tc.wantErr == "" {
					require.NotNil(t, app)
					require.Equal(t, float64(1000), app.TotalAmount)
				} else {
					require.Nil(t, app)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
