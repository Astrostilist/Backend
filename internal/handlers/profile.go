package handlers

import (
	"astroapi/internal/models"
	"context"
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
	if _, err := uuid.Parse(req.UserID); err != nil {
		errors["user_id"] = "must be a valid UUID"
	}
	if _, err := time.Parse("2006-01-02", req.BirthDate); err != nil {
		errors["birth_date"] = "must be in ISO 8601 format (YYYY-MM-DD)"
	}
	return errors
}

// Создаем структуру хендлера с паблишером внутри
type EventPublisher interface {
	PublishMessage(ctx context.Context, streamName, subject string, payload any) error
}

// Заменяем жесткую привязку на интерфейс
type ProfileHandler struct {
	publisher EventPublisher
}

func NewProfileHandler(p EventPublisher) *ProfileHandler {
	return &ProfileHandler{publisher: p}
}

// Метод обработки запроса
func (h *ProfileHandler) HandleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json format"}`, http.StatusBadRequest)
		return
	}

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

	requestID := uuid.New().String()

	// Отправляем сообщение
	err := h.publisher.PublishMessage(r.Context(), models.MsgStreamEvents, "astro.events.profile", req)
	if err != nil {
		http.Error(w, `{"error": "failed to publish event to NATS"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	if err := json.NewEncoder(w).Encode(map[string]string{
		"request_id": requestID,
	}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
