package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockPublisher — это наша заглушка для тестов
type mockPublisher struct {
	err error // Ошибка, которую мы хотим сымитировать
}

// Реализуем интерфейс EventPublisher
func (m *mockPublisher) PublishMessage(ctx context.Context, streamName, subject string, payload any) error {
	return m.err
}

func TestProfileHandler_HandleProfile(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           map[string]interface{}
		mockPubErr     error
		expectedStatus int
	}{
		{
			name:           "Method Not Allowed (GET)",
			method:         http.MethodGet,
			body:           nil,
			mockPubErr:     nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid JSON (Body is wrong)",
			method:         http.MethodPost,
			body:           map[string]interface{}{"user_id": make(chan int)}, // Сломает JSON кодер
			mockPubErr:     nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Validation Failed (Bad UUID)",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":       "not-a-uuid",
				"birth_date":    "1990-01-01",
				"birth_place":   "Moscow",
				"consent_given": true,
			},
			mockPubErr:     nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "NATS Publish Error",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":       "123e4567-e89b-12d3-a456-426614174000",
				"birth_date":    "1990-01-01",
				"birth_place":   "Moscow",
				"consent_given": true,
			},
			mockPubErr:     errors.New("nats connection lost"), // Имитируем сбой NATS
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:   "Success",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":       "123e4567-e89b-12d3-a456-426614174000",
				"birth_date":    "1990-01-01",
				"birth_place":   "Moscow",
				"consent_given": true,
			},
			mockPubErr:     nil, // Всё хорошо
			expectedStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Настраиваем фейковый паблишер
			mockPub := &mockPublisher{err: tt.mockPubErr}
			handler := NewProfileHandler(mockPub)

			// 2. Готовим тело запроса
			var reqBody []byte
			if tt.body != nil {
				// Если мы специально подсунули "неконвертируемый" тип, игнорируем ошибку для теста на кривой JSON
				reqBody, _ = json.Marshal(tt.body)
				if tt.name == "Invalid JSON (Body is wrong)" {
					reqBody = []byte(`{"user_id": "123", missing quotes}`)
				}
			}

			// 3. Создаем фейковый HTTP запрос и ResponseRecorder
			req := httptest.NewRequest(tt.method, "/api/v1/astro/profile", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// 4. Вызываем хендлер
			handler.HandleProfile(rr, req)

			// 5. Проверяем результат
			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}
		})
	}
}
