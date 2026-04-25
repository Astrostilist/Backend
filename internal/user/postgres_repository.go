package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"astroapi/internal/crypto"
)

// PostgresRepository — реализация Repository поверх PostgreSQL.
type PostgresRepository struct {
	db            *sql.DB
	encryptionKey []byte
}

// NewPostgresRepository создаёт репозиторий. Ключ должен быть 32 байта (AES-256).
func NewPostgresRepository(db *sql.DB, encryptionKey []byte) *PostgresRepository {
	return &PostgresRepository{db: db, encryptionKey: encryptionKey}
}

// Save делает upsert пользователя, шифруя birth_date перед записью.
func (r *PostgresRepository) Save(ctx context.Context, u User) error {
	encryptedDOB, err := crypto.Encrypt(u.BirthDate, r.encryptionKey)
	if err != nil {
		return fmt.Errorf("encrypt birth_date: %w", err)
	}

	const query = `
		INSERT INTO users (user_id, encrypted_dob, consent_given)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET encrypted_dob = EXCLUDED.encrypted_dob,
		    consent_given = EXCLUDED.consent_given,
		    updated_at    = CURRENT_TIMESTAMP
	`
	if _, err := r.db.ExecContext(ctx, query, u.UserID, encryptedDOB, u.ConsentGiven); err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

// Get читает пользователя и расшифровывает birth_date.
func (r *PostgresRepository) Get(ctx context.Context, userID string) (User, error) {
	const query = `
		SELECT user_id, encrypted_dob, consent_given
		FROM users
		WHERE user_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, userID)

	var (
		u            User
		encryptedDOB []byte
	)
	if err := row.Scan(&u.UserID, &encryptedDOB, &u.ConsentGiven); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("scan user: %w", err)
	}

	birthDate, err := crypto.Decrypt(encryptedDOB, r.encryptionKey)
	if err != nil {
		return User{}, fmt.Errorf("decrypt birth_date: %w", err)
	}
	u.BirthDate = birthDate
	return u, nil
}
