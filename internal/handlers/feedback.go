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
)

type FeedbackHandler struct {
	repo         repositories.FeedbackRepository
	requestsRepo requests.Repository
}

func NewFeedbackHandler(repo repositories.FeedbackRepository, requestsRepo requests.Repository) *FeedbackHandler {
	return &FeedbackHandler{
		repo:         repo,
		requestsRepo: requestsRepo,
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

	// 1. Проверка существования и статуса запроса
	requestLog, err := h.requestsRepo.Get(r.Context(), req.RequestID)
	if err != nil {
		if errors.Is(err, requests.ErrNotFound) {
			h.sendResponse(w, http.StatusNotFound, "not_found", "запрос не найден")
			return
		}
		h.sendResponse(w, http.StatusInternalServerError, "db_error", "ошибка базы данных")
		return
	}

	// Фидбек принимаем только для успешно завершенных генераций
	if requestLog.Status != requests.StatusCompleted {
		h.sendResponse(w, http.StatusBadRequest, "wrong_status", "оценка возможна только для завершенных запросов")
		return
	}

	// 2. Сохранение фидбека
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

	// 3. Успешный ответ (200 OK)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Feedback saved successfully",
	})
}

func (h *FeedbackHandler) sendResponse(w http.ResponseWriter, code int, msg, errStr string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"message": msg,
		"error":   errStr,
	})
}
