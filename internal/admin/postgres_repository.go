package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLength = 8

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) HasSuperAdmin(ctx context.Context) (bool, error) {
	const query = `
		SELECT EXISTS(
			SELECT 1
			FROM admin_users
			WHERE role = $1 AND is_active = TRUE
		)
	`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, RoleSuperAdmin).Scan(&exists); err != nil {
		return false, fmt.Errorf("check superadmin exists: %w", err)
	}
	return exists, nil
}

func (r *PostgresRepository) CreateSuperAdmin(ctx context.Context, input CreateSuperAdminInput) (User, error) {
	email, password, err := validateCreateInput(input)
	if err != nil {
		return User{}, err
	}

	exists, err := r.HasSuperAdmin(ctx)
	if err != nil {
		return User{}, err
	}
	if exists {
		return User{}, ErrSuperAdminExists
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash superadmin password: %w", err)
	}

	const query = `
		INSERT INTO admin_users (email, password_hash, role, is_active)
		VALUES ($1, $2, $3, TRUE)
		RETURNING id::text, email, password_hash, role, is_active
	`

	var created User
	err = r.db.QueryRowContext(ctx, query, email, string(passwordHash), RoleSuperAdmin).Scan(
		&created.ID,
		&created.Email,
		&created.PasswordHash,
		&created.Role,
		&created.IsActive,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrSuperAdminExists
		}
		return User{}, fmt.Errorf("create superadmin: %w", err)
	}
	return created, nil
}

func (r *PostgresRepository) VerifyCredentials(ctx context.Context, email, password string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return User{}, ErrInvalidCredential
	}

	const query = `
		SELECT id::text, email, password_hash, role, is_active
		FROM admin_users
		WHERE LOWER(email) = LOWER($1) AND is_active = TRUE
	`

	var found User
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&found.ID,
		&found.Email,
		&found.PasswordHash,
		&found.Role,
		&found.IsActive,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrInvalidCredential
		}
		return User{}, fmt.Errorf("get admin user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)) != nil {
		return User{}, ErrInvalidCredential
	}
	return found, nil
}

func validateCreateInput(input CreateSuperAdminInput) (string, string, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := strings.TrimSpace(input.Password)
	valid := email != "" && strings.Contains(email, "@") && len(password) >= minPasswordLength
	if !valid {
		return "", "", ErrInvalidInput
	}
	return email, password, nil
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
