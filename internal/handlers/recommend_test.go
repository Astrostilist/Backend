package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	almocks "astroapi/internal/alisa/mocks"
	"astroapi/internal/handlers"
	msgmocks "astroapi/internal/handlers/mocks"
	"astroapi/internal/models"
	reqmocks "astroapi/internal/requests/mocks"
	rulemocks "astroapi/internal/ruleengine/mocks"
	usermocks "astroapi/internal/user/mocks"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

const validUserID = "123e4567-e89b-12d3-a456-426614174000"

func newRecommendDeps(t *testing.T) (
	*msgmocks.MockMsgPublisher,
	*usermocks.MockRepository,
	*rulemocks.MockRepository,
	*almocks.MockGenerator,
	*reqmocks.MockRepository,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	return msgmocks.NewMockMsgPublisher(ctrl),
		usermocks.NewMockRepository(ctrl),
		rulemocks.NewMockRepository(ctrl),
		almocks.NewMockGenerator(ctrl),
		reqmocks.NewMockRepository(ctrl)
}

func TestRecommend_AsyncPublishesToNATS(t *testing.T) {
	t.Parallel()
	pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)

	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	pub.EXPECT().
		PublishMessage(gomock.Any(), models.MsgStreamEvents, models.MsgRecommendSubj, gomock.Any()).
		Return(nil).
		Times(1)

	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, nil, reqRepo, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "personal_style",
		"mode":     "async",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Handle(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotEmpty(t, resp["request_id"])
}

func TestRecommend_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{name: "bad scenario", payload: map[string]any{"user_id": validUserID, "scenario": "bad"}},
		{name: "bad user_id", payload: map[string]any{"user_id": "not-uuid", "scenario": "personal_style"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)
			_ = context.Background()
			// ни один зависимый мок не должен быть вызван
			h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, nil, reqRepo, zap.NewNop())

			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			h.Handle(rr, req)
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestRecommend_InvalidJSON(t *testing.T) {
	t.Parallel()
	pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)
	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, nil, reqRepo, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader([]byte("invalid {")))
	rr := httptest.NewRecorder()
	h.Handle(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRecommend_DBCreateError(t *testing.T) {
	t.Parallel()
	pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)

	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(errors.New("db is down")).Times(1)

	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, nil, reqRepo, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "personal_style",
		"mode":     "async",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Handle(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestRecommend_AsyncPublishError(t *testing.T) {
	t.Parallel()
	pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)

	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	pub.EXPECT().PublishMessage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("nats down")).Times(1)

	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, nil, reqRepo, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "personal_style",
		"mode":     "async",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Handle(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
}
