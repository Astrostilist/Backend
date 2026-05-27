package repositories

//go:generate mockgen -destination=mocks/mock_personal_data_repo.go -package=mocks . PersonalDataRepository

import (
	"astroapi/internal/repositories/domain"
	"context"
)

// PersonalDataRepository – порт для сохранения персональных данных
type PersonalDataRepository interface {
	Save(ctx context.Context, data domain.PersonalData) error
}
