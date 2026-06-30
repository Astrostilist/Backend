package usecases

import (
	"context"
	"errors"

	"astroapi/internal/repositories"
	"astroapi/internal/repositories/domain"
)

var ErrNotFound = errors.New("item not found")

//go:generate mockgen -source=astro_profile.go -destination=mocks/mock_astro_profile.go -package=mocks
type ProcessAstroProfileInterface interface {
	ExecuteSave(ctx context.Context, profile domain.AstroProfile) error
	ExecuteReceivingByHash(ctx context.Context, hash string) (*domain.AstroProfile, error)
}

type ProcessAstroProfileUseCase struct {
	dbRepo    repositories.AstroProfileRepository
	cacheRepo repositories.AstroProfileRepository
}

func NewProcessAstroProfileUseCase(
	dbRepo repositories.AstroProfileRepository,
	cacheRepo repositories.AstroProfileRepository,
) *ProcessAstroProfileUseCase {
	return &ProcessAstroProfileUseCase{
		dbRepo:    dbRepo,
		cacheRepo: cacheRepo,
	}
}

func (uc *ProcessAstroProfileUseCase) ExecuteSave(ctx context.Context, profile domain.AstroProfile) error {
	if profile.ConsentGiven {
		return uc.dbRepo.Save(ctx, profile)
	}
	return uc.cacheRepo.Save(ctx, profile)
}

func (uc *ProcessAstroProfileUseCase) ExecuteReceivingByHash(ctx context.Context, hash string) (*domain.AstroProfile, error) {
	profile, err := uc.cacheRepo.ReceivingByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		profile, err = uc.dbRepo.ReceivingByHash(ctx, hash)
		if err != nil {
			return nil, err
		}
		if profile == nil {
			return nil, ErrNotFound
		}
	}
	return profile, nil
}
