// Package requests хранит журнал обращений пользователей и результаты генерации.
// requests_log используется для аудита, generation_results — для идемпотентности воркера.
package requests

import (
	"context"
	"errors"
)

//go:generate mockgen -source=requests.go -destination=mocks/mock_requests.go -package=mocks

var (
	ErrNotFound       = errors.New("request not found")
	ErrStatusConflict = errors.New("request status conflict")
)

// Статусы обработки.
const (
	StatusPending = "pending"
	StatusAccepted   = StatusPending
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// Request — состояние запроса и результата генерации по request_id.
type Request struct {
	RequestID    string
	UserID       string
	Scenario     string
	Status       string
	AttemptCount int
	ErrorReason  string
	Result       []byte // JSON payload
}

// Repository описывает журнал запросов и таблицу generation_results.
type Repository interface {
	Create(ctx context.Context, req Request) error
	StartProcessing(ctx context.Context, requestID string) (bool, error)
	UpdateStatus(ctx context.Context, requestID, status string, result []byte, errReason string) error
	Get(ctx context.Context, requestID string) (Request, error)
}
