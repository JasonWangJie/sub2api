package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const asyncImageActiveTaskStatusesSQL = `
'queued', 'invoking', 'upstream_succeeded', 'uploading',
'billing_pending', 'storage_failed', 'billing_failed'`

const asyncImageTerminalTaskStatusesSQL = `
'succeeded', 'failed', 'execution_unknown', 'storage_failed', 'billing_failed', 'expired'`

func (r *asyncImageTaskRepository) RegisterAsyncImageInputObject(ctx context.Context, params service.RegisterAsyncImageInputObjectParams) (*service.AsyncImageInputObject, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("async image task repository is not configured")
	}
	object := &service.AsyncImageInputObject{}
	var width, height sql.NullInt64
	var filename sql.NullString
	err := r.sql.QueryRowContext(ctx, `
INSERT INTO async_image_input_objects (
    upload_id, user_id, api_key_id, provider, bucket, object_key,
    content_type, byte_size, checksum, width, height, url_hash, filename, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING id, upload_id, user_id, api_key_id, provider, bucket, object_key,
          content_type, byte_size, checksum, width, height, url_hash, filename,
          expires_at, cleanup_claimed_at, created_at`,
		strings.TrimSpace(params.UploadID), params.UserID, params.APIKeyID,
		params.ObjectRef.Provider, params.ObjectRef.Bucket, params.ObjectRef.ObjectKey,
		params.ObjectRef.ContentType, params.ObjectRef.SizeBytes, params.ObjectRef.ChecksumSHA256,
		nullablePositiveInt(params.ObjectRef.Width), nullablePositiveInt(params.ObjectRef.Height),
		strings.TrimSpace(params.URLHash), nullableTrimmedString(params.Filename), params.ExpiresAt,
	).Scan(
		&object.ID, &object.UploadID, &object.UserID, &object.APIKeyID,
		&object.ObjectRef.Provider, &object.ObjectRef.Bucket, &object.ObjectRef.ObjectKey,
		&object.ObjectRef.ContentType, &object.ObjectRef.SizeBytes, &object.ObjectRef.ChecksumSHA256,
		&width, &height, &object.URLHash, &filename,
		&object.ExpiresAt, &object.CleanupClaimedAt, &object.CreatedAt,
	)
	if err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrAsyncImageInvalidInput)
	}
	object.ObjectRef.Width = nullIntValue(width)
	object.ObjectRef.Height = nullIntValue(height)
	object.Filename = filename.String
	return object, nil
}

