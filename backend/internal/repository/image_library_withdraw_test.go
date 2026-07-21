package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestWithdrawPublicationCannotOverrideAdminHiddenState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM image_library_items WHERE asset_id=\$1 AND user_id=\$2`).
		WithArgs("img_hidden", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(17)))
	mock.ExpectQuery(`(?s)SELECT id,status FROM image_plaza_publications.*status IN \('pending_review','published'\).*FOR UPDATE`).
		WithArgs(int64(17), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectRollback()

	repo := NewImageLibraryRepository(db).(*imageLibraryRepository)
	err = repo.WithdrawPublication(context.Background(), 42, "img_hidden")
	require.ErrorIs(t, err, service.ErrImagePublicationNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
