package usecases

import (
	"astroapi/internal/repositories"
	"astroapi/internal/repositories/domain"
	"context"
)

type ProcessPersonalDataUseCase struct {
	dbRepo    repositories.PersonalDataRepository // умеет сохранять в базу
	cacheRepo repositories.PersonalDataRepository // умеет «сохранять» в память
}

// конструктор зависимостей
func NewProcessPersonalDataUseCase(
	dbRepo repositories.PersonalDataRepository,
	cacheRepo repositories.PersonalDataRepository,
) *ProcessPersonalDataUseCase {
	return &ProcessPersonalDataUseCase{
		dbRepo:    dbRepo,
		cacheRepo: cacheRepo,
	}
}

// входные данные для usecase
type ProcessPersonalDataInput struct {
	PersonalData domain.PersonalData // пользователь с флагом ConsentGiven
}

func (uc *ProcessPersonalDataUseCase) Execute(ctx context.Context, input ProcessPersonalDataInput) error {
	if input.PersonalData.ConsentGiven {
		return uc.dbRepo.Save(ctx, input.PersonalData) // пишем в БД
	}
	return uc.cacheRepo.Save(ctx, input.PersonalData) // пишем только в память
}