func (r *asyncImageTaskRepository) FindAsyncImageInputObjectsByURLHashes(ctx context.Context, hashes []string) ([]service.AsyncImageInputObject, error) {
	if len(hashes) == 0 {
		return []service.AsyncImageInputObject{}, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
SELECT id, upload_id, user_id, api_key_id, provider, bucket, object_key,
       content_type, byte_size, checksum, width, height, url_hash, filename,
       expires_at, cleanup_claimed_at, created_at
FROM async_image_input_objects
WHERE url_hash = ANY($1)`, pq.Array(hashes))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	objects := make([]service.AsyncImageInputObject, 0, len(hashes))
	for rows.Next() {
		object, scanErr := scanAsyncImageInputObject(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		objects = append(objects, object)
	}
	return objects, rows.Err()
}

func bindAsyncImageInputObjects(ctx context.Context, tx *sql.Tx, params service.CreateAsyncImageTaskParams) error {
	seen := make(map[int64]struct{}, len(params.InputObjectIDs))
	for _, inputID := range params.InputObjectIDs {
		if inputID <= 0 {
			return service.ErrAsyncImageInvalidInput
		}
		if _, exists := seen[inputID]; exists {
			continue
		}
		seen[inputID] = struct{}{}
		result, err := tx.ExecContext(ctx, `
WITH owned_input AS (
    SELECT i.id
    FROM async_image_input_objects i
    WHERE i.id = $2
      AND i.user_id = $3
      AND i.api_key_id = $4
      AND i.expires_at > NOW()
      AND i.cleanup_claimed_at IS NULL
    FOR UPDATE OF i
)
INSERT INTO async_image_task_inputs (task_id, input_object_id)
SELECT $1, i.id
FROM owned_input i
ON CONFLICT (task_id, input_object_id) DO NOTHING`,
			params.TaskID, inputID, params.UserID, params.APIKeyID,
		)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return service.ErrAsyncImageInvalidInput
		}
	}
	return nil
}

func (r *asyncImageTaskRepository) ClaimExpiredAsyncImageInputObjects(ctx context.Context, before, staleBefore time.Time, limit int) ([]service.AsyncImageInputObject, error) {
	limit = normalizeAsyncImageCleanupLimit(limit)
	rows, err := r.sql.QueryContext(ctx, `
WITH candidates AS (
    SELECT i.id
    FROM async_image_input_objects i
    WHERE i.expires_at <= $1
      AND (i.cleanup_claimed_at IS NULL OR i.cleanup_claimed_at <= $2)
      AND NOT EXISTS (
          SELECT 1
          FROM async_image_task_inputs ti
          JOIN async_image_tasks t ON t.task_id = ti.task_id
          WHERE ti.input_object_id = i.id
            AND t.status IN (`+asyncImageActiveTaskStatusesSQL+`)
      )
    ORDER BY i.expires_at, i.id
    LIMIT $3
    FOR UPDATE OF i SKIP LOCKED
)
UPDATE async_image_input_objects i
SET cleanup_claimed_at = NOW()
FROM candidates c
WHERE i.id = c.id
RETURNING i.id, i.upload_id, i.user_id, i.api_key_id, i.provider, i.bucket,
          i.object_key, i.content_type, i.byte_size, i.checksum, i.width,
          i.height, i.url_hash, i.filename, i.expires_at,
          i.cleanup_claimed_at, i.created_at`, before, staleBefore, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	objects := make([]service.AsyncImageInputObject, 0)
	for rows.Next() {
		object, scanErr := scanAsyncImageInputObject(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		objects = append(objects, object)
	}
	return objects, rows.Err()
}

func (r *asyncImageTaskRepository) CompleteAsyncImageInputObjectDeletion(ctx context.Context, id int64, claimedAt time.Time) error {
	return requireAsyncImageCleanupDelete(r.sql.ExecContext(ctx, `
DELETE FROM async_image_input_objects
WHERE id = $1 AND cleanup_claimed_at = $2
  AND NOT EXISTS (
      SELECT 1
      FROM async_image_task_inputs ti
      JOIN async_image_tasks t ON t.task_id = ti.task_id
      WHERE ti.input_object_id = async_image_input_objects.id
        AND t.status IN (`+asyncImageActiveTaskStatusesSQL+`)
  )`, id, claimedAt))
}

func (r *asyncImageTaskRepository) ReleaseAsyncImageInputObjectDeletion(ctx context.Context, id int64, claimedAt time.Time) error {
	_, err := r.sql.ExecContext(ctx, `
UPDATE async_image_input_objects SET cleanup_claimed_at = NULL
WHERE id = $1 AND cleanup_claimed_at = $2`, id, claimedAt)
	return err
}

func (r *asyncImageTaskRepository) ClaimExpiredAsyncImageResults(ctx context.Context, createdBefore, staleBefore time.Time, limit int) ([]service.AsyncImageResult, error) {
	limit = normalizeAsyncImageCleanupLimit(limit)
	rows, err := r.sql.QueryContext(ctx, `
WITH candidates AS (
    SELECT r.id
    FROM async_image_results r
    JOIN async_image_tasks t ON t.task_id = r.task_id
	    WHERE r.created_at <= $1
	      AND (r.cleanup_claimed_at IS NULL OR r.cleanup_claimed_at <= $2)
	      AND t.status IN (`+asyncImageTerminalTaskStatusesSQL+`)
	      AND t.cleanup_claimed_at IS NULL
	    ORDER BY r.created_at, r.id
	    LIMIT $3
	    FOR UPDATE OF r, t SKIP LOCKED
)
UPDATE async_image_results r
SET cleanup_claimed_at = NOW()
FROM candidates c
WHERE r.id = c.id
RETURNING r.id, r.task_id, r.image_index, r.provider, r.bucket, r.object_key,
          r.content_type, r.byte_size, r.checksum, r.width, r.height,
          r.cleanup_claimed_at, r.created_at`, createdBefore, staleBefore, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := make([]service.AsyncImageResult, 0)
	for rows.Next() {
		var result service.AsyncImageResult
		var width, height sql.NullInt64
		if err := rows.Scan(
			&result.ID, &result.TaskID, &result.ImageIndex, &result.Provider,
			&result.Bucket, &result.ObjectKey, &result.ContentType, &result.ByteSize,
			&result.Checksum, &width, &height, &result.CleanupClaimedAt, &result.CreatedAt,
		); err != nil {
			return nil, err
		}
		result.Width = nullableInt(width)
		result.Height = nullableInt(height)
		results = append(results, result)
	}
	return results, rows.Err()
}

func (r *asyncImageTaskRepository) CompleteAsyncImageResultDeletion(ctx context.Context, id int64, claimedAt time.Time) error {
	return requireAsyncImageCleanupDelete(r.sql.ExecContext(ctx, `
DELETE FROM async_image_results r
USING async_image_tasks t
WHERE r.id = $1 AND r.cleanup_claimed_at = $2
  AND t.task_id = r.task_id
  AND t.status IN (`+asyncImageTerminalTaskStatusesSQL+`)`, id, claimedAt))
}

func (r *asyncImageTaskRepository) ReleaseAsyncImageResultDeletion(ctx context.Context, id int64, claimedAt time.Time) error {
	_, err := r.sql.ExecContext(ctx, `
UPDATE async_image_results SET cleanup_claimed_at = NULL
WHERE id = $1 AND cleanup_claimed_at = $2`, id, claimedAt)
	return err
}

func (r *asyncImageTaskRepository) ClaimExpiredAsyncImageTasks(ctx context.Context, expiresBefore, createdBefore, staleBefore time.Time, limit int) ([]service.AsyncImageRetentionTask, error) {
	limit = normalizeAsyncImageCleanupLimit(limit)
	rows, err := r.sql.QueryContext(ctx, `
WITH candidates AS (
    SELECT t.id
    FROM async_image_tasks t
    WHERE t.status IN (`+asyncImageTerminalTaskStatusesSQL+`)
	      AND (t.expires_at <= $1 OR (t.expires_at IS NULL AND t.created_at <= $2))
	      AND (t.cleanup_claimed_at IS NULL OR t.cleanup_claimed_at <= $3)
	      AND NOT EXISTS (
	          SELECT 1 FROM async_image_results r
	          WHERE r.task_id = t.task_id
	            AND r.cleanup_claimed_at IS NOT NULL
	            AND r.cleanup_claimed_at > $3
	      )
    ORDER BY COALESCE(t.expires_at, t.created_at), t.id
    LIMIT $4
    FOR UPDATE OF t SKIP LOCKED
)
UPDATE async_image_tasks t
SET cleanup_claimed_at = NOW()
FROM candidates c
WHERE t.id = c.id
RETURNING t.task_id, t.cleanup_claimed_at`, expiresBefore, createdBefore, staleBefore, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tasks := make([]service.AsyncImageRetentionTask, 0)
	for rows.Next() {
		var task service.AsyncImageRetentionTask
		if err := rows.Scan(&task.TaskID, &task.CleanupClaimedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *asyncImageTaskRepository) CompleteAsyncImageTaskDeletion(ctx context.Context, taskID string, claimedAt time.Time) error {
	return requireAsyncImageCleanupDelete(r.sql.ExecContext(ctx, `
DELETE FROM async_image_tasks
WHERE task_id = $1 AND cleanup_claimed_at = $2
  AND status IN (`+asyncImageTerminalTaskStatusesSQL+`)`, taskID, claimedAt))
}

func (r *asyncImageTaskRepository) ReleaseAsyncImageTaskDeletion(ctx context.Context, taskID string, claimedAt time.Time) error {
	_, err := r.sql.ExecContext(ctx, `
UPDATE async_image_tasks SET cleanup_claimed_at = NULL
WHERE task_id = $1 AND cleanup_claimed_at = $2`, taskID, claimedAt)
	return err
}

func scanAsyncImageInputObject(scanner interface{ Scan(dest ...any) error }) (service.AsyncImageInputObject, error) {
	var object service.AsyncImageInputObject
	var width, height sql.NullInt64
	var filename sql.NullString
	err := scanner.Scan(
		&object.ID, &object.UploadID, &object.UserID, &object.APIKeyID,
		&object.ObjectRef.Provider, &object.ObjectRef.Bucket, &object.ObjectRef.ObjectKey,
		&object.ObjectRef.ContentType, &object.ObjectRef.SizeBytes, &object.ObjectRef.ChecksumSHA256,
		&width, &height, &object.URLHash, &filename, &object.ExpiresAt,
		&object.CleanupClaimedAt, &object.CreatedAt,
	)
	if err != nil {
		return object, err
	}
	object.ObjectRef.Width = nullIntValue(width)
	object.ObjectRef.Height = nullIntValue(height)
	object.Filename = filename.String
	return object, nil
}

func normalizeAsyncImageCleanupLimit(limit int) int {
	if limit <= 0 || limit > 10000 {
		return 100
	}
	return limit
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullIntValue(value sql.NullInt64) int {
	if !value.Valid {
		return 0
	}
	return int(value.Int64)
}

func nullableTrimmedString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func requireAsyncImageCleanupDelete(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrAsyncImageInvalidTransition
	}
	return nil
}
