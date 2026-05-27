package models

import "time"

type Recommendation struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Scenario   string    `json:"scenario"`
	Status     string    `json:"status"`
	ResultText *string   `json:"result_text,omitempty"` // Указатель, так как текст может быть NULL (пока ИИ думает)
	CreatedAt  time.Time `json:"created_at"`
}
