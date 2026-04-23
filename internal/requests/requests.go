// Package requests хранит журнал обращений пользователей (requests_log).
// Используется для async-режима рекомендаций и аудита вызовов.
package requests

import (
	"context"
	"errors"
)

//go:generate mockgen -source=requests.go -destination=mocks/mock_requests.go -package=mocks

var ErrNotFound = errors.New("request not found")

// Статусы обработки.
const (
	StatusAccepted   = "accepted"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// Request — запись в requests_log.
type Request struct {
	RequestID    string
	UserID       string
	Scenario     string
	Status       string
	AttemptCount int
	ErrorReason  string
	Result       []byte // JSON payload
}

// Repository описывает журнал запросов.
type Repository interface {
	Create(ctx context.Context, req Request) error
	UpdateStatus(ctx context.Context, requestID, status string, result []byte, errReason string) error
	Get(ctx context.Context, requestID string) (Request, error)
}
