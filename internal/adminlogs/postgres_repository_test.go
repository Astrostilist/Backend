package adminlogs_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"astroapi/internal/adminlogs"
	"astroapi/internal/requests"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// newAdminLogsRepoTest создает sqlmock-репозиторий для тестов.
// На вход принимает тест, на выход возвращает репозиторий, БД и mock.
func newAdminLogsRepoTest(t *testing.T) (*adminlogs.PostgresRepository, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	return adminlogs.NewPostgresRepository(db), db, mock
}

// TestListFiltersByFailedStatus проверяет выборку только failed-записей по статусу.
// На вход принимает тестовый контекст, на выход проверяет результат репозитория.
func TestListFiltersByFailedStatus(t *testing.T) {
	t.Parallel()

	repository, db, mock := newAdminLogsRepoTest(t)
	defer db.Close()

	createdAt := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Minute)

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM requests_log`).
		WithArgs(requests.StatusFailed, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"request_id", "user_id", "status", "error_reason", "created_at", "completed_at",
	}).AddRow("req-1", "user-1", requests.StatusFailed, "failed reason", createdAt, completedAt)

	mock.ExpectQuery(`SELECT request_id, user_id, status`).
		WithArgs(requests.StatusFailed, nil, nil, 50).
		WillReturnRows(rows)

	result, err := repository.List(context.Background(), adminlogs.ListOptions{
		Status: requests.StatusFailed,
		Limit:  50,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Items, 1)
	require.Equal(t, requests.StatusFailed, result.Items[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestListTruncatesErrorMessage проверяет обрезку error_message до 500 символов.
// На вход принимает тестовый контекст, на выход проверяет длину ошибки в результате.
func TestListTruncatesErrorMessage(t *testing.T) {
	t.Parallel()

	repository, db, mock := newAdminLogsRepoTest(t)
	defer db.Close()

	createdAt := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	longError := strings.Repeat("я", 501)

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM requests_log`).
		WithArgs("", nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"request_id", "user_id", "status", "error_reason", "created_at", "completed_at",
	}).AddRow("req-1", "user-1", requests.StatusFailed, longError, createdAt, nil)

	mock.ExpectQuery(`SELECT request_id, user_id, status`).
		WithArgs("", nil, nil, 50).
		WillReturnRows(rows)

	result, err := repository.List(context.Background(), adminlogs.ListOptions{Limit: 50})

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Len(t, []rune(result.Items[0].ErrorMessage), 500)
	require.NoError(t, mock.ExpectationsWereMet())
}
