package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestProfileHandler(t *testing.T) {
)

func TestProfileHandler(t *testing.T) {
	// Создаем "таблицу" тест-кейсов
	tests := []struct {
		name           string
		method         string
		payload        string
		expectedStatus int
	}{
		{
			name:           "1. Успешный запрос (DoD: корректный запрос → 202)",
			method:         http.MethodPost,
			payload:        `{"user_id": "123e4567-e89b-12d3-a456-426614174000", "birth_date": "1990-01-01", "birth_place": "Moscow", "consent_given": true}`,
			expectedStatus: http.StatusAccepted, // 202
		},
		{
			name:           "2. Ошибка валидации даты (DoD: birth_date = '32-13-2024' → 400)",
			method:         http.MethodPost,
			payload:        `{"user_id": "123e4567-e89b-12d3-a456-426614174000", "birth_date": "32-13-2024", "birth_place": "Moscow", "consent_given": true}`,
			expectedStatus: http.StatusBadRequest, // 400
		},
		{
			name:           "3. Отсутствует user_id (DoD: отсутствует user_id → 400)",
			method:         http.MethodPost,
			payload:        `{"birth_date": "1990-01-01", "birth_place": "Moscow", "consent_given": true}`,
			expectedStatus: http.StatusBadRequest, // 400
		},
		{
			name:           "4. Некорректный JSON",
			method:         http.MethodPost,
			payload:        `{bad json`,
			expectedStatus: http.StatusBadRequest, // 400
		},
		{
			name:           "5. Неверный HTTP метод",
			method:         http.MethodGet,
			payload:        ``,
			expectedStatus: http.StatusMethodNotAllowed, // 405
		},
		{
			name:           "6. Превышен лимит размера тела запроса",
			method:         http.MethodPost,
			payload:        string(make([]byte, 1048577)), // Запрос больше 1 МБ
			expectedStatus: http.StatusBadRequest,
		},
	}

	// Запускаем цикл
	// Запускаем цикл по нашей таблице
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Формируем запрос
			req, err := http.NewRequest(tt.method, "/api/v1/astro/profile", bytes.NewBufferString(tt.payload))
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			// Создаем "диктофон"
			r := chi.NewRouter()
			r.Post("/api/v1/astro/profile", ProfileHandler)

			// Выполняем запрос
			r.ServeHTTP(rr, req)

			// Создаем "диктофон" для записи ответа
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(ProfileHandler)

			// Выполняем запрос
			handler.ServeHTTP(rr, req)

			// Проверяем статус-код
			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Сценарий '%s': получили статус %v, а ожидали %v", tt.name, status, tt.expectedStatus)
			}
		})
	}
}
