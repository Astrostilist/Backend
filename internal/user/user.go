// Package user хранит пользователей сервиса с шифрованием даты рождения.
// Дата рождения шифруется AES-GCM ключом из ENCRYPTION_KEY и сохраняется
// в колонку users.encrypted_dob (BYTEA).
package user

import (
	"context"
	"errors"
)

//go:generate mockgen -source=user.go -destination=mocks/mock_user.go -package=mocks

// ErrNotFound возвращается, когда пользователь не найден.
var ErrNotFound = errors.New("user not found")

// User — доменная сущность: дата рождения уже расшифрована.
type User struct {
	UserID       string
	BirthDate    string // ISO 8601 (YYYY-MM-DD), plaintext в памяти
	BirthTame    string
	ConsentGiven bool
}

// Repository — абстракция над хранилищем пользователей.
type Repository interface {
	Save(ctx context.Context, user User) error
	Get(ctx context.Context, userID string) (User, error)
}
