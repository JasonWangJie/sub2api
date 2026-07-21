package repository

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageTaskMigrationContainsDurabilityAndOwnershipConstraints(t *testing.T) {
	content, err := migrations.FS.ReadFile("185_async_image_tasks.sql")
	require.NoError(t, err)
	sqlText := string(content)
	for _, required := range []string{
		"allow_async_image_generation BOOLEAN NOT NULL DEFAULT false",
		"CREATE TABLE IF NOT EXISTS async_image_tasks",
		"CREATE TABLE IF NOT EXISTS async_image_results",
		"CREATE TABLE IF NOT EXISTS async_image_staging_objects",
		"CREATE TABLE IF NOT EXISTS async_image_events",
		"CREATE TABLE IF NOT EXISTS async_image_outbox",
		"CREATE TABLE IF NOT EXISTS async_image_input_objects",
		"CREATE TABLE IF NOT EXISTS async_image_task_inputs",
		"async_image_tasks_owner_idempotency_uidx",
		"WHERE idempotency_key IS NOT NULL",
		"request_payload BYTEA NOT NULL",
		"billing_payload JSONB",
		"UNIQUE(task_id, image_index)",
		"REFERENCES async_image_tasks(task_id) ON DELETE CASCADE",
	} {
		require.Contains(t, sqlText, required)
	}
}

