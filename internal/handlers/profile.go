package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"astroapi/internal/models"
	"astroapi/internal/requests"
	"astroapi/internal/usecases"
	"astroapi/internal/usecases/repositories/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	profileMaxBodyBytes = 1 << 20 // 1MB
	profileScenarioName = "profile"
)

// ProfileRequest — payload POST /api/v1/astro/profile.
type ProfileRequest struct {
	UserID       string `json:"user_id"`
	BirthDate    string `json:"birth_date"`
	BirthTime    string `json:"birth_time,omitempty"`
	BirthPlace   string `json:"birth_place"`
	ConsentGiven bool   `json:"consent_given"`
}

// Validate возвращает map ошибок валидации (field -> message).
func (req *ProfileRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if _, err := uuid.Parse(req.UserID); err != nil {
		errs["user_id"] = "must be a valid UUID"
	}
	if _, err := time.Parse("2006-01-02", req.BirthDate); err != nil {
		errs["birth_date"] = "must be in ISO 8601 format (YYYY-MM-DD)"
	}
	return errs
}

// ProfileHandler обрабатывает POST /api/v1/astro/profile.
// Действия: валидация → создать запись в requests_log (status=accepted) →
// Сохраняем данный либо в БД, либо в memcached →
// опубликовать событие в JetStream → вернуть 202 с request_id.
type ProfileHandler struct {
	publisher    MsgPublisher
	requestsRepo requests.Repository
	uc           *usecases.ProcessPersonalDataUseCase
	logger       *zap.Logger
}

func NewProfileHandler(publisher MsgPublisher, requestsRepo requests.Repository, uc *usecases.ProcessPersonalDataUseCase, logger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{publisher: publisher, requestsRepo: requestsRepo, uc: uc, logger: logger}
}

func (h *ProfileHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, profileMaxBodyBytes)

	var req ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, io.EOF) {
			writeError(w, status, "request body is required")
			return
		}
		writeError(w, status, "invalid json format")
		return
	}

	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"details": validationErrors,
		})
		return
	}

	requestID := uuid.New().String()

	if err := h.requestsRepo.Create(r.Context(), requests.Request{
		RequestID: requestID,
		UserID:    req.UserID,
		Scenario:  profileScenarioName,
		Status:    requests.StatusAccepted,
	}); err != nil {
		h.logger.Error("failed to create requests_log entry", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	err := h.uc.Execute(r.Context(), usecases.ProcessPersonalDataInput{
		PersonalData: domain.PersonalData{
			UserID:       req.UserID,
			DOB:          req.BirthDate,
			ConsentGiven: req.ConsentGiven,
		},
	})

	if err != nil {
		h.logger.Error("failed to process personal data", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to process data")
		return
	}

	payload := profilePayload{RequestID: requestID, Profile: req}
	if err := h.publisher.PublishMessage(r.Context(), models.MsgStreamEvents, models.MsgProfileSubj, payload); err != nil {
		h.logger.Error("failed to publish profile event", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to publish event")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"request_id": requestID})
}

// profilePayload — сообщение в JetStream. Выделено структурой, чтобы его типизированно читал consumer.
type profilePayload struct {
	RequestID string         `json:"request_id"`
	Profile   ProfileRequest `json:"profile"`
}
