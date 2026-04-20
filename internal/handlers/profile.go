package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"astroapi/internal/models"

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

// 1. Используем готовый MsgPublisher
type ProfileHandler struct {
	publisher MsgPublisher
}

func NewProfileHandler(p MsgPublisher) *ProfileHandler {
	return &ProfileHandler{publisher: p}
}

func (h *ProfileHandler) HandleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// 3. Используем Сашину функцию writeError
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json format")
		return
	}

	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		// 3. Используем Сашину функцию writeJSON для кастомного ответа с ошибками
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"details": validationErrors,
		})
		return
	}

	requestID := uuid.New().String()

	// 2. Используем константу MsgProfileSubj
	err := h.publisher.PublishMessage(r.Context(), models.MsgStreamEvents, models.MsgProfileSubj, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish event to NATS")
		return
	}

	// 3. Используем writeJSON для успешного ответа
	writeJSON(w, http.StatusAccepted, map[string]string{
		"request_id": requestID,
	})
}