func TestAsyncImageTaskRepositoryCreateBindsOwnedInputInTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)INSERT INTO async_image_tasks .*RETURNING").
		WillReturnRows(asyncImageTaskRows(now, "asyncimg_input", "hash-input", service.AsyncImageTaskStatusQueued))
	mock.ExpectExec("(?s)WITH owned_input AS .*i.user_id = \\$3.*i.api_key_id = \\$4.*cleanup_claimed_at IS NULL.*FOR UPDATE OF i.*INSERT INTO async_image_task_inputs").
		WithArgs("asyncimg_input", int64(41), int64(1), int64(2)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("(?s)INSERT INTO async_image_events").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("(?s)INSERT INTO async_image_outbox .*ON CONFLICT").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewAsyncImageTaskRepository(db)
	_, reused, err := repo.CreateAsyncImageTask(context.Background(), service.CreateAsyncImageTaskParams{
		TaskID: "asyncimg_input", UserID: 1, APIKeyID: 2, GroupID: 3,
		Protocol: service.AsyncImageProtocolSC, Platform: service.PlatformGemini,
		RequestType: service.AsyncImageRequestTypeImageToImage, Model: "gemini-image",
		RequestHash: "hash-input", RequestPayload: []byte("ciphertext"), InputObjectIDs: []int64{41},
	})
	require.NoError(t, err)
	require.False(t, reused)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAsyncImageTaskRepositoryCreateIsTransactionalWithEventAndOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)INSERT INTO async_image_tasks .*RETURNING").
		WillReturnRows(asyncImageTaskRows(now, "asyncimg_1", "hash-1", service.AsyncImageTaskStatusQueued))
	mock.ExpectExec("(?s)INSERT INTO async_image_events").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("(?s)INSERT INTO async_image_outbox .*ON CONFLICT").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewAsyncImageTaskRepository(db)
	task, reused, err := repo.CreateAsyncImageTask(context.Background(), service.CreateAsyncImageTaskParams{
		TaskID: "asyncimg_1", UserID: 1, APIKeyID: 2, GroupID: 3,
		Protocol: service.AsyncImageProtocolBB, Platform: service.PlatformGemini,
		RequestType: service.AsyncImageRequestTypeTextToImage, Model: "gemini-image",
		RequestHash: "hash-1", RequestPayload: []byte("ciphertext"),
	})
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, "asyncimg_1", task.TaskID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAsyncImageTaskRepositoryIdempotencyRejectsDifferentRequestHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	key := "retry-key"
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)INSERT INTO async_image_tasks .*RETURNING").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "async_image_tasks_owner_idempotency_uidx"})
	mock.ExpectRollback()
	mock.ExpectQuery("(?s)FROM async_image_tasks WHERE api_key_id = \\$1 AND idempotency_key = \\$2").
		WithArgs(int64(2), key).
		WillReturnRows(asyncImageTaskRows(time.Now().UTC(), "asyncimg_existing", "different-hash", service.AsyncImageTaskStatusQueued))

	repo := NewAsyncImageTaskRepository(db)
	_, reused, err := repo.CreateAsyncImageTask(context.Background(), service.CreateAsyncImageTaskParams{
		TaskID: "asyncimg_new", UserID: 1, APIKeyID: 2, GroupID: 3,
		Protocol: service.AsyncImageProtocolBB, Platform: service.PlatformGemini,
		RequestType: service.AsyncImageRequestTypeTextToImage, Model: "gemini-image",
		IdempotencyKey: &key, RequestHash: "new-hash", RequestPayload: []byte("ciphertext"),
	})
	require.ErrorIs(t, err, service.ErrAsyncImageIdempotencyConflict)
	require.False(t, reused)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAsyncImageTaskRepositoryTransitionUsesVersionCASAndEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status, version FROM async_image_tasks").
		WithArgs("asyncimg_1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "version"}).AddRow(service.AsyncImageTaskStatusQueued, int64(1)))
	mock.ExpectQuery("(?s)UPDATE async_image_tasks SET.*WHERE task_id = \\$1.*version = \\$38.*updated_at <= \\$40.*RETURNING").
		WillReturnRows(asyncImageTaskRows(now, "asyncimg_1", "hash-1", service.AsyncImageTaskStatusInvoking))
	mock.ExpectExec("(?s)INSERT INTO async_image_events").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	progress := 10
	repo := NewAsyncImageTaskRepository(db)
	task, err := repo.TransitionAsyncImageTask(context.Background(), service.AsyncImageTaskTransition{
		TaskID: "asyncimg_1", ExpectedVersion: 1,
		FromStatuses: []string{service.AsyncImageTaskStatusQueued},
		ToStatus:     service.AsyncImageTaskStatusInvoking, Progress: &progress,
	})
	require.NoError(t, err)
	require.Equal(t, service.AsyncImageTaskStatusInvoking, task.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAsyncImageTaskRepositoryTouchHeartbeatDoesNotAdvanceVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("(?s)UPDATE async_image_tasks.*SET updated_at = NOW\\(\\).*status = ANY\\(\\$2\\)").
		WithArgs("asyncimg_heartbeat", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewAsyncImageTaskRepository(db)
	err = repo.TouchAsyncImageTask(context.Background(), "asyncimg_heartbeat", []string{service.AsyncImageTaskStatusInvoking})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAsyncImageTaskRepositoryClaimOutboxUsesSkipLocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)FOR UPDATE SKIP LOCKED.*RETURNING").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "task_id", "event_type", "dedup_key", "payload", "attempts",
			"available_at", "claimed_at", "published_at", "last_error", "created_at", "updated_at",
		}).AddRow(int64(1), "asyncimg_1", "task_ready", "asyncimg_1:created", []byte(`{}`), 1, now, now, nil, nil, now, now))
	mock.ExpectCommit()

	repo := NewAsyncImageTaskRepository(db)
	entries, err := repo.ClaimAsyncImageOutbox(context.Background(), 10, now.Add(-time.Minute))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "asyncimg_1", entries[0].TaskID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAsyncImageTaskRepositoryBackfillsOnlyMissingLibraryArchiveOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec(`(?s)INSERT INTO async_image_outbox.*library_archive.*image_library_items.*NOT EXISTS.*async_image_outbox`).
		WithArgs(200).
		WillReturnResult(sqlmock.NewResult(0, 2))

	repo := NewAsyncImageTaskRepository(db).(*asyncImageTaskRepository)
	count, err := repo.EnqueueMissingAsyncImageLibraryArchives(context.Background(), 200)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAsyncImageTaskRepositoryTerminalOutboxRetainsFailureWithoutRequeue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	completedAt := time.Now().UTC()
	mock.ExpectExec(`(?s)UPDATE async_image_outbox.*published_at = \$2.*last_error = \$3.*published_at IS NULL`).
		WithArgs(int64(7), completedAt, "image quota exceeded").
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewAsyncImageTaskRepository(db).(*asyncImageTaskRepository)
	err = repo.MarkAsyncImageOutboxTerminal(context.Background(), 7, completedAt, "image quota exceeded")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceAsyncImageResultsRegistersSharedStorageObject(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("asyncimg_1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("DELETE FROM async_image_results").
		WithArgs("asyncimg_1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("(?s)INSERT INTO image_storage_objects.*ON CONFLICT.*RETURNING id").
		WithArgs("aliyun", "images", "results/1.png", "image/png", int64(123), "checksum", 10, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(77)))
	mock.ExpectExec("(?s)INSERT INTO async_image_results.*storage_object_id").
		WithArgs("asyncimg_1", 0, "aliyun", "images", "results/1.png", "image/png", int64(123), "checksum", 10, 20, int64(77)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewAsyncImageTaskRepository(db)
	err = repo.ReplaceAsyncImageResults(context.Background(), "asyncimg_1", []service.AsyncImageResult{{
		ImageIndex: 0, Provider: "aliyun", Bucket: "images", ObjectKey: "results/1.png",
		ContentType: "image/png", ByteSize: 123, Checksum: "checksum", Width: intPtr(10), Height: intPtr(20),
	}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func intPtr(value int) *int {
	return &value
}

func asyncImageTaskRows(now time.Time, taskID, requestHash, status string) *sqlmock.Rows {
	columns := []string{
		"id", "task_id", "user_id", "api_key_id", "group_id", "account_id",
		"protocol", "platform", "request_type", "model", "status", "billing_status", "progress",
		"requested_image_size", "actual_image_size", "aspect_ratio", "image_count", "actual_cost", "currency",
		"idempotency_key", "request_hash", "request_payload", "prompt_preview",
		"upstream_request_id", "billing_request_id", "billing_payload",
		"retry_count", "storage_retry_count", "billing_retry_count", "version",
		"error_code", "error_message", "submitted_at", "started_at", "upstream_succeeded_at",
		"finished_at", "expires_at", "created_at", "updated_at",
	}
	values := []driver.Value{
		int64(1), taskID, int64(1), int64(2), int64(3), nil,
		service.AsyncImageProtocolBB, service.PlatformGemini, service.AsyncImageRequestTypeTextToImage,
		"gemini-image", status, service.AsyncImageBillingStatusPending, 0,
		nil, nil, nil, 1, nil, "USD", nil, requestHash, []byte("ciphertext"), nil,
		nil, nil, nil, 0, 0, 0, int64(1), nil, nil, now, nil, nil, nil, nil, now, now,
	}
	return sqlmock.NewRows(columns).AddRow(values...)
}
