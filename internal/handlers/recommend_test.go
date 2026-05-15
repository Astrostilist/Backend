package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	almocks "astroapi/internal/alisa/mocks"
	msgmocks "astroapi/internal/handlers/mocks"
	reqmocks "astroapi/internal/requests/mocks"
	rulemocks "astroapi/internal/ruleengine/mocks"
	usermocks "astroapi/internal/user/mocks"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestRecommend_AsyncSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pub := msgmocks.NewMockMsgPublisher(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	userRepo := usermocks.NewMockRepository(ctrl)
	rulesRepo := rulemocks.NewMockRepository(ctrl)
	ai := almocks.NewMockGenerator(ctrl)

	h := NewRecommendHandler(pub, userRepo, rulesRepo, ai, reqRepo, zap.NewNop())

	// 1. Ожидаем создание записи в БД со статусом pending
	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// 2. Ожидаем публикацию в NATS (теперь используем точку astro.events)
	pub.EXPECT().
		PublishMessage(gomock.Any(), "astro_events", "astro.events.recommend", gomock.Any()).
		Return(nil).
		Times(1)

	body, _ := json.Marshal(map[string]any{
		"user_id":  "123e4567-e89b-12d3-a456-426614174000",
		"scenario": "personal_style",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Handle(rr, req)

	// Проверяем DoD задачи #76: должен быть 202 Accepted
	require.Equal(t, http.StatusAccepted, rr.Code)
}

func TestRecommend_NatsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pub := msgmocks.NewMockMsgPublisher(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)

	h := NewRecommendHandler(pub, nil, nil, nil, reqRepo, zap.NewNop())

	reqRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
	pub.EXPECT().PublishMessage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("nats down"))

	body, _ := json.Marshal(map[string]any{
		"user_id":  "123e4567-e89b-12d3-a456-426614174000",
		"scenario": "personal_style",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Handle(rr, req)

	// Ожидаем 503 Service Unavailable при ошибке NATS
	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
}
