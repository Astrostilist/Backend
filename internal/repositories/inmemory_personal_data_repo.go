package repositories

import (
	"astroapi/internal/domain"
	"log"
)

type inMemoryPersonalDataRepo struct{}

func NewInMemoryPersonalDataRepository() PersonalDataRepository {
	return &inMemoryPersonalDataRepo{}
}

func (r *inMemoryPersonalDataRepo) Save(data domain.PersonalData) error {
	// обработка данных без сохранения
	log.Printf("[IN-MEMORY] Processed personal data for user %s (not persisted)", data.UserID)
	return nil
}
