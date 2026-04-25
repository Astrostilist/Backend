package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"astroapi/internal/requests"

	"go.uber.org/zap"
)

type mockMsgPublisher struct {
	err error
}

func (m *mockMsgPublisher) PublishMessage(ctx context.Context, streamName, subject string, payload any) error {
	return m.err
}

type mockRequestsRepo struct {
	requests.Repository
	err error
}

func (m *mockRequestsRepo) Create(ctx context.Context, req requests.Request) error {
	return m.err
}

func TestProfileHandler_HandleProfile(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           interface{}
		mockPubErr     error
		mockRepoErr    error
		expectedStatus int
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Empty Body",
			method:         http.MethodPost,
			body:           "", // Пустое тело
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			body:           "invalid { json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Validation Failed (Bad UUID)",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":       "not-a-uuid", // Ошибка валидации
				"birth_date":    "1990-01-01",
				"birth_place":   "Moscow",
				"consent_given": true,
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Database Error",
			method: http.MethodPost,
			body: map[string]interface{}{
				"user_id":       "123e4567-e89b-12d3-a456-426614174000",
				"birth_date":    "1990-01-01",
				"birth_place":   "Moscow",
				"consent_given": true,
			},
			mockRepoErr:    errors.New("db timeout"), // Имитируем падение БД
			expectedStatus: http.StatusInternalServerError,
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
			mockPubErr:     errors.New("nats connection lost"), // Имитируем падение NATS
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
			expectedStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := &mockMsgPublisher{err: tt.mockPubErr}
			mockRepo := &mockRequestsRepo{err: tt.mockRepoErr}

			handler := NewProfileHandler(mockPub, mockRepo, zap.NewNop())

			var reqBody []byte
			if strBody, ok := tt.body.(string); ok {
				reqBody = []byte(strBody)
			} else if tt.body != nil {
				reqBody, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(tt.method, "/api/v1/astro/profile", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.HandleProfile(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}
		})
	}
}
