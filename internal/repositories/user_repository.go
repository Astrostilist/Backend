//go:generate mockgen -destination=mock_user_repository.go -package=repositories . UserRepository
package repositories

import (
	"context"
	"database/sql"
)

const queryDeleteFromUsers = `DELETE FROM users WHERE user_id = $1`
const queryDeleteFromUserConsents = `DELETE FROM user_consents WHERE user_id = $1`
const queryDeleteFromGenerationResults = `DELETE FROM generation_results WHERE user_id = $1`
const queryDeleteFromFeedback = `DELETE FROM feedback WHERE user_id = $1`

type UserRepository interface {
	DeleteUsers(ctx context.Context, userID string) (found bool, err error)
}

type userRepo struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepo{db: db}
}

func (t *userRepo) DeleteUsers(ctx context.Context, userID string) (bool, error) {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}

	deleteQueries := []string{queryDeleteFromUserConsents, queryDeleteFromGenerationResults, queryDeleteFromFeedback}
	for _, query := range deleteQueries {
		if _, err := tx.ExecContext(ctx, query, userID); err != nil {
			_ = tx.Rollback()
			return false, err
		}
	}

	res, err := tx.ExecContext(ctx, queryDeleteFromUsers, userID)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if rows == 0 {
		if err := tx.Rollback(); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
