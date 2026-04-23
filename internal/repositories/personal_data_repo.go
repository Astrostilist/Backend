package repositories

import (
	"astroapi/internal/repositories/domain"
	"context"
)

// PersonalDataRepository – порт для сохранения персональных данных
type PersonalDataRepository interface {
	Save(ctx context.Context, data domain.PersonalData) error
}
