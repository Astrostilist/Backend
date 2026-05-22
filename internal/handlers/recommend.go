package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"astroapi/internal/alisa"
	astrologger "astroapi/internal/infrastructure/logger"
	"astroapi/internal/models"
	"astroapi/internal/requests"
	"astroapi/internal/user"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const (
	recommendMaxBodyBytes = 1 << 20
	recommendSyncTimeout  = 5 * time.Second
)

// validScenarios — допустимые значения scenario.
var validScenarios = map[string]bool{
	"personal_style": true,
	"perfect_gift":   true,
}

// RecommendRequest — payload POST /api/v1/astro/recommend.
type RecommendRequest struct {
	UserID   string         `json:"user_id"`
	Scenario string         `json:"scenario"`
	Context  map[string]any `json:"context,omitempty"`
	Mode     string         `json:"mode,omitempty"`
}

// Validate проверяет запрос и проставляет Mode=async по умолчанию.
func (req *RecommendRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if _, err := uuid.Parse(req.UserID); err != nil {
		errs["user_id"] = "must be a valid UUID format"
	}
	if !validScenarios[req.Scenario] {
		errs["scenario"] = "scenario must be either 'personal_style' or 'perfect_gift'"
	}
	return errs
}

// RecommendHandler обслуживает POST /api/v1/astro/recommend.
type RecommendHandler struct {
	publisher     MsgPublisher
	userRepo      user.Repository
	rulesRepo     RuleMatcher
	aiClient      alisa.Generator
	astroProvider AstroProvider
	requestsRepo  requests.Repository
	logger        *zap.Logger
}

func NewRecommendHandler(
	publisher MsgPublisher,
	userRepo user.Repository,
	rulesRepo RuleMatcher,
	aiClient alisa.Generator,
	astroProvider AstroProvider,
	requestsRepo requests.Repository,
	logger *zap.Logger,
) *RecommendHandler {
	return &RecommendHandler{
		publisher:     publisher,
		userRepo:      userRepo,
		rulesRepo:     rulesRepo,
		aiClient:      aiClient,
		astroProvider: astroProvider,
		requestsRepo:  requestsRepo,
		logger:        logger,
	}
}

func (h *RecommendHandler) Handle(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rctx := r.Context()
	requestID := uuid.New().String()

	// Span для бизнес-логики хэндлера
	tracer := otel.Tracer("http-api")
	hctx, handlerSpan := tracer.Start(rctx, "handler.recommend")
	handlerSpan.SetAttributes(attribute.String("request_id", requestID))
	defer handlerSpan.End()

	r.Body = http.MaxBytesReader(w, r.Body, recommendMaxBodyBytes)

	var req RecommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "request body is required")
			handlerSpan.RecordError(err)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid json format")
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

	if err := h.requestsRepo.Create(hctx, requests.Request{
		RequestID: requestID,
		UserID:    req.UserID,
		Scenario:  req.Scenario,
		Status:    requests.StatusPending,
	}); err != nil {
		astrologger.Error(hctx, "failed to create requests_log entry", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	pubctx, publishSpan := tracer.Start(hctx, "nats.publish-recommend")
	defer publishSpan.End()

	payload := recommendPayload{RequestID: requestID, Recommend: req}
	if err := h.publisher.PublishMessage(pubctx, models.MsgStreamEvents, models.MsgRecommendSubj, payload); err != nil {
		astrologger.Error(pubctx, "failed to publish recommend event", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to publish event")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"request_id": requestID})

}

// recommendPayload — сообщение в JetStream.
type recommendPayload struct {
	RequestID string           `json:"request_id"`
	Recommend RecommendRequest `json:"recommend"`
}
