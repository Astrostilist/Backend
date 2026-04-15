package repositories

import "astroapi/internal/domain"

// PersonalDataRepository – порт для сохранения персональных данных
type PersonalDataRepository interface {
	Save(data domain.PersonalData) error
}
