//go:build unit

package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	_ "modernc.org/sqlite"
)

func TestSelectInstanceRejectsDailyUsageQueryFailure(t *testing.T) {
	ctx := context.Background()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(TypeEasyPay).SetConfig("{}").SetSupportedTypes(TypeAlipay).
		SetLimits(`{"alipay":{"dailyLimit":100}}`).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE payment_orders"); err != nil {
		t.Fatal(err)
	}
	_, err = NewDefaultLoadBalancer(client, nil).SelectInstance(ctx, TypeEasyPay, TypeAlipay, StrategyRoundRobin, 50)
	if err == nil || !strings.Contains(err.Error(), "query provider instance daily usage") {
		t.Fatalf("expected daily usage query failure to block selection, got %v", err)
	}
}

func TestRecheckSelectedInstanceLimitsRejectsStaleSelection(t *testing.T) {
	ctx := context.Background()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(TypeEasyPay).SetConfig("{}").SetSupportedTypes(TypeAlipay).
		SetLimits(`{"alipay":{"dailyLimit":100}}`).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lb := NewDefaultLoadBalancer(client, nil)
	selection, err := lb.SelectInstance(ctx, TypeEasyPay, TypeAlipay, StrategyRoundRobin, 50)
	if err != nil {
		t.Fatal(err)
	}
	user, err := client.User.Create().SetEmail("payer@example.com").SetPasswordHash("hash").SetUsername("payer").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail("payer@example.com").SetUserName("payer").
		SetAmount(60).SetPayAmount(60).SetRechargeCode("pending-usage").
		SetPaymentType(TypeAlipay).SetPaymentTradeNo("").SetProviderInstanceID(selection.InstanceID).
		SetClientIP("127.0.0.1").SetSrcHost("app.example.com").
		SetExpiresAt(time.Now().Add(time.Hour)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := RecheckSelectedInstanceLimitsInTx(ctx, tx, selection, TypeAlipay, 40); err != nil {
		t.Fatalf("exact remaining capacity rejected: %v", err)
	}
	if err := RecheckSelectedInstanceLimitsInTx(ctx, tx, selection, TypeAlipay, 50); err == nil || !strings.Contains(err.Error(), "insufficient capacity") {
		t.Fatalf("stale selection exceeded daily capacity: %v", err)
	}
}

func TestInstanceDailyLimitCountsCrossMidnightPaymentsAndPendingReservations(t *testing.T) {
	ctx := context.Background()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(TypeEasyPay).SetConfig("{}").SetSupportedTypes(TypeAlipay).
		SetLimits(`{"alipay":{"dailyLimit":100}}`).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user, err := client.User.Create().SetEmail("midnight@example.com").SetPasswordHash("hash").SetUsername("midnight").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lb := NewDefaultLoadBalancer(client, nil)
	staleSelection, err := lb.SelectInstance(ctx, TypeEasyPay, TypeAlipay, StrategyRoundRobin, 50)
	if err != nil {
		t.Fatal(err)
	}
	previousDay := startOfDay(time.Now()).Add(-time.Minute)
	newOrder := func(code, status string, amount float64) *dbent.PaymentOrderCreate {
		return client.PaymentOrder.Create().
			SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
			SetAmount(amount).SetPayAmount(amount).SetRechargeCode(code).
			SetPaymentType(TypeAlipay).SetPaymentTradeNo("").SetProviderInstanceID(staleSelection.InstanceID).
			SetClientIP("127.0.0.1").SetSrcHost("app.example.com").
			SetStatus(status).SetCreatedAt(previousDay).SetExpiresAt(time.Now().Add(time.Hour))
	}
	if _, err := newOrder("paid-across-midnight", OrderStatusRefunded, 60).SetPaidAt(time.Now()).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := lb.SelectInstance(ctx, TypeEasyPay, TypeAlipay, StrategyRoundRobin, 40); err != nil {
		t.Fatalf("exact daily remainder rejected: %v", err)
	}
	if _, err := lb.SelectInstance(ctx, TypeEasyPay, TypeAlipay, StrategyRoundRobin, 41); err == nil {
		t.Fatal("payment made today from a yesterday order did not consume daily capacity")
	}
	if _, err := newOrder("pending-across-midnight", OrderStatusPending, 10).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := lb.SelectInstance(ctx, TypeEasyPay, TypeAlipay, StrategyRoundRobin, 30); err != nil {
		t.Fatalf("exact capacity after pending reservation rejected: %v", err)
	}
	if _, err := lb.SelectInstance(ctx, TypeEasyPay, TypeAlipay, StrategyRoundRobin, 31); err == nil {
		t.Fatal("yesterday pending order did not reserve today's capacity")
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := RecheckSelectedInstanceLimitsInTx(ctx, tx, staleSelection, TypeAlipay, 31); err == nil {
		t.Fatal("transactional recheck ignored cross-midnight usage")
	}
}

func TestRecheckSelectedInstanceLimitsAcquiresPostgresTransactionLock(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("lock unavailable"))
	mock.ExpectRollback()
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = RecheckSelectedInstanceLimitsInTx(ctx, tx, &InstanceSelection{InstanceID: "7", ProviderKey: TypeEasyPay}, TypeAlipay, 50)
	if err == nil || !strings.Contains(err.Error(), "lock payment instance capacity") {
		t.Fatalf("expected transaction lock failure, got %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInstanceSupportsType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		supportedTypes string
		target         PaymentType
		expected       bool
	}{
		{
			name:           "exact match single type",
			supportedTypes: "alipay",
			target:         "alipay",
			expected:       true,
		},
		{
			name:           "no match single type",
			supportedTypes: "wxpay",
			target:         "alipay",
			expected:       false,
		},
		{
			name:           "match in comma-separated list",
			supportedTypes: "alipay,wxpay,stripe",
			target:         "wxpay",
			expected:       true,
		},
		{
			name:           "first in comma-separated list",
			supportedTypes: "alipay,wxpay",
			target:         "alipay",
			expected:       true,
		},
		{
			name:           "last in comma-separated list",
			supportedTypes: "alipay,wxpay,stripe",
			target:         "stripe",
			expected:       true,
		},
		{
			name:           "no match in comma-separated list",
			supportedTypes: "alipay,wxpay",
			target:         "stripe",
			expected:       false,
		},
		{
			name:           "empty target",
			supportedTypes: "alipay,wxpay",
			target:         "",
			expected:       false,
		},
		{
			name:           "types with spaces are trimmed",
			supportedTypes: " alipay , wxpay ",
			target:         "alipay",
			expected:       true,
		},
		{
			name:           "legacy alipay direct supports canonical visible method",
			supportedTypes: "alipay_direct",
			target:         "alipay",
			expected:       true,
		},
		{
			name:           "legacy wxpay direct supports canonical visible method",
			supportedTypes: "wxpay_direct",
			target:         "wxpay",
			expected:       true,
		},
		{
			name:           "empty supported types means all supported",
			supportedTypes: "",
			target:         "alipay",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := InstanceSupportsType(tt.supportedTypes, tt.target)
			if got != tt.expected {
				t.Fatalf("InstanceSupportsType(%q, %q) = %v, want %v", tt.supportedTypes, tt.target, got, tt.expected)
			}
		})
	}
}

func TestGetInstanceChannelLimitsFallsBackToLegacyDirectAliases(t *testing.T) {
	t.Parallel()

	inst := testInstance(1, TypeAlipay, makeLimitsJSON(TypeAlipayDirect, ChannelLimits{SingleMax: 66}))
	got := getInstanceChannelLimits(inst, TypeAlipay)
	if got.SingleMax != 66 {
		t.Fatalf("getInstanceChannelLimits() = %+v, want SingleMax=66", got)
	}

	wxInst := testInstance(2, TypeWxpay, makeLimitsJSON(TypeWxpayDirect, ChannelLimits{SingleMin: 8}))
	wxGot := getInstanceChannelLimits(wxInst, TypeWxpay)
	if wxGot.SingleMin != 8 {
		t.Fatalf("getInstanceChannelLimits() = %+v, want SingleMin=8", wxGot)
	}
}

// ---------------------------------------------------------------------------
// Helper to build test PaymentProviderInstance values
// ---------------------------------------------------------------------------

func testInstance(id int64, providerKey, limits string) *dbent.PaymentProviderInstance {
	return &dbent.PaymentProviderInstance{
		ID:          id,
		ProviderKey: providerKey,
		Limits:      limits,
		Enabled:     true,
	}
}

// makeLimitsJSON builds a limits JSON string for a single payment type.
func makeLimitsJSON(paymentType string, cl ChannelLimits) string {
	m := map[string]ChannelLimits{paymentType: cl}
	b, _ := json.Marshal(m)
	return string(b)
}

// ---------------------------------------------------------------------------
// filterByLimits
// ---------------------------------------------------------------------------

func TestFilterByLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		candidates  []instanceCandidate
		paymentType PaymentType
		orderAmount float64
		wantIDs     []int64 // expected surviving instance IDs
	}{
		{
			name: "order below SingleMin is filtered out",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10})), dailyUsed: 0},
			},
			paymentType: "alipay",
			orderAmount: 5,
			wantIDs:     nil,
		},
		{
			name: "order at exact SingleMin boundary passes",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10})), dailyUsed: 0},
			},
			paymentType: "alipay",
			orderAmount: 10,
			wantIDs:     []int64{1},
		},
		{
			name: "order above SingleMax is filtered out",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMax: 100})), dailyUsed: 0},
			},
			paymentType: "alipay",
			orderAmount: 150,
			wantIDs:     nil,
		},
		{
			name: "order at exact SingleMax boundary passes",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMax: 100})), dailyUsed: 0},
			},
			paymentType: "alipay",
			orderAmount: 100,
			wantIDs:     []int64{1},
		},
		{
			name: "daily used + orderAmount exceeding dailyLimit is filtered out",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 500})), dailyUsed: 480},
			},
			paymentType: "alipay",
			orderAmount: 30,
			wantIDs:     nil, // 480+30=510 > 500
		},
		{
			name: "daily used + orderAmount equal to dailyLimit passes (strict greater-than)",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 500})), dailyUsed: 480},
			},
			paymentType: "alipay",
			orderAmount: 20,
			wantIDs:     []int64{1}, // 480+20=500, 500 > 500 is false → passes
		},
		{
			name: "daily used + orderAmount below dailyLimit passes",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 500})), dailyUsed: 400},
			},
			paymentType: "alipay",
			orderAmount: 50,
			wantIDs:     []int64{1},
		},
		{
			name: "no limits configured passes through",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", ""), dailyUsed: 99999},
			},
			paymentType: "alipay",
			orderAmount: 100,
			wantIDs:     []int64{1},
		},
		{
			name: "multiple candidates with partial filtering",
			candidates: []instanceCandidate{
				// singleMax=50, order=80 → filtered out
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMax: 50})), dailyUsed: 0},
				// no limits → passes
				{inst: testInstance(2, "easypay", ""), dailyUsed: 0},
				// singleMin=100, order=80 → filtered out
				{inst: testInstance(3, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 100})), dailyUsed: 0},
				// daily limit ok → passes (500+80=580 < 1000)
				{inst: testInstance(4, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 1000})), dailyUsed: 500},
			},
			paymentType: "alipay",
			orderAmount: 80,
			wantIDs:     []int64{2, 4},
		},
		{
			name: "zero SingleMin and SingleMax means no single-transaction limit",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 0, SingleMax: 0, DailyLimit: 0})), dailyUsed: 0},
			},
			paymentType: "alipay",
			orderAmount: 99999,
			wantIDs:     []int64{1},
		},
		{
			name: "all limits combined - order passes all checks",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10, SingleMax: 200, DailyLimit: 1000})), dailyUsed: 500},
			},
			paymentType: "alipay",
			orderAmount: 50,
			wantIDs:     []int64{1},
		},
		{
			name: "all limits combined - order fails SingleMin",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10, SingleMax: 200, DailyLimit: 1000})), dailyUsed: 500},
			},
			paymentType: "alipay",
			orderAmount: 5,
			wantIDs:     nil,
		},
		{
			name:        "empty candidates returns empty",
			candidates:  nil,
			paymentType: "alipay",
			orderAmount: 10,
			wantIDs:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterByLimits(tt.candidates, tt.paymentType, tt.orderAmount)
			gotIDs := make([]int64, len(got))
			for i, c := range got {
				gotIDs[i] = c.inst.ID
			}
			if !int64SliceEqual(gotIDs, tt.wantIDs) {
				t.Fatalf("filterByLimits() returned IDs %v, want %v", gotIDs, tt.wantIDs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// pickLeastAmount
// ---------------------------------------------------------------------------

func TestPickLeastAmount(t *testing.T) {
	t.Parallel()

	t.Run("picks candidate with lowest dailyUsed", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(1, "easypay", ""), dailyUsed: 300},
			{inst: testInstance(2, "easypay", ""), dailyUsed: 100},
			{inst: testInstance(3, "easypay", ""), dailyUsed: 200},
		}
		got := pickLeastAmount(candidates)
		if got.inst.ID != 2 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 2", got.inst.ID)
		}
	})

	t.Run("with equal dailyUsed picks the first one", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(1, "easypay", ""), dailyUsed: 100},
			{inst: testInstance(2, "easypay", ""), dailyUsed: 100},
			{inst: testInstance(3, "easypay", ""), dailyUsed: 200},
		}
		got := pickLeastAmount(candidates)
		if got.inst.ID != 1 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 1 (first with lowest)", got.inst.ID)
		}
	})

	t.Run("single candidate returns that candidate", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(42, "easypay", ""), dailyUsed: 999},
		}
		got := pickLeastAmount(candidates)
		if got.inst.ID != 42 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 42", got.inst.ID)
		}
	})

	t.Run("zero usage among non-zero picks zero", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(1, "easypay", ""), dailyUsed: 500},
			{inst: testInstance(2, "easypay", ""), dailyUsed: 0},
			{inst: testInstance(3, "easypay", ""), dailyUsed: 300},
		}
		got := pickLeastAmount(candidates)
		if got.inst.ID != 2 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 2", got.inst.ID)
		}
	})
}

