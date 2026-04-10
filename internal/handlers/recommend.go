package handlers

import (
	"astroapi/internal/messaging"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// RecommendRequest описывает ожидаемый JSON для получения рекомендаций
type RecommendRequest struct {
	UserID   string                 `json:"user_id"`
	Scenario string                 `json:"scenario"`
	Context  map[string]interface{} `json:"context,omitempty"`
	Mode     string                 `json:"mode,omitempty"`
}

// Создаем глобальный словарь разрешенных сценариев
var validScenarios = map[string]bool{
	"personal_style": true,
	"perfect_gift":   true,
}

// Validate проверяет входные данные на соответствие ТЗ
func (req *RecommendRequest) Validate() map[string]string {
	errors := make(map[string]string)

	if _, err := uuid.Parse(req.UserID); err != nil {
		errors["user_id"] = "must be a valid UUID format"
	}

	if !validScenarios[req.Scenario] {
		errors["scenario"] = "scenario must be either 'personal_style' or 'perfect_gift'"
	}

	// Если клиент ничего не передал (пустая строка), ставим режим по умолчанию
	if req.Mode == "" {
		req.Mode = "async"
	} else if req.Mode != "sync" && req.Mode != "async" {
		// Если клиент передал что-то, кроме sync или async, выдаем ошибку
		errors["mode"] = "mode must be either 'sync' or 'async'"
	}

	return errors
}

func RecommendHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Проверка метода
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. Защита от OOM
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	// 3. Декодирование JSON
	var req RecommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json format"}`, http.StatusBadRequest)
		return
	}

	// 4. Валидация
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "validation_failed",
			"details": validationErrors,
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// 5. Генерируем ID
	requestID := uuid.New().String()

	payload, err := json.Marshal(req)
	if err != nil {
		http.Error(w, `{"error": "failed to encode payload"}`, http.StatusInternalServerError)
		return
	}

	if req.Mode == "async" {
		if messaging.JS != nil {
			_, err = messaging.JS.Publish(r.Context(), "astro.events.recommend", payload)
			if err != nil {
				http.Error(w, `{"error": "failed to publish event to NATS"}`, http.StatusInternalServerError)
				return
			}
		}

		// Отдаем 202 Accepted
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"request_id": requestID,
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-time.After(2 * time.Second):
		// Имитация успешного ответа от нейросети через 2 секунды
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"request_id": requestID,
			"result":     "Здесь будет крутая рекомендация от AlisaAI",
			"status":     "completed",
		}); err != nil {
			// Если вдруг не получилось закодировать, отдаем ошибку сервера
			http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
		}

	case <-ctx.Done():
		http.Error(w, `{"error": "timeout: AI service is taking too long"}`, http.StatusGatewayTimeout)
	}
}
