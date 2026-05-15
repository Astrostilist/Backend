package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"astroapi/internal/requests"
	"astroapi/internal/usecases"
	"astroapi/internal/usecases/repositories/domain"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type mockPersonalDataRepo struct {
	mockSave func(ctx context.Context, data domain.PersonalData) error
}

func (m *mockPersonalDataRepo) Save(ctx context.Context, d domain.PersonalData) error {
	return m.mockSave(ctx, d)
}

func TestProfileHandler_HandleProfile(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		mockRepoErr    error
		expectedStatus int
	}{
		{
			name: "Success Profile (200 OK)",
			body: map[string]interface{}{
				"user_id":       "123e4567-e89b-12d3-a456-426614174000",
				"birth_date":    "1990-01-01",
				"consent_given": true,
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := &mockMsgPublisher{}
			mockRepo := &mockRequestsRepo{
				mockCreate: func(ctx context.Context, req requests.Request) error { return tt.mockRepoErr },
			}
			mockDataRepo := &mockPersonalDataRepo{mockSave: func(ctx context.Context, d domain.PersonalData) error { return nil }}

			uc := usecases.NewProcessPersonalDataUseCase(mockDataRepo, nil)
			handler := NewProfileHandler(mockPub, mockRepo, uc, zap.NewNop())

			reqBody, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/profile", bytes.NewReader(reqBody))
			rr := httptest.NewRecorder()

			handler.HandleProfile(rr, req)
			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
