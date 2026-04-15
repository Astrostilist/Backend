package usecases

import (
	"astroapi/internal/domain"
	"astroapi/internal/repositories"
)

type ProcessPersonalDataUseCase struct {
	dbRepo    repositories.PersonalDataRepository // умеет сохранять в базу
	inMemRepo repositories.PersonalDataRepository // умеет «сохранять» в память
}

func (uc *ProcessPersonalDataUseCase) Execute(user domain.User, data domain.PersonalData) error {
	if user.ConsentGiven {
		return uc.dbRepo.Save(data) // пишем в БД
	}
	return uc.inMemRepo.Save(data) // пишем только в память
}
