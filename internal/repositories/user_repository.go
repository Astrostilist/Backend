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
	query := []string{queryDeleteFromUserConsents, queryDeleteFromGenerationResults, queryDeleteFromFeedback}
	for _, i := range query {
		if _, err := tx.ExecContext(ctx, i, userID); err != nil {
			_ = tx.Rollback()
			return false, err
		}
	}
	req, err := tx.ExecContext(ctx, queryDeleteFromUsers, userID)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	rows, err := req.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	// проверка было ли что-то удалено
	if rows == 0 {
		return false, nil
	}
	return true, tx.Commit()
}
