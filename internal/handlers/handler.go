package handlers

import (
	"encoding/json"
	"net/http"

	"astroapi/internal/database"
)

type Response struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HelloWorldHandler обрабатывает GET запрос на /api/v1/
func HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	// Проверяем только GET метод
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Проверяем подключение к БД
	var dbStatus string
	if database.DB != nil {
		if err := database.DB.Ping(); err == nil {
			dbStatus = "connected"
		} else {
			dbStatus = "disconnected"
		}
	} else {
		dbStatus = "not initialized"
	}

	// Устанавливаем заголовок Content-Type
	w.Header().Set("Content-Type", "application/json")

	// Создаем ответ
	response := Response{
		Message: "Hello world",
		Data: map[string]any{
			"database_status": dbStatus,
		},
	}

	// Кодируем в JSON и отправляем
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
