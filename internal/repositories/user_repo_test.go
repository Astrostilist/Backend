package repositories

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func setupUserRepoMock(t *testing.T) (sqlmock.Sqlmock, UserRepository, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	cleanup := func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}

	return mock, NewUserRepository(db), cleanup
}

func expectRelatedUserDeletes(mock sqlmock.Sqlmock, userID string) {
	mock.ExpectExec(regexp.QuoteMeta(queryDeleteFromRequestsLog)).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(queryDeleteFromAstroProfiles)).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// успешное удаление
func TestDeleteUser_Success(t *testing.T) {
	mock, repo, cleanup := setupUserRepoMock(t)
	defer cleanup()
	userID := "user-123"

	mock.ExpectBegin()
	expectRelatedUserDeletes(mock, userID)
	mock.ExpectExec(regexp.QuoteMeta(queryDeleteFromUsers)).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	found, err := repo.DeleteUsers(context.Background(), userID)
	require.NoError(t, err)
	require.True(t, found)
}

// удаление несуществующего → возвращает found=false
func TestDeleteUser_NotFound(t *testing.T) {
	mock, repo, cleanup := setupUserRepoMock(t)
	defer cleanup()
	userID := "user-456"

	mock.ExpectBegin()
	expectRelatedUserDeletes(mock, userID)
	mock.ExpectExec(regexp.QuoteMeta(queryDeleteFromUsers)).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	found, err := repo.DeleteUsers(context.Background(), userID)
	require.NoError(t, err)
	require.False(t, found)
}

// rollback при ошибке удаления requests_log
func TestDeleteUser_RollbackOnError(t *testing.T) {
	mock, repo, cleanup := setupUserRepoMock(t)
	defer cleanup()
	userID := "user-123"
	expectedErr := errors.New("delete requests_log failed")

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(queryDeleteFromRequestsLog)).
		WithArgs(userID).
		WillReturnError(expectedErr)
	mock.ExpectRollback()

	found, err := repo.DeleteUsers(context.Background(), userID)
	require.ErrorIs(t, err, expectedErr)
	require.False(t, found)
}