// ---------------------------------------------------------------------------
// getInstanceChannelLimits
// ---------------------------------------------------------------------------

func TestGetInstanceChannelLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		inst        *dbent.PaymentProviderInstance
		paymentType PaymentType
		want        ChannelLimits
	}{
		{
			name:        "empty limits string returns zero ChannelLimits",
			inst:        testInstance(1, "easypay", ""),
			paymentType: "alipay",
			want:        ChannelLimits{},
		},
		{
			name:        "invalid JSON returns zero ChannelLimits",
			inst:        testInstance(1, "easypay", "not-json{"),
			paymentType: "alipay",
			want:        ChannelLimits{},
		},
		{
			name: "valid JSON with matching payment type",
			inst: testInstance(1, "easypay",
				`{"alipay":{"singleMin":5,"singleMax":200,"dailyLimit":1000}}`),
			paymentType: "alipay",
			want:        ChannelLimits{SingleMin: 5, SingleMax: 200, DailyLimit: 1000},
		},
		{
			name: "payment type not in limits returns zero ChannelLimits",
			inst: testInstance(1, "easypay",
				`{"alipay":{"singleMin":5,"singleMax":200}}`),
			paymentType: "wxpay",
			want:        ChannelLimits{},
		},
		{
			name: "stripe provider uses stripe lookup key regardless of payment type",
			inst: testInstance(1, "stripe",
				`{"stripe":{"singleMin":10,"singleMax":500,"dailyLimit":5000}}`),
			paymentType: "alipay",
			want:        ChannelLimits{SingleMin: 10, SingleMax: 500, DailyLimit: 5000},
		},
		{
			name: "stripe provider ignores payment type key even if present",
			inst: testInstance(1, "stripe",
				`{"stripe":{"singleMin":10,"singleMax":500},"alipay":{"singleMin":1,"singleMax":100}}`),
			paymentType: "alipay",
			want:        ChannelLimits{SingleMin: 10, SingleMax: 500},
		},
		{
			name: "non-stripe provider uses payment type as lookup key",
			inst: testInstance(1, "easypay",
				`{"alipay":{"singleMin":5},"wxpay":{"singleMin":10}}`),
			paymentType: "wxpay",
			want:        ChannelLimits{SingleMin: 10},
		},
		{
			name: "valid JSON with partial limits (only dailyLimit)",
			inst: testInstance(1, "easypay",
				`{"alipay":{"dailyLimit":800}}`),
			paymentType: "alipay",
			want:        ChannelLimits{DailyLimit: 800},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getInstanceChannelLimits(tt.inst, tt.paymentType)
			if got != tt.want {
				t.Fatalf("getInstanceChannelLimits() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// startOfDay
// ---------------------------------------------------------------------------

func TestStartOfDay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "midday returns midnight of same day",
			in:   time.Date(2025, 6, 15, 14, 30, 45, 123456789, time.UTC),
			want: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "midnight returns same time",
			in:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			want: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "last second of day returns midnight of same day",
			in:   time.Date(2025, 12, 31, 23, 59, 59, 999999999, time.UTC),
			want: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "preserves timezone location",
			in:   time.Date(2025, 3, 10, 15, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			want: time.Date(2025, 3, 10, 0, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := startOfDay(tt.in)
			if !got.Equal(tt.want) {
				t.Fatalf("startOfDay(%v) = %v, want %v", tt.in, got, tt.want)
			}
			// Also verify location is preserved.
			if got.Location().String() != tt.want.Location().String() {
				t.Fatalf("startOfDay() location = %v, want %v", got.Location(), tt.want.Location())
			}
		})
	}
}

func TestDecryptConfig_PlaintextAndLegacyCompat(t *testing.T) {
	t.Parallel()

	key := make([]byte, AES256KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	wrongKey := make([]byte, AES256KeySize)
	for i := range wrongKey {
		wrongKey[i] = byte(0xFF - i)
	}

	plaintextJSON := `{"appId":"app-123","secret":"sec-xyz"}`

	legacyEncrypted, err := Encrypt(plaintextJSON, key)
	if err != nil {
		t.Fatalf("seed Encrypt: %v", err)
	}

	tests := []struct {
		name   string
		stored string
		key    []byte
		want   map[string]string
	}{
		{
			name:   "empty stored returns nil map",
			stored: "",
			key:    key,
			want:   nil,
		},
		{
			name:   "plaintext JSON parses directly",
			stored: plaintextJSON,
			key:    nil,
			want:   map[string]string{"appId": "app-123", "secret": "sec-xyz"},
		},
		{
			name:   "plaintext JSON works even with key present",
			stored: plaintextJSON,
			key:    key,
			want:   map[string]string{"appId": "app-123", "secret": "sec-xyz"},
		},
		{
			name:   "legacy ciphertext with correct key decrypts",
			stored: legacyEncrypted,
			key:    key,
			want:   map[string]string{"appId": "app-123", "secret": "sec-xyz"},
		},
		{
			name:   "legacy ciphertext with no key treated as empty",
			stored: legacyEncrypted,
			key:    nil,
			want:   nil,
		},
		{
			name:   "legacy ciphertext with wrong key treated as empty",
			stored: legacyEncrypted,
			key:    wrongKey,
			want:   nil,
		},
		{
			name:   "garbage data treated as empty",
			stored: "not-json-and-not-ciphertext",
			key:    key,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lb := NewDefaultLoadBalancer(nil, tt.key)
			got, err := lb.decryptConfig(tt.stored)
			if err != nil {
				t.Fatalf("decryptConfig unexpected error: %v", err)
			}
			if !stringMapEqual(got, tt.want) {
				t.Fatalf("decryptConfig = %v, want %v", got, tt.want)
			}
		})
	}
}

// stringMapEqual compares two map[string]string values; nil and empty are equal.
func stringMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// int64SliceEqual compares two int64 slices for equality.
// Both nil and empty slices are treated as equal.
func int64SliceEqual(a, b []int64) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
