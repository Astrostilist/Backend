package repositories

import (
	"astroapi/internal/domain"
	"database/sql"
)

type dbPersonalDataRepo struct {
	db *sql.DB
}

func NewDBPersonalDataRepository(db *sql.DB) PersonalDataRepository {
	return &dbPersonalDataRepo{db: db}
}

func (r *dbPersonalDataRepo) Save(data domain.PersonalData) error {
	//сохранение данных в бд
	return nil
}
