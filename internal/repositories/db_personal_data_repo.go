package repositories

import (
	"astroapi/internal/crypto"
	"astroapi/internal/repositories/domain"
	"context"
	"database/sql"
	"fmt"
	"time"
)

type dbPersonalDataRepo struct {
	db  *sql.DB
	key []byte //секретный ключ для шифрования DOB
}

func NewDBPersonalDataRepository(db *sql.DB, encryptionKey []byte) PersonalDataRepository {
	return &dbPersonalDataRepo{
		db:  db,
		key: encryptionKey,
	}
}

func (r *dbPersonalDataRepo) Save(ctx context.Context, data domain.PersonalData) error {
	encryptedDob, err := crypto.Encrypt(data.DOB, r.key)
	if err != nil {
		return err
	}
	now := time.Now()
	// сохраняем запись если такого пользователя нет, если пользователь с таким user_id уже есть, то редактируем запись
	query := `INSERT INTO users (user_id, encrypted_dob, consent_given, created_at, updated_at)
              VALUES ($1, $2, $3, $4, $5)
              ON CONFLICT (user_id) DO UPDATE SET
              encrypted_dob = EXCLUDED.encrypted_dob,
              updated_at = EXCLUDED.updated_at;`
	_, err = r.db.Exec(query, data.UserID, encryptedDob, data.ConsentGiven, now, now)
	if err != nil {
		return fmt.Errorf("ошибка добавление данных в таблицу users: %w", err)
	}
	return nil
}
