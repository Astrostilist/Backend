package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"astroapi/internal/models"
	"astroapi/internal/repositories"

	"github.com/google/uuid"
)

type FeedbackHandler struct {
	repo repositories.FeedbackRepository
}

func NewFeedbackHandler(repo repositories.FeedbackRepository) *FeedbackHandler {
	return &FeedbackHandler{repo: repo}
}

type feedbackRequest struct {
	RequestID string `json:"request_id"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
}

func (h *FeedbackHandler) CreateFeedback(w http.ResponseWriter, r *http.Request) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "неверный формат JSON"}`, http.StatusBadRequest)
		return
	}

	if req.RequestID == "" {
		http.Error(w, `{"error": "request_id обязателен"}`, http.StatusBadRequest)
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		http.Error(w, `{"error": "rating должен быть от 1 до 5"}`, http.StatusUnprocessableEntity)
		return
	}

	if len(req.Comment) > 500 {
		http.Error(w, `{"error": "comment не должен превышать 500 символов"}`, http.StatusUnprocessableEntity)
		return
	}

	feedback := &models.Feedback{
		ID:        uuid.New().String(),
		RequestID: req.RequestID,
		Rating:    req.Rating,
		Comment:   req.Comment,
		CreatedAt: time.Now(),
	}

	err := h.repo.Create(r.Context(), feedback)
	if err != nil {
		// Эталонная обработка ошибок (без парсинга строк)
		if errors.Is(err, models.ErrFeedbackExists) {
			http.Error(w, `{"error": "отзыв для этого request_id уже существует"}`, http.StatusConflict)
			return
		}

		if errors.Is(err, models.ErrRequestNotFound) {
			http.Error(w, `{"error": "указанный request_id не найден"}`, http.StatusNotFound)
			return
		}

		http.Error(w, `{"error": "внутренняя ошибка сервера"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status": "success", "message": "Отзыв успешно сохранен"}`))
}
