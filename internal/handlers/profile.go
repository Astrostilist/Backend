package handlers

import (
	"astroapi/internal/messaging"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ProfileRequest struct {
	UserID       string `json:"user_id"`
	BirthDate    string `json:"birth_date"`
	BirthTime    string `json:"birth_time,omitempty"`
	BirthPlace   string `json:"birth_place"`
	ConsentGiven bool   `json:"consent_given"`
}

func (req *ProfileRequest) Validate() map[string]string {
	errors := make(map[string]string)
	// проверяем  user_id
	if _, err := uuid.Parse(req.UserID); err != nil {
		errors["user_id"] = "must be a valid UUID"
	}
	if _, err := time.Parse("2006-01-02", req.BirthDate); err != nil {
		errors["birth_date"] = "must be in ISO 8601 format (YYYY-MM-DD)"
	}
	return errors
}

// Обработка эндпоинта POST
func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	// Проверка метода
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Предохранитель от OOM
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json format"}`, http.StatusBadRequest)
		return
	}

	// Валидация
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest) // <-- Обязательно 400 статус для ошибки!

		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "validation_failed",
			"details": validationErrors,
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Генерация ID
	requestID := uuid.New().String()

	// Отправка задачи в NATS JetStream
	payload, err := json.Marshal(req)
	if err != nil {
		http.Error(w, `{"error": "failed to encode payload"}`, http.StatusInternalServerError)
		return
	}

	if messaging.JS != nil {
		_, err = messaging.JS.Publish(r.Context(), "astro.events.profile", payload)
		if err != nil {
			http.Error(w, `{"error": "failed to publish event to NATS"}`, http.StatusInternalServerError)
			return
		}
	}

	// Возврат успешного ответа
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // <-- Обязательно 202 статус для успеха!

	if err := json.NewEncoder(w).Encode(map[string]string{
		"request_id": requestID,
	}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
