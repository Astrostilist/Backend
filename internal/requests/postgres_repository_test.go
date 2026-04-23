package requests_test

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"astroapi/internal/requests"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func newRepo(t *testing.T) (*requests.PostgresRepository, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	return requests.NewPostgresRepository(db), db, mock
}

func TestCreate_Success(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO requests_log`)).
		WithArgs("req-1", "u-1", "profile", requests.StatusAccepted, 0).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(context.Background(), requests.Request{
		RequestID: "req-1",
		UserID:    "u-1",
		Scenario:  "profile",
		Status:    requests.StatusAccepted,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreate_DBError(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO requests_log`).
		WillReturnError(errors.New("conn refused"))

	err := repo.Create(context.Background(), requests.Request{RequestID: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insert requests_log")
}

func TestUpdateStatus_Completed_WithResult(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	payload := []byte(`{"text":"ok"}`)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE requests_log`)).
		WithArgs("req-1", requests.StatusCompleted, string(payload), "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateStatus(context.Background(), "req-1", requests.StatusCompleted, payload, "")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateStatus_Failed_WithReason(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	// payload=nil → resultArg=nil
	mock.ExpectExec(`UPDATE requests_log`).
		WithArgs("req-2", requests.StatusFailed, nil, "user not found").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateStatus(context.Background(), "req-2", requests.StatusFailed, nil, "user not found")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateStatus_NotFound(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	mock.ExpectExec(`UPDATE requests_log`).
		WithArgs("ghost", requests.StatusCompleted, nil, "").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.UpdateStatus(context.Background(), "ghost", requests.StatusCompleted, nil, "")
	require.ErrorIs(t, err, requests.ErrNotFound)
}

func TestGet_Found(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"request_id", "user_id", "scenario", "status", "attempt_count", "error_reason", "result_payload",
	}).AddRow("req-1", "u-1", "profile", requests.StatusCompleted, 1, "", `{"ok":true}`)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT request_id, user_id, scenario, status, attempt_count`)).
		WithArgs("req-1").
		WillReturnRows(rows)

	got, err := repo.Get(context.Background(), "req-1")
	require.NoError(t, err)
	require.Equal(t, "req-1", got.RequestID)
	require.Equal(t, "u-1", got.UserID)
	require.Equal(t, requests.StatusCompleted, got.Status)
	require.Equal(t, 1, got.AttemptCount)
	require.Equal(t, []byte(`{"ok":true}`), got.Result)
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT request_id`).
		WithArgs("nope").
		WillReturnError(sql.ErrNoRows)

	_, err := repo.Get(context.Background(), "nope")
	require.ErrorIs(t, err, requests.ErrNotFound)
}

func TestGet_EmptyResultPayload(t *testing.T) {
	t.Parallel()
	repo, db, mock := newRepo(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"request_id", "user_id", "scenario", "status", "attempt_count", "error_reason", "result_payload",
	}).AddRow("req-2", "u-1", "profile", requests.StatusAccepted, 0, "", "")

	mock.ExpectQuery(`SELECT request_id`).
		WithArgs("req-2").
		WillReturnRows(rows)

	got, err := repo.Get(context.Background(), "req-2")
	require.NoError(t, err)
	require.Nil(t, got.Result)
}
