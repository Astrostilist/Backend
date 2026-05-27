package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

//go:generate mockgen -source=health.go -destination=mocks/mock_health.go -package=mocks

// Response — общая обёртка для JSON-ответов сервиса.
type HealthResponse struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// HealthService абстрагирует проверку состояния зависимостей (например, БД).
type HealthService interface {
	GetInfraStatus(ctx context.Context) (any, error)
}

// HealthHandler обслуживает health-эндпоинт.
type HealthHandler struct {
	service HealthService
}

func NewHealthHandler(s HealthService) *HealthHandler {
	return &HealthHandler{service: s}
}

func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
	defer cancel()
	infrastate, infraerr := h.service.GetInfraStatus(ctx)

	if infraerr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)

		respBody := HealthResponse{
			Data:  infrastate,
			Error: infraerr.Error(),
		}

		if err := json.NewEncoder(w).Encode(respBody); err != nil {
			return
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	respBody := HealthResponse{
		Data: infrastate,
	}
	if err := json.NewEncoder(w).Encode(respBody); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
