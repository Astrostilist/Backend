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
	"astroapi/internal/user"
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

	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, reqRepo, zap.NewNop())

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

func TestRecommend_SyncCallsAIAndReturnsResult(t *testing.T) {
	t.Parallel()
	pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)

	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	userRepo.EXPECT().Get(gomock.Any(), validUserID).Return(user.User{
		UserID:    validUserID,
		BirthDate: "1990-01-01",
	}, nil).Times(1)
	rulesRepo.EXPECT().Match(gomock.Any(), gomock.Any()).Return([]string{"luxury"}, nil).Times(1)
	ai.EXPECT().Generate(gomock.Any(), gomock.Any()).Return("sample recommendation", nil).Times(1)
	reqRepo.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), "completed", gomock.Any(), "").Return(nil).Times(1)

	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, reqRepo, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "perfect_gift",
		"mode":     "sync",
		"context":  map[string]any{"triggers": []string{"Полнолуние"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Handle(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, "sample recommendation", resp["result"])
	require.NotEmpty(t, resp["request_id"])
}

func TestRecommend_SyncUserNotFound(t *testing.T) {
	t.Parallel()
	pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)

	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	userRepo.EXPECT().Get(gomock.Any(), validUserID).Return(user.User{}, user.ErrNotFound).Times(1)
	reqRepo.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), "failed", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, reqRepo, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "personal_style",
		"mode":     "sync",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Handle(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestRecommend_SyncAIError(t *testing.T) {
	t.Parallel()
	pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)

	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	userRepo.EXPECT().Get(gomock.Any(), validUserID).Return(user.User{UserID: validUserID, BirthDate: "1990-01-01"}, nil).Times(1)
	rulesRepo.EXPECT().Match(gomock.Any(), gomock.Any()).Return([]string{}, nil).Times(1)
	ai.EXPECT().Generate(gomock.Any(), gomock.Any()).Return("", errors.New("boom")).Times(1)
	reqRepo.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), "failed", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, reqRepo, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "personal_style",
		"mode":     "sync",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Handle(rr, req)
	require.Equal(t, http.StatusBadGateway, rr.Code)
}

func TestRecommend_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{name: "bad scenario", payload: map[string]any{"user_id": validUserID, "scenario": "bad"}},
		{name: "bad mode", payload: map[string]any{"user_id": validUserID, "scenario": "personal_style", "mode": "wrong"}},
		{name: "bad user_id", payload: map[string]any{"user_id": "not-uuid", "scenario": "personal_style"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub, userRepo, rulesRepo, ai, reqRepo := newRecommendDeps(t)
			_ = context.Background()
			// ни один зависимый мок не должен быть вызван
			h := handlers.NewRecommendHandler(pub, userRepo, rulesRepo, ai, reqRepo, zap.NewNop())

			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			h.Handle(rr, req)
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}
