package admin

import (
	"context"
	"errors"
)

const RoleSuperAdmin = "super_admin"

var (
	ErrSuperAdminExists  = errors.New("superadmin already exists")
	ErrInvalidCredential = errors.New("invalid admin credentials")
	ErrInvalidInput      = errors.New("invalid admin input")
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	IsActive     bool
}

type CreateSuperAdminInput struct {
	Email    string
	Password string
}

type Repository interface {
	HasSuperAdmin(ctx context.Context) (bool, error)
	CreateSuperAdmin(ctx context.Context, input CreateSuperAdminInput) (User, error)
	VerifyCredentials(ctx context.Context, email, password string) (User, error)
}
