package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"astroapi/internal/models"
)

type mockFeedbackRepo struct {
	mockCreate func(f *models.Feedback) error
}

func (m *mockFeedbackRepo) Create(f *models.Feedback) error {
	if m.mockCreate != nil {
		return m.mockCreate(f)
	}
	return nil
}

func TestCreateFeedback(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		mockSetup      func(f *models.Feedback) error
		expectedStatus int
	}{
		{
			name:           "Успешное сохранение (201 Created)",
			requestBody:    `{"request_id": "550e8400-e29b-41d4-a716-446655440000", "rating": 5, "comment": "Отлично!"}`,
			mockSetup:      func(f *models.Feedback) error { return nil },
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Ошибка валидации: rating=6 (422 Unprocessable Entity)", // DoD: Тест: rating=6 -> 422
			requestBody:    `{"request_id": "550e8400-e29b-41d4-a716-446655440000", "rating": 6}`,
			mockSetup:      func(f *models.Feedback) error { return nil },
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:        "Ошибка: несуществующий request_id (404 Not Found)", // DoD: Тест: несуществующий request_id -> 404
			requestBody: `{"request_id": "fake-uuid", "rating": 4}`,
			mockSetup: func(f *models.Feedback) error {
				return errors.New("указанный request_id не найден")
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Ошибка: дублирование отзыва (409 Conflict)", // DoD: Дублирование feedback -> 409 Conflict
			requestBody: `{"request_id": "550e8400-e29b-41d4-a716-446655440000", "rating": 4}`,
			mockSetup: func(f *models.Feedback) error {
				return errors.New("отзыв для этого request_id уже существует")
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Подготавливаем заглушку репозитория и сам хендлер
			repo := &mockFeedbackRepo{mockCreate: tt.mockSetup}
			handler := NewFeedbackHandler(repo)

			// Создаем HTTP-запрос
			req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/feedback", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")

			// Создаем объект для записи ответа
			rr := httptest.NewRecorder()

			// Вызываем хендлер напрямую
			handler.CreateFeedback(rr, req)

			// Проверяем, совпал ли полученный статус с ожидаемым по ТЗ
			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("хендлер вернул неверный статус: получили %v, ожидали %v", status, tt.expectedStatus)
			}
		})
	}
}
