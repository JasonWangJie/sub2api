package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAI429ModeDurableStopAndOutboxTransaction(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
		repo := &accountRepository{client: client}
		state := &service.OpenAI429State{Generation: "generation", Version: 17, Phase: service.OpenAI429Stopped, Consecutive429: 10}
		encoded, _ := json.Marshal(state)
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)UPDATE accounts.*openai_429_mode_enabled.*openai_429_mode_generation.*version.*`).
			WithArgs(int64(1), string(encoded), "generation", int64(17)).WillReturnResult(sqlmock.NewResult(0, affected))
		if affected > 0 {
			mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WithArgs(service.SchedulerOutboxEventAccountChanged, int64(1), nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		}
		mock.ExpectCommit()
		require.NoError(t, repo.StoreOpenAI429ModeState(context.Background(), 1, state))
		require.NoError(t, mock.ExpectationsWereMet())
		_ = client.Close()
	}
}

func TestOpenAI429ModeEditorPreservesLatestRuntime(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	account := &service.Account{ID: 1}
	extra := map[string]any{service.OpenAI429ModeEnabledKey: true, service.OpenAI429ModeGenerationKey: "generation", service.OpenAI429ModeStateKey: map[string]any{"phase": "normal"}}
	mock.ExpectQuery(`SELECT extra .*openai_429_mode_generation.*`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"generation", "state"}).AddRow("generation", []byte(`{"phase":"stopped","consecutive_429":10}`)))
	require.NoError(t, preserveOpenAI429ManagedExtra(context.Background(), client, account, extra))
	require.Equal(t, "stopped", extra[service.OpenAI429ModeStateKey].(map[string]any)["phase"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenAI429ModeStaleEditorCannotRestoreOlderGeneration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	extra := map[string]any{service.OpenAI429ModeEnabledKey: true, service.OpenAI429ModeGenerationKey: "0001"}
	mock.ExpectQuery(`SELECT extra .*openai_429_mode_generation.*`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"generation", "state"}).AddRow("0002", nil))
	require.ErrorContains(t, preserveOpenAI429ManagedExtra(context.Background(), client, &service.Account{ID: 1}, extra), "configuration changed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenAI429ModeQuotaTakeoverPreservesNewerHealthCache(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &tempUnschedCache{rdb: client}
	ctx := context.Background()
	quota := &service.TempUnschedState{UntilUnix: time.Now().Add(time.Hour).Unix(), ErrorMessage: "quota threshold"}
	health := &service.TempUnschedState{UntilUnix: time.Now().Add(2 * time.Hour).Unix(), ErrorMessage: "credentials invalid"}
	require.NoError(t, cache.SetTempUnsched(ctx, 1, quota))
	require.NoError(t, cache.DeleteTempUnschedIfUnchanged(ctx, 1, quota))
	state, err := cache.GetTempUnsched(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, state)
	require.NoError(t, cache.SetTempUnsched(ctx, 1, health))
	require.NoError(t, cache.DeleteTempUnschedIfUnchanged(ctx, 1, quota))
	state, err = cache.GetTempUnsched(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, health, state)
}
