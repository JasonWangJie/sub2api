package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageInputObjectRepositoryPersistsOwnerAndStableRef(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)
	mock.ExpectQuery("(?s)INSERT INTO async_image_input_objects .*RETURNING").
		WithArgs(
			"asyncimg_upload", int64(10), int64(20), service.ImageStorageProviderAliyun,
			"images", "inputs/20/upload.png", "image/png", int64(12), "checksum",
			640, 480, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"reference.png", expiresAt,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "upload_id", "user_id", "api_key_id", "provider", "bucket", "object_key",
			"content_type", "byte_size", "checksum", "width", "height", "url_hash", "filename",
			"expires_at", "cleanup_claimed_at", "created_at",
		}).AddRow(
			int64(1), "asyncimg_upload", int64(10), int64(20), service.ImageStorageProviderAliyun,
			"images", "inputs/20/upload.png", "image/png", int64(12), "checksum", 640, 480,
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "reference.png",
			expiresAt, nil, now,
		))

	repo := NewAsyncImageTaskRepository(db).(*asyncImageTaskRepository)
	object, err := repo.RegisterAsyncImageInputObject(context.Background(), service.RegisterAsyncImageInputObjectParams{
		UploadID: "asyncimg_upload", UserID: 10, APIKeyID: 20,
		ObjectRef: service.ObjectRef{
			Provider: service.ImageStorageProviderAliyun, Bucket: "images", ObjectKey: "inputs/20/upload.png",
			ContentType: "image/png", SizeBytes: 12, ChecksumSHA256: "checksum", Width: 640, Height: 480,
		},
		URLHash:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Filename: "reference.png", ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	require.Equal(t, int64(20), object.APIKeyID)
	require.Equal(t, "inputs/20/upload.png", object.ObjectRef.ObjectKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimExpiredInputsExcludesActiveTaskReferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery("(?s)NOT EXISTS.*async_image_task_inputs.*t.status IN.*FOR UPDATE OF i SKIP LOCKED.*UPDATE async_image_input_objects").
		WithArgs(now, now.Add(-30*time.Minute), 25).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "upload_id", "user_id", "api_key_id", "provider", "bucket", "object_key",
			"content_type", "byte_size", "checksum", "width", "height", "url_hash", "filename",
			"expires_at", "cleanup_claimed_at", "created_at",
		}))

	repo := NewAsyncImageTaskRepository(db).(*asyncImageTaskRepository)
	objects, err := repo.ClaimExpiredAsyncImageInputObjects(context.Background(), now, now.Add(-30*time.Minute), 25)
	require.NoError(t, err)
	require.Empty(t, objects)
	require.NoError(t, mock.ExpectationsWereMet())
}
