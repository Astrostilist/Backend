package handlers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"astroapi/internal/handlers"
	msgmocks "astroapi/internal/handlers/mocks"
	"astroapi/internal/models"
	reqmocks "astroapi/internal/requests/mocks"
	"astroapi/internal/usecases"
	repomocks "astroapi/internal/usecases/repositories/mocks"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

const validProfilePayload = `{
	"user_id": "123e4567-e89b-12d3-a456-426614174000",
	"birth_date": "1990-01-01",
	"birth_place": "Moscow",
	"consent_given": true
}`

func TestProfileHandler_Success(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	publisher := msgmocks.NewMockMsgPublisher(ctrl)
	requestsRepo := reqmocks.NewMockRepository(ctrl)
	// Мокаем репозитории для usecase
	dbRepo := repomocks.NewMockPersonalDataRepository(ctrl)
	cacheRepo := repomocks.NewMockPersonalDataRepository(ctrl)
	// Создаём реальный usecase с моками
	uc := usecases.NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

	requestsRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	dbRepo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	publisher.EXPECT().
		PublishMessage(gomock.Any(), models.MsgStreamEvents, models.MsgProfileSubj, gomock.Any()).
		Return(nil).
		Times(1)

	h := handlers.NewProfileHandler(publisher, requestsRepo, uc, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/profile", bytes.NewBufferString(validProfilePayload))
	rr := httptest.NewRecorder()

	h.Handle(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotEmpty(t, resp["request_id"])
}

func TestProfileHandler_ValidationAndErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		method         string
		payload        string
		expectedStatus int
	}{
		{
			name:           "invalid birth_date",
			method:         http.MethodPost,
			payload:        `{"user_id":"123e4567-e89b-12d3-a456-426614174000","birth_date":"32-13-2024","birth_place":"Moscow","consent_given":true}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing user_id",
			method:         http.MethodPost,
			payload:        `{"birth_date":"1990-01-01","birth_place":"Moscow","consent_given":true}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "malformed json",
			method:         http.MethodPost,
			payload:        `{bad json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			payload:        "",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "body too large",
			method:         http.MethodPost,
			payload:        string(make([]byte, (1<<20)+1)),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			publisher := msgmocks.NewMockMsgPublisher(ctrl)
			requestsRepo := reqmocks.NewMockRepository(ctrl)
			// Мокаем репозитории для usecase
			dbRepo := repomocks.NewMockPersonalDataRepository(ctrl)
			cacheRepo := repomocks.NewMockPersonalDataRepository(ctrl)
			// Создаём реальный usecase с моками
			uc := usecases.NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

			h := handlers.NewProfileHandler(publisher, requestsRepo, uc, zap.NewNop())
			req := httptest.NewRequest(tc.method, "/api/v1/astro/profile", bytes.NewBufferString(tc.payload))
			rr := httptest.NewRecorder()

			h.Handle(rr, req)
			require.Equal(t, tc.expectedStatus, rr.Code)
		})
	}
}

func TestProfileHandler_PublishFailure(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	publisher := msgmocks.NewMockMsgPublisher(ctrl)
	requestsRepo := reqmocks.NewMockRepository(ctrl)
	// Мокаем репозитории для usecase
	dbRepo := repomocks.NewMockPersonalDataRepository(ctrl)
	cacheRepo := repomocks.NewMockPersonalDataRepository(ctrl)
	// Создаём реальный usecase с моками
	uc := usecases.NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

	requestsRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	dbRepo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	publisher.EXPECT().
		PublishMessage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("nats down")).
		Times(1)

	h := handlers.NewProfileHandler(publisher, requestsRepo, uc, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/profile", bytes.NewBufferString(validProfilePayload))
	rr := httptest.NewRecorder()

	h.Handle(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
}
