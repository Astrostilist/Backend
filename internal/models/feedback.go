package models

import (
	"errors"
	"time"
)

var (
	ErrRequestNotFound = errors.New("указанный request_id не найден")
	ErrFeedbackExists  = errors.New("отзыв для этого request_id уже существует")
)

type Feedback struct {
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
