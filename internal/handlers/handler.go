package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"astroapi/internal/database"
)

//go:generate mockgen -source=handler.go -destination=mocks/mock_hello.go -package=mocks

// Response — общая обёртка для JSON-ответов сервиса.
type Response struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HelloService абстрагирует проверку состояния зависимостей (например, БД).
type HelloService interface {
	GetDBStatus(ctx context.Context) string
}

// HelloHandler обслуживает корневой health-подобный эндпоинт.
type HelloHandler struct {
	service HelloService
}

func NewHelloHandler(s HelloService) *HelloHandler {
	return &HelloHandler{service: s}
}

func (h *HelloHandler) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbStatus := h.service.GetDBStatus(ctx)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(Response{
		Message: "Hello world",
		Data:    map[string]any{"database_status": dbStatus},
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// RealHelloService использует реальное соединение с PostgreSQL.
type RealHelloService struct {
	db *database.PostgresDB
}

func NewRealHelloService(db *database.PostgresDB) *RealHelloService {
	return &RealHelloService{db: db}
}

func (s *RealHelloService) GetDBStatus(ctx context.Context) string {
	if s == nil || s.db == nil {
		return "not initialized"
	}
	if err := s.db.Ping(ctx); err != nil {
		return "disconnected"
	}
	return "connected"
}
