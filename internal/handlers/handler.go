package handlers

import (
	"encoding/json"
	"net/http"
)

// Response остается без изменений
type Response struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Создаем интерфейс для сервисного слоя
type HelloService interface {
	GetDBStatus() string
}

// Создаем структуру хендлера, которая хранит сервис
type HelloHandler struct {
	service HelloService
}

// Конструктор для создания нового хендлера
func NewHelloHandler(s HelloService) *HelloHandler {
	return &HelloHandler{service: s}
}

func (h *HelloHandler) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := h.service.GetDBStatus()

	w.Header().Set("Content-Type", "application/json")

	response := Response{
		Message: "Hello world",
		Data: map[string]any{
			"database_status": dbStatus,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
