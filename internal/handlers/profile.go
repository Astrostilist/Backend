package handlers

import (
	astrologger "astroapi/internal/infrastructure/logger"
	"astroapi/internal/models"
	"astroapi/internal/repositories/domain"
	"astroapi/internal/requests"
	"astroapi/internal/resilience"
	"astroapi/internal/usecases"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const (
	profileMaxBodyBytes = 1 << 20
	profileScenarioName = "profile"
)

type ProfileRequest struct {
	UserID       string   `json:"user_id"`
	BirthDate    string   `json:"birth_date"`
	BirthTime    string   `json:"birth_time,omitempty"`
	BirthPlace   string   `json:"birth_place,omitempty"`
	Lat          *float64 `json:"lat"`
	Lon          *float64 `json:"lon"`
	Timezone     string   `json:"timezone,omitempty"`
	ConsentGiven bool     `json:"consent_given"`
}

// ProfileHandler обрабатывает POST /api/v1/astro/profile.
// Действия: валидация → создать запись в requests_log (status=pending) →
// Сохраняем данный либо в БД, либо в memcached →
// опубликовать событие в JetStream → вернуть 202 с request_id.
type ProfileHandler struct {
	publisher    MsgPublisher
	requestsRepo requests.Repository
	uc           *usecases.ProcessPersonalDataUseCase
	logger       *zap.Logger
}

// profilePayload — сообщение в JetStream. Выделено структурой, чтобы его типизированно читал consumer.
type profilePayload struct {
	RequestID string         `json:"request_id"`
	Profile   ProfileRequest `json:"profile"`
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
	if req.Lat == nil {
		errs["lat"] = "is required for natal chart calculation"
	}
	if req.Lon == nil {
		errs["lon"] = "is required for natal chart calculation"
	}
	return errs
}

func NewProfileHandler(
	publisher MsgPublisher,
	requestsRepo requests.Repository,
	uc *usecases.ProcessPersonalDataUseCase,
	logger *zap.Logger,
) *ProfileHandler {
	return &ProfileHandler{
		publisher:    publisher,
		requestsRepo: requestsRepo,
		uc:           uc,
		logger:       logger,
	}
}

func (h *ProfileHandler) HandleProfile(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rctx := r.Context()
	requestID := uuid.New().String()

	// Span для бизнес-логики хэндлера
	tracer := otel.Tracer("http-api")
	hctx, handlerSpan := tracer.Start(rctx, "handler.profile")
	handlerSpan.SetAttributes(attribute.String("request_id", requestID))
	defer handlerSpan.End()

	r.Body = http.MaxBytesReader(w, r.Body, profileMaxBodyBytes)

	var req ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, io.EOF) {
			writeError(w, status, "request body is required")
			handlerSpan.RecordError(err)
			return
		}
		writeError(w, status, "invalid json format")
		handlerSpan.RecordError(err)
		return
	}

	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"details": validationErrors,
		})
		handlerSpan.RecordError(errors.New("validation error"))
		return
	}

	// TODO:
	// + begin transaction
	// store user request
	if err := h.requestsRepo.Create(hctx, requests.Request{
		RequestID: requestID,
		UserID:    req.UserID,
		Scenario:  profileScenarioName,
		Status:    requests.StatusPending,
	}); err != nil {
		astrologger.Error(hctx, "failed to create requests_log entry", zap.Error(err))
		handlerSpan.RecordError(err)
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	// store user (DB or cache)
	err := h.uc.Execute(hctx, usecases.ProcessPersonalDataInput{
		PersonalData: domain.PersonalData{
			UserID:       req.UserID,
			DOB:          req.BirthDate,
			ConsentGiven: req.ConsentGiven,
		},
	})

	if err != nil {
		astrologger.Error(hctx, "failed to process personal data", zap.Error(err))
		handlerSpan.RecordError(err)
		if resilience.IsServiceUnavailable(err) {
			writeError(w, http.StatusServiceUnavailable, "service unavailable")
			return
		}

		writeError(w, http.StatusInternalServerError, "failed to process data")
		return
	}

	pubctx, publishSpan := tracer.Start(hctx, "nats.publish-profile")
	defer publishSpan.End()
	payload := profilePayload{RequestID: requestID, Profile: req}
	if err := h.publisher.PublishMessage(hctx, models.MsgStreamEvents, models.MsgProfileSubj, payload); err != nil {
		astrologger.Error(pubctx, "failed to publish profile event", zap.Error(err))
		publishSpan.RecordError(err)
		writeError(w, http.StatusInternalServerError, "failed to publish event")
		return
	}

	// TODO:
	// - end transaction

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"request_id": requestID}); err != nil {
		handlerSpan.RecordError(err)
		astrologger.Error(hctx, "failed to encode profile response", zap.Error(err))
	}
}
