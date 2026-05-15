package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"astroapi/internal/models"
	"astroapi/internal/requests"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockFeedbackRepo — простая заглушка для репозитория фидбеков
type mockFeedbackRepo struct {
	mockCreate func(ctx context.Context, fb *models.Feedback) error
}

func (m *mockFeedbackRepo) Create(ctx context.Context, fb *models.Feedback) error {
	if m.mockCreate != nil {
		return m.mockCreate(ctx, fb)
	}
	return nil
}

func TestFeedbackHandler_CreateFeedback(t *testing.T) {
	logger := zap.NewNop() // «Тихий» логгер для тестов

	t.Run("Успешное сохранение (200 OK)", func(t *testing.T) {
		fbRepo := &mockFeedbackRepo{}
		reqRepo := &mockRequestsRepo{
			mockGet: func(ctx context.Context, id string) (requests.Request, error) {
				return requests.Request{
					RequestID: id,
					Status:    requests.StatusCompleted, // Статус должен быть Completed
				}, nil
			},
		}

		// Создаем хендлер, передавая правильные переменные и логгер
		h := NewFeedbackHandler(fbRepo, reqRepo, logger)

		body := map[string]interface{}{
			"request_id": "test-req-123",
			"rating":     5,
			"comment":    "Отлично!",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", bytes.NewReader(jsonBody))
		rr := httptest.NewRecorder()

		// Вызываем метод CreateFeedback (убедись, что в feedback.go он называется так)
		h.CreateFeedback(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Ошибка: запрос не найден (404 Not Found)", func(t *testing.T) {
		fbRepo := &mockFeedbackRepo{}
		reqRepo := &mockRequestsRepo{
			mockGet: func(ctx context.Context, id string) (requests.Request, error) {
				return requests.Request{}, requests.ErrNotFound
			},
		}

		h := NewFeedbackHandler(fbRepo, reqRepo, logger)

		body := map[string]interface{}{
			"request_id": "unknown-id",
			"rating":     3,
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", bytes.NewReader(jsonBody))
		rr := httptest.NewRecorder()

		h.CreateFeedback(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
	})
}
