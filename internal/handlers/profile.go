package handlers

import (
	"astroapi/internal/models"
	"encoding/json"
	"net/http"

	"astroapi/internal/requests"
	"astroapi/internal/usecases"
	"astroapi/internal/usecases/repositories/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const profileScenarioName = "profile"

type ProfileRequest struct {
	UserID       string `json:"user_id"`
	BirthDate    string `json:"birth_date"`
	BirthTime    string `json:"birth_time,omitempty"`
	BirthPlace   string `json:"birth_place"`
	ConsentGiven bool   `json:"consent_given"`
}

type profilePayload struct {
	RequestID string         `json:"request_id"`
	Profile   ProfileRequest `json:"profile"`
}

type ProfileHandler struct {
	publisher    MsgPublisher
	requestsRepo requests.Repository
	uc           *usecases.ProcessPersonalDataUseCase
	logger       *zap.Logger
}

func NewProfileHandler(pub MsgPublisher, repo requests.Repository, uc *usecases.ProcessPersonalDataUseCase, logger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{pub, repo, uc, logger}
}

func (h *ProfileHandler) HandleProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendResponse(w, http.StatusBadRequest, "invalid_json", "неверный формат JSON")
		return
	}

	requestID := uuid.New().String()
	if err := h.requestsRepo.Create(r.Context(), requests.Request{
		RequestID: requestID,
		UserID:    req.UserID,
		Scenario:  profileScenarioName,
		Status:    requests.StatusPending,
	}); err != nil {
		h.logger.Error("failed to create request", zap.Error(err))
		h.sendResponse(w, http.StatusInternalServerError, "db_error", "ошибка базы данных")
		return
	}

	if err := h.uc.Execute(r.Context(), usecases.ProcessPersonalDataInput{
		PersonalData: domain.PersonalData{UserID: req.UserID, DOB: req.BirthDate, ConsentGiven: req.ConsentGiven},
	}); err != nil {
		h.sendResponse(w, http.StatusInternalServerError, "process_error", "ошибка обработки данных")
		return
	}

	payload := profilePayload{RequestID: requestID, Profile: req}
	if err := h.publisher.PublishMessage(r.Context(), models.MsgStreamEvents, models.MsgProfileSubj, payload); err != nil {
		h.logger.Error("failed to publish profile", zap.Error(err))
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"request_id": requestID,
		"status":     "profile_created",
	}); err != nil {
		h.logger.Error("failed to encode response", zap.Error(err))
	}
}

func (h *ProfileHandler) sendResponse(w http.ResponseWriter, code int, msg, errStr string) {
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": msg, "error": errStr}); err != nil {
		h.logger.Error("failed to encode error response", zap.Error(err))
	}
}

func (req *ProfileRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if _, err := uuid.Parse(req.UserID); err != nil {
		errs["user_id"] = "некорректный формат UUID"
	}
	if req.BirthDate == "" {
		errs["birth_date"] = "дата рождения обязательна"
	}
	return errs
}
