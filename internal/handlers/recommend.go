package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"astroapi/internal/alisa"
	"astroapi/internal/models"
	"astroapi/internal/requests"
	"astroapi/internal/resilience"
	"astroapi/internal/user"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	recommendMaxBodyBytes = 1 << 20
	recommendSyncTimeout  = 5 * time.Second
	recommendModeSync     = "sync"
	recommendModeAsync    = "async"
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
	if req.Mode == "" {
		req.Mode = recommendModeAsync
	} else if req.Mode != recommendModeSync && req.Mode != recommendModeAsync {
		errs["mode"] = "mode must be either 'sync' or 'async'"
	}
	return errs
}

// RecommendHandler обслуживает POST /api/v1/astro/recommend.
type RecommendHandler struct {
	publisher    MsgPublisher
	userRepo     user.Repository
	rulesRepo    RuleMatcher
	aiClient     alisa.Generator
	astroClient  AstroProfileGetter
	requestsRepo requests.Repository
	logger       *zap.Logger
}

func NewRecommendHandler(
	publisher MsgPublisher,
	userRepo user.Repository,
	rulesRepo RuleMatcher,
	aiClient alisa.Generator,
	astroClient AstroProfileGetter,
	requestsRepo requests.Repository,
	logger *zap.Logger,
) *RecommendHandler {
	return &RecommendHandler{
		publisher:    publisher,
		userRepo:     userRepo,
		rulesRepo:    rulesRepo,
		aiClient:     aiClient,
		astroClient:  astroClient,
		requestsRepo: requestsRepo,
		logger:       logger,
	}
}

func (h *RecommendHandler) Handle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, recommendMaxBodyBytes)

	var req RecommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "request body is required")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid json format")
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
		Scenario:  req.Scenario,
		Status:    requests.StatusPending,
	}); err != nil {
		h.logger.Error("failed to create requests_log entry", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	if req.Mode == recommendModeAsync {
		h.handleAsync(w, r, requestID, req)
		return
	}
	h.handleSync(w, r, requestID, req)
}

func (h *RecommendHandler) handleAsync(w http.ResponseWriter, r *http.Request, requestID string, req RecommendRequest) {
	payload := recommendPayload{RequestID: requestID, Recommend: req}
	if err := h.publisher.PublishMessage(r.Context(), models.MsgStreamEvents, models.MsgRecommendSubj, payload); err != nil {
		h.logger.Error("failed to publish recommend event", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to publish event")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"request_id": requestID})
}

func (h *RecommendHandler) handleSync(w http.ResponseWriter, r *http.Request, requestID string, req RecommendRequest) {
	ctx, cancel := context.WithTimeout(r.Context(), recommendSyncTimeout)
	defer cancel()

	result, err := buildRecommendation(ctx, req, h.userRepo, h.rulesRepo, h.aiClient, h.astroClient, h.logger)
	if err != nil {
		h.markFailed(r.Context(), requestID, err)
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusGatewayTimeout, "ai service timeout")
			return
		}
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user profile not found")
			return
		}
		if resilience.IsServiceUnavailable(err) {
			writeError(w, http.StatusServiceUnavailable, "service temporarily unavailable")
			return
		}
		h.logger.Error("recommend sync failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "failed to build recommendation")
		return
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		h.logger.Error("failed to marshal recommendation result", zap.Error(marshalErr))
		writeError(w, http.StatusInternalServerError, "failed to encode result")
		return
	}
	if err := h.requestsRepo.UpdateStatus(r.Context(), requestID, requests.StatusCompleted, resultJSON, ""); err != nil {
		h.logger.Error("failed to update requests_log", zap.Error(err))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"request_id": requestID,
		"result":     result.Text,
		"tags":       result.Tags,
		"status":     requests.StatusCompleted,
	})
}

func (h *RecommendHandler) markFailed(ctx context.Context, requestID string, err error) {
	if updateErr := h.requestsRepo.UpdateStatus(ctx, requestID, requests.StatusFailed, nil, err.Error()); updateErr != nil {
		h.logger.Warn("failed to update requests_log to failed", zap.Error(updateErr))
	}
}

// recommendPayload — сообщение в JetStream.
type recommendPayload struct {
	RequestID string           `json:"request_id"`
	Recommend RecommendRequest `json:"recommend"`
}
