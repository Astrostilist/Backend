package repositories

import (
	"astroapi/internal/crypto"
	"astroapi/internal/repositories/domain"
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
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
	tracer := otel.Tracer("db-repo")
	repoctx, repoSpan := tracer.Start(ctx, "user-profile.Save")
	defer repoSpan.End()

	encryptedDob, err := crypto.Encrypt(data.DOB, r.key)
	if err != nil {
		repoSpan.RecordError(err)
		return err
	}
	now := time.Now()
	query := `INSERT INTO users (user_id, encrypted_dob, consent_given, created_at, updated_at)
              VALUES ($1, $2, $3, $4, $5)
              ON CONFLICT (user_id) DO UPDATE SET
              encrypted_dob = EXCLUDED.encrypted_dob,
              updated_at = EXCLUDED.updated_at;`
	_, err = r.db.ExecContext(repoctx, query, data.UserID, encryptedDob, data.ConsentGiven, now, now)
	if err != nil {
		err := fmt.Errorf("error adding data to the users table: %w", err)
		repoSpan.RecordError(err)
		return err
	}
	return nil
}
