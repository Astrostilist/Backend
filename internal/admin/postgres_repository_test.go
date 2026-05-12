package admin_test

import (
	"context"
	"errors"
	"testing"

	"astroapi/internal/admin"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestCreateSuperAdmin_CreatesFirstAdmin(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := admin.NewPostgresRepository(db)

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(admin.RoleSuperAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	mock.ExpectQuery("INSERT INTO admin_users").
		WithArgs("root@example.com", sqlmock.AnyArg(), admin.RoleSuperAdmin).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "password_hash", "role", "is_active",
		}).AddRow("admin-id", "root@example.com", "$2a$10$hash", admin.RoleSuperAdmin, true))

	created, err := repo.CreateSuperAdmin(context.Background(), admin.CreateSuperAdminInput{
		Email:    " Root@Example.com ",
		Password: "secure-password",
	})

	require.NoError(t, err)
	require.Equal(t, "root@example.com", created.Email)
	require.Equal(t, admin.RoleSuperAdmin, created.Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateSuperAdmin_SkipsDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := admin.NewPostgresRepository(db)

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(admin.RoleSuperAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = repo.CreateSuperAdmin(context.Background(), admin.CreateSuperAdminInput{
		Email:    "root@example.com",
		Password: "secure-password",
	})

	require.True(t, errors.Is(err, admin.ErrSuperAdminExists))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyCredentials_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secure-password"), bcrypt.MinCost)
	require.NoError(t, err)

	repo := admin.NewPostgresRepository(db)
	mock.ExpectQuery("SELECT id::text, email, password_hash, role, is_active").
		WithArgs("root@example.com").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "password_hash", "role", "is_active",
		}).AddRow("admin-id", "root@example.com", string(passwordHash), admin.RoleSuperAdmin, true))

	found, err := repo.VerifyCredentials(context.Background(), "root@example.com", "secure-password")

	require.NoError(t, err)
	require.Equal(t, admin.RoleSuperAdmin, found.Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyCredentials_WrongPassword(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secure-password"), bcrypt.MinCost)
	require.NoError(t, err)

	repo := admin.NewPostgresRepository(db)
	mock.ExpectQuery("SELECT id::text, email, password_hash, role, is_active").
		WithArgs("root@example.com").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "password_hash", "role", "is_active",
		}).AddRow("admin-id", "root@example.com", string(passwordHash), admin.RoleSuperAdmin, true))

	_, err = repo.VerifyCredentials(context.Background(), "root@example.com", "bad-password")

	require.True(t, errors.Is(err, admin.ErrInvalidCredential))
	require.NoError(t, mock.ExpectationsWereMet())
}
