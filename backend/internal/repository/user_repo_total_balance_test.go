package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestSumNonAdminBalancesFiltersRoleAndDeletedUsers(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newUserRepositoryWithSQL(client, db)

	mock.ExpectQuery("sum ordinary user balances").
		WithArgs(service.RoleUser).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(12.345))

	total, err := repo.SumNonAdminBalances(context.Background())
	require.NoError(t, err)
	require.InDelta(t, 12.345, total, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, `"users"."role" = $1`)
	require.Contains(t, normalized, `"users"."deleted_at" IS NULL`)
	require.NotContains(t, strings.ToLower(normalized), `"users"."role" <>`)
}

func TestSumNonAdminBalancesReturnsZeroForNullAggregate(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newUserRepositoryWithSQL(client, db)

	mock.ExpectQuery("sum ordinary user balances").
		WithArgs(service.RoleUser).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(nil))

	total, err := repo.SumNonAdminBalances(context.Background())
	require.NoError(t, err)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
