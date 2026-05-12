package handlers

import (
	"context"
	"net/http"
)

//go:generate mockgen -source=health.go -destination=mocks/mock_health.go -package=mocks

// Response — общая обёртка для JSON-ответов сервиса.
type HealthResponse struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HealthService абстрагирует проверку состояния зависимостей (например, БД).
type HealthService interface {
	GetDBStatus(ctx context.Context) string
}

// HealthHandler обслуживает health-эндпоинт.
type HealthHandler struct {
	service HealthService
}

func NewHealthHandler(s HealthService) *HealthHandler {
	return &HealthHandler{service: s}
}

func (h *HealthHandler) HealthHandler(w http.ResponseWriter, r *http.Request) {

}
