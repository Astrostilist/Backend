package repositories

import (
	"context"

	"astroapi/internal/repositories/domain"
)

type AstroProfileRepository interface {
	ReceivingByHash(ctx context.Context, hash string) (*domain.AstroProfile, error)
	Save(ctx context.Context, profile domain.AstroProfile) error
}
