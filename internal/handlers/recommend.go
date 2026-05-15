package handlers

import (
	"astroapi/internal/models"
	"context"
	"encoding/json"
	"net/http"

	"astroapi/internal/requests"
	"astroapi/internal/user" // Добавляем импорт пакета user

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UserRepo должен точно соответствовать реализации в пакете user
type UserRepo interface {
	Get(ctx context.Context, userID string) (user.User, error)
}

type AIGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type RecommendRequest struct {
	UserID     string         `json:"user_id"`
	Scenario   string         `json:"scenario"`
	Context    map[string]any `json:"context,omitempty"`
	WebhookURL string         `json:"webhook_url,omitempty"`
}

type recommendPayload struct {
	RequestID string           `json:"request_id"`
	Recommend RecommendRequest `json:"recommend"`
}

type RecommendHandler struct {
	publisher    MsgPublisher
	userRepo     UserRepo
	rulesRepo    RuleMatcher
	aiClient     AIGenerator
	requestsRepo requests.Repository
	logger       *zap.Logger
}

func NewRecommendHandler(
	pub MsgPublisher,
	userRepo UserRepo,
	rulesRepo RuleMatcher,
	ai AIGenerator,
	reqRepo requests.Repository,
	logger *zap.Logger,
) *RecommendHandler {
	return &RecommendHandler{
		publisher:    pub,
		userRepo:     userRepo,
		rulesRepo:    rulesRepo,
		aiClient:     ai,
		requestsRepo: reqRepo,
		logger:       logger,
	}
}

// Переименовываем HandleRecommend в Handle, так как тесты вызывают именно h.Handle
func (h *RecommendHandler) Handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req RecommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendResponse(w, http.StatusBadRequest, "invalid_json", "неверный формат JSON")
		return
	}

	if _, err := uuid.Parse(req.UserID); err != nil {
		h.sendResponse(w, http.StatusBadRequest, "validation_failed", "некорректный user_id")
		return
	}

	requestID := uuid.New().String()
	_ = h.requestsRepo.Create(r.Context(), requests.Request{
		RequestID: requestID,
		UserID:    req.UserID,
		Scenario:  req.Scenario,
		Status:    requests.StatusPending,
	})

	payload := recommendPayload{RequestID: requestID, Recommend: req}
	if err := h.publisher.PublishMessage(r.Context(), models.MsgStreamEvents, models.MsgRecommendSubj, payload); err != nil {
		h.logger.Error("nats publish failed", zap.Error(err))
		h.sendResponse(w, http.StatusServiceUnavailable, "nats_error", "ошибка очереди событий")
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"request_id":             requestID,
		"status":                 "pending",
		"estimated_time_seconds": 30,
	})
}

func (h *RecommendHandler) sendResponse(w http.ResponseWriter, code int, msg, errStr string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": msg, "error": errStr})
}
