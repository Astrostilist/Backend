package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestRecommendHandler(t *testing.T) {
	// Создаем "таблицу" тест-кейсов
	tests := []struct {
		name           string
		method         string
		body           map[string]interface{}
		expectedStatus int
	}{
		{
			// DoD: Тест: невалидный scenario → 400
			name:   "Невалидный scenario -> 400",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":  "123e4567-e89b-12d3-a456-426614174000",
				"scenario": "bad_scenario", // Специально делаем ошибку
			},
			expectedStatus: http.StatusBadRequest, // 400
		},
		{
			// DoD: Тест: корректный запрос async → 202 + request_id
			name:   "Корректный запрос async -> 202",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":  "123e4567-e89b-12d3-a456-426614174000",
				"scenario": "personal_style",
				"mode":     "async",
			},
			expectedStatus: http.StatusAccepted, // 202
		},
		{
			// DoD: Таймаут sync-режима не зависает дольше 5 секунд
			name:   "Sync-режим отрабатывает (имитация 2 сек)",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":  "123e4567-e89b-12d3-a456-426614174000",
				"scenario": "perfect_gift",
				"mode":     "sync",
			},
			expectedStatus: http.StatusOK, // У нас стоит заглушка на 200 OK через 2 секунды
		},
		{
			// DoD: Тест: невалидный mode -> 400
			name:   "Невалидный режим работы (mode) -> 400",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":  "123e4567-e89b-12d3-a456-426614174000",
				"scenario": "personal_style",
				"mode":     "wrong_mode", // Специально передаем мусор
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Упаковываем тестовые данные в JSON
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(tt.method, "/api/v1/astro/recommend", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			// Создаем "шпиона" (Recorder), который запишет ответ сервера
			rr := httptest.NewRecorder()

			// Засекаем время (важно для проверки таймаута!)
			start := time.Now()

			// Заворачиваем функцию в роутер и вызываем
			r := chi.NewRouter()
			r.Post("/api/v1/astro/recommend", RecommendHandler)
			r.ServeHTTP(rr, req)

			// Считаем, сколько времени занял запрос
			duration := time.Since(start)

			// Проверяем статус-код
			if rr.Code != tt.expectedStatus {
				t.Errorf("Ожидался статус %d, получен %d. Тело: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			// Специфичные проверки
			if tt.name == "Корректный запрос async -> 202" {
				var response map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				if err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if response["request_id"] == "" {
					t.Errorf("Ожидался request_id в ответе, но он пустой")
				}
			}

			// Проверка для DoD: не дольше 5 секунд
			if duration > 5*time.Second {
				t.Errorf("Запрос завис дольше 5 секунд! Заняло: %v", duration)
			}
		})
	}
}
