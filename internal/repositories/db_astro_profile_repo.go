package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"

	"astroapi/internal/crypto"
	"astroapi/internal/repositories/domain"
)

type dbAstroProfileRepo struct {
	db  *sql.DB
	key []byte
}

func NewDbAstroProfileRepo(db *sql.DB, encryptionKey []byte) AstroProfileRepository {
	return &dbAstroProfileRepo{
		db:  db,
		key: encryptionKey,
	}
}

func (r *dbAstroProfileRepo) Save(ctx context.Context, profile domain.AstroProfile) error {
	tracer := otel.Tracer("db-astro-profile-repo")
	repoctx, repoSpan := tracer.Start(ctx, "astro-profile.Save")
	defer repoSpan.End()

	encryptedDob, err := crypto.Encrypt(profile.DOB, r.key)
	if err != nil {
		err = fmt.Errorf("data encryption failed: %w", err)
		repoSpan.RecordError(err)
		return err
	}

	profileDataJSON, err := json.Marshal(profile.ProfileData)
	if err != nil {
		err = fmt.Errorf("failed to marshal profile data: %w", err)
		repoSpan.RecordError(err)
		return err
	}

	now := time.Now()
	query := `INSERT INTO astro_profiles (id, user_id, profile_hash, dob_encrypted, consent_given, profile_data, created_at)
              VALUES ($1, $2, $3, $4, $5, $6, $7)
              ON CONFLICT (user_id, profile_hash) DO UPDATE SET
              dob_encrypted = EXCLUDED.dob_encrypted,
              consent_given = EXCLUDED.consent_given,
              profile_data = EXCLUDED.profile_data;`
	_, err = r.db.ExecContext(repoctx, query, profile.ID, profile.UserID, profile.ProfileHash, encryptedDob, profile.ConsentGiven, profileDataJSON, now)
	if err != nil {
		err := fmt.Errorf("error adding data to the astro_profile table: %w", err)
		repoSpan.RecordError(err)
		return err
	}

	return nil
}

func (r *dbAstroProfileRepo) ReceivingByHash(ctx context.Context, hash string) (_ *domain.AstroProfile, retErr error) {
	tracer := otel.Tracer("db-astro-profile-repo")
	repoctx, repoSpan := tracer.Start(ctx, "astro-profile.ReceivingByHas")
	defer repoSpan.End()

	if hash == "" {
		err := errors.New("profile hash must not be empty")
		repoSpan.RecordError(err)
		return nil, err
	}
	query := `SELECT id, user_id, profile_hash, dob_encrypted, consent_given, profile_data
	          FROM astro_profiles
			  WHERE profile_hash = $1`
	rows, err := r.db.QueryContext(repoctx, query, hash)
	if err != nil {
		err := fmt.Errorf("error creating request: %w", err)
		repoSpan.RecordError(err)
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("close rows: %w", closeErr)
			repoSpan.RecordError(retErr)
		}
	}()

	var p *domain.AstroProfile
	var encryptedDob []byte
	if rows.Next() {
		p = &domain.AstroProfile{}
		var profileDataJSON []byte
		err := rows.Scan(&p.ID, &p.UserID, &p.ProfileHash, &encryptedDob, &p.ConsentGiven, &profileDataJSON)
		if err != nil {
			err := fmt.Errorf("data scanning error: %w", err)
			repoSpan.RecordError(err)
			return nil, err
		}
		p.DOB, err = crypto.Decrypt(encryptedDob, r.key)
		if err != nil {
			err := fmt.Errorf("data decryption error: %w", err)
			repoSpan.RecordError(err)
			return nil, err
		}
		if err = json.Unmarshal(profileDataJSON, &p.ProfileData); err != nil {
			err = fmt.Errorf("failed to unmarshal profile data: %w", err)
			repoSpan.RecordError(err)
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		err := fmt.Errorf("error while iterating over rows: %w", err)
		repoSpan.RecordError(err)
		return nil, err
	}

	return p, nil
}
