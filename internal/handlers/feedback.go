package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
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

// CreateFeedback эндпоинт POST /api/v1/astro/feedback
func (h *FeedbackHandler) CreateFeedback(w http.ResponseWriter, r *http.Request) {
	// 1. Декодируем входящий JSON
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "неверный формат JSON"}`, http.StatusBadRequest)
		return
	}

	// ВАЛИДАЦИЯ (согласно DoD)
	if req.RequestID == "" {
		http.Error(w, `{"error": "request_id обязателен"}`, http.StatusBadRequest)
		return
	}

	// Проверка: rating строго 1–5 -> иначе 422 Unprocessable Entity
	if req.Rating < 1 || req.Rating > 5 {
		http.Error(w, `{"error": "rating должен быть от 1 до 5"}`, http.StatusUnprocessableEntity)
		return
	}

	// Проверка длины комментария (опционально, но полезно для защиты БД)
	if len(req.Comment) > 500 {
		http.Error(w, `{"error": "comment не должен превышать 500 символов"}`, http.StatusUnprocessableEntity)
		return
	}

	// Формируем модель для БД
	feedback := &models.Feedback{
		ID:        uuid.New().String(), // Генерируем уникальный ID для самого отзыва
		RequestID: req.RequestID,
		Rating:    req.Rating,
		Comment:   req.Comment,
		CreatedAt: time.Now(),
	}

	// Пытаемся сохранить в базу через наш репозиторий
	err := h.repo.Create(feedback)
	if err != nil {
		errStr := err.Error()

		// Обрабатываем ошибку дубликата -> 409 Conflict
		if strings.Contains(errStr, "уже существует") {
			http.Error(w, `{"error": "`+errStr+`"}`, http.StatusConflict)
			return
		}

		// Обрабатываем ошибку несуществующего request_id -> 404 Not Found
		if strings.Contains(errStr, "не найден") {
			http.Error(w, `{"error": "`+errStr+`"}`, http.StatusNotFound)
			return
		}

		// Если упало что-то другое (например, отвалилась БД) -> 500
		http.Error(w, `{"error": "внутренняя ошибка сервера"}`, http.StatusInternalServerError)
		return
	}

	// Возвращаем 201 Created
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status": "success", "message": "Отзыв успешно сохранен"}`))
}
