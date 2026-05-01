package repositories

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"astroapi/internal/models"
)

type FeedbackRepository interface {
	Create(f *models.Feedback) error
}

type feedbackPG struct {
	db *sql.DB
}

func NewFeedbackRepository(db *sql.DB) FeedbackRepository {
	return &feedbackPG{db: db}
}

func (r *feedbackPG) Create(f *models.Feedback) error {
	query := `
		INSERT INTO feedback (id, request_id, rating, comment, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(context.Background(), query, f.ID, f.RequestID, f.Rating, f.Comment, f.CreatedAt)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "unique constraint") || strings.Contains(errStr, "23505") {
			return errors.New("отзыв для этого request_id уже существует")
		}
		if strings.Contains(errStr, "foreign key constraint") || strings.Contains(errStr, "23503") {
			return errors.New("указанный request_id не найден")
		}
		return err
	}
	return nil
}
