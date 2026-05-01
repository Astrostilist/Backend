package models

import "time"

type Feedback struct {
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
