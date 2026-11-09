//go:build unit

package service

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestPaymentConfigSettingsTransactionLocksAndRollsBack(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
	}{
		{name: "commit"},
		{name: "rollback", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			svc := &PaymentConfigService{entClient: client}
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
				WithArgs(hashAdvisoryLockID("payment_config_update")).
				WillReturnResult(sqlmock.NewResult(0, 1))
			if tc.fail {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			writeErr := errors.New("setting update failed")
			refreshed := false
			err = svc.RunSettingsTransaction(context.Background(), func(ctx context.Context) error {
				require.NotNil(t, dbent.TxFromContext(ctx))
				if tc.fail {
					return writeErr
				}
				return nil
			}, func() {
				refreshed = true
				require.False(t, svc.updateMu.TryLock(), "post-commit refresh must hold the local settings lock")
				require.NoError(t, mock.ExpectationsWereMet(), "post-commit refresh must follow the database commit")
			})
			if tc.fail {
				require.ErrorIs(t, err, writeErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, !tc.fail, refreshed)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
