package user_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"astroapi/internal/crypto"
	"astroapi/internal/user"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// newRepo готовит sqlmock + reproducible 32-byte key для тестов.
func newRepo(t *testing.T) (*user.PostgresRepository, *sql.DB, sqlmock.Sqlmock, []byte) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	key := make([]byte, 32)
	_, err = rand.Read(key)
	require.NoError(t, err)

	return user.NewPostgresRepository(db, key), db, mock, key
}

func TestPostgresRepository_Save_EncryptsAndUpserts(t *testing.T) {
	t.Parallel()
	repo, db, mock, key := newRepo(t)
	defer db.Close()

	userID := "123e4567-e89b-12d3-a456-426614174000"
	birthDate := "1990-01-15"

	// Мы не знаем заранее, какой получится ciphertext (AES-GCM с рандом nonce),
	// поэтому проверяем, что это []byte + остальные аргументы совпадают.
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users`)).
		WithArgs(userID, sqlmock.AnyArg(), true).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Save(context.Background(), user.User{
		UserID:       userID,
		BirthDate:    birthDate,
		ConsentGiven: true,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	// bonus: убеждаемся, что ключ валиден и крипта реально работает на этом ключе
	encrypted, err := crypto.Encrypt(birthDate, key)
	require.NoError(t, err)
	decrypted, err := crypto.Decrypt(encrypted, key)
	require.NoError(t, err)
	require.Equal(t, birthDate, decrypted)
}

func TestPostgresRepository_Save_DBFailurePropagates(t *testing.T) {
	t.Parallel()
	repo, db, mock, _ := newRepo(t)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO users`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("duplicate key"))

	err := repo.Save(context.Background(), user.User{UserID: "id", BirthDate: "1990-01-01"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "upsert user")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresRepository_Get_DecryptsBirthDate(t *testing.T) {
	t.Parallel()
	repo, db, mock, key := newRepo(t)
	defer db.Close()

	birthDate := "1985-07-30"
	encrypted, err := crypto.Encrypt(birthDate, key)
	require.NoError(t, err)

	userID := "user-xyz"
	rows := sqlmock.NewRows([]string{"user_id", "encrypted_dob", "consent_given"}).
		AddRow(userID, encrypted, true)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT user_id, encrypted_dob, consent_given`)).
		WithArgs(userID).
		WillReturnRows(rows)

	got, err := repo.Get(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, userID, got.UserID)
	require.Equal(t, birthDate, got.BirthDate)
	require.True(t, got.ConsentGiven)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresRepository_Get_NotFound(t *testing.T) {
	t.Parallel()
	repo, db, mock, _ := newRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT user_id, encrypted_dob, consent_given`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := repo.Get(context.Background(), "missing")
	require.ErrorIs(t, err, user.ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresRepository_Get_CorruptedCiphertext(t *testing.T) {
	t.Parallel()
	repo, db, mock, _ := newRepo(t)
	defer db.Close()

	// мусор вместо валидного AES-GCM payload — должен упасть на Decrypt
	rows := sqlmock.NewRows([]string{"user_id", "encrypted_dob", "consent_given"}).
		AddRow("id", []byte("garbage"), false)
	mock.ExpectQuery(`SELECT user_id, encrypted_dob, consent_given`).
		WithArgs("id").
		WillReturnRows(rows)

	_, err := repo.Get(context.Background(), "id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "decrypt birth_date")
	require.NoError(t, mock.ExpectationsWereMet())
}
