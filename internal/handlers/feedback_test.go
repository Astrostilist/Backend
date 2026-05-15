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

	"github.com/stretchr/testify/assert"
)

type mockFeedbackRepo struct {
	mockCreate func(ctx context.Context, f *models.Feedback) error
}

func (m *mockFeedbackRepo) Create(ctx context.Context, f *models.Feedback) error {
	return m.mockCreate(ctx, f)
}

func TestCreateFeedback(t *testing.T) {
	validID := "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name           string
		requestBody    string
		reqMockSetup   func(ctx context.Context, id string) (requests.Request, error)
		fbMockSetup    func(ctx context.Context, f *models.Feedback) error
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:        "Успешное сохранение (200 OK)",
			requestBody: `{"request_id": "` + validID + `", "rating": 5, "comment": "Excellent"}`,
			reqMockSetup: func(ctx context.Context, id string) (requests.Request, error) {
				return requests.Request{RequestID: id, Status: requests.StatusCompleted}, nil
			},
			fbMockSetup:    func(ctx context.Context, f *models.Feedback) error { return nil },
			expectedStatus: http.StatusOK,
			expectedMsg:    "successfully",
		},
		{
			name:        "Ошибка: запрос в процессе (400 Bad Request)",
			requestBody: `{"request_id": "` + validID + `", "rating": 5}`,
			reqMockSetup: func(ctx context.Context, id string) (requests.Request, error) {
				return requests.Request{RequestID: id, Status: requests.StatusPending}, nil
			},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "wrong_status",
		},
		{
			name:        "Ошибка: запрос не найден (404 Not Found)",
			requestBody: `{"request_id": "` + validID + `", "rating": 4}`,
			reqMockSetup: func(ctx context.Context, id string) (requests.Request, error) {
				return requests.Request{}, requests.ErrNotFound
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fbRepo := &mockFeedbackRepo{mockCreate: tt.fbMockSetup}
			reqRepo := &mockRequestsRepo{mockGet: tt.reqMockSetup}

			handler := NewFeedbackHandler(fbRepo, reqRepo)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", bytes.NewBufferString(tt.requestBody))
			rr := httptest.NewRecorder()

			handler.CreateFeedback(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			var response map[string]string
			json.Unmarshal(rr.Body.Bytes(), &response)
			assert.Contains(t, response["message"], tt.expectedMsg)
		})
	}
}
