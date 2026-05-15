package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"astroapi/internal/models"
	"astroapi/internal/repositories"
	"astroapi/internal/requests"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type FeedbackHandler struct {
	repo         repositories.FeedbackRepository
	requestsRepo requests.Repository
	logger       *zap.Logger // ИСПРАВЛЕНО: добавлен логгер в структуру
}

func NewFeedbackHandler(repo repositories.FeedbackRepository, requestsRepo requests.Repository, logger *zap.Logger) *FeedbackHandler {
	return &FeedbackHandler{
		repo:         repo,
		requestsRepo: requestsRepo,
		logger:       logger,
	}
}

type feedbackRequest struct {
	RequestID string `json:"request_id"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
}

func (h *FeedbackHandler) CreateFeedback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendResponse(w, http.StatusBadRequest, "invalid_json", "неверный формат JSON")
		return
	}

	requestLog, err := h.requestsRepo.Get(r.Context(), req.RequestID)
	if err != nil {
		if errors.Is(err, requests.ErrNotFound) {
			h.sendResponse(w, http.StatusNotFound, "not_found", "запрос не найден")
			return
		}
		h.sendResponse(w, http.StatusInternalServerError, "db_error", "ошибка базы данных")
		return
	}

	if requestLog.Status != requests.StatusCompleted {
		h.sendResponse(w, http.StatusBadRequest, "wrong_status", "оценка возможна только для завершенных запросов")
		return
	}

	feedback := &models.Feedback{
		ID:        uuid.New().String(),
		RequestID: req.RequestID,
		Rating:    req.Rating,
		Comment:   req.Comment,
		CreatedAt: time.Now(),
	}

	if err := h.repo.Create(r.Context(), feedback); err != nil {
		if errors.Is(err, models.ErrFeedbackExists) {
			h.sendResponse(w, http.StatusConflict, "already_exists", "отзыв уже оставлен")
			return
		}
		h.sendResponse(w, http.StatusInternalServerError, "save_failed", "ошибка сохранения отзыва")
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"message": "feedback saved",
	}); err != nil {
		h.logger.Error("failed to encode response", zap.Error(err))
	}
}

func (h *FeedbackHandler) sendResponse(w http.ResponseWriter, code int, msg, errStr string) {
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"message": msg,
		"error":   errStr,
	}); err != nil {
		h.logger.Error("failed to encode error response", zap.Error(err))
	}
}
