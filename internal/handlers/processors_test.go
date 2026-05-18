package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"astroapi/internal/alisa"
	almocks "astroapi/internal/alisa/mocks"
	"astroapi/internal/handlers"
	handlermocks "astroapi/internal/handlers/mocks"
	"astroapi/internal/requests"
	reqmocks "astroapi/internal/requests/mocks"
	rulemocks "astroapi/internal/ruleengine/mocks"
	"astroapi/internal/user"
	usermocks "astroapi/internal/user/mocks"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestProfileProcessor_HappyPath(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	astroClient := handlermocks.NewMockAstroProfileGetter(ctrl)

	reqRepo.EXPECT().
		Get(gomock.Any(), "req-1").
		Return(requests.Request{RequestID: "req-1", Status: requests.StatusPending}, nil).Times(1)
	reqRepo.EXPECT().
		StartProcessing(gomock.Any(), "req-1").
		Return(true, nil).Times(1)
	astroClient.EXPECT().
		GetAstroProfileContext(gomock.Any(), "1990-01-01", "Moscow").
		Return(alisa.AstroProfile{BirthDate: "1990-01-01", BirthPlace: "Moscow"}, nil).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-1", requests.StatusCompleted, gomock.AssignableToTypeOf([]byte{}), "").
		Return(nil).Times(1)

	p := handlers.NewProfileProcessor(userRepo, reqRepo, astroClient, zap.NewNop())

	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-1",
		"profile": map[string]any{
			"user_id":       validUserID,
			"birth_date":    "1990-01-01",
			"birth_place":   "Moscow",
			"consent_given": true,
		},
	})
	require.NoError(t, p.Handle(context.Background(), payload))
}

func TestProfileProcessor_ValidationError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	astroClient := handlermocks.NewMockAstroProfileGetter(ctrl)

	p := handlers.NewProfileProcessor(userRepo, reqRepo, astroClient, zap.NewNop())

	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-1",
		"profile": map[string]any{
			"user_id":    "not-uuid",
			"birth_date": "bad-date",
		},
	})
	err := p.Handle(context.Background(), payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation")
}

func TestProfileProcessor_AstroAPIFailureMarksRequestRetry(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	astroClient := handlermocks.NewMockAstroProfileGetter(ctrl)

	astroErr := errors.New("astro api down")
	reqRepo.EXPECT().
		Get(gomock.Any(), "req-1").
		Return(requests.Request{RequestID: "req-1", Status: requests.StatusPending}, nil).Times(2)
	reqRepo.EXPECT().
		StartProcessing(gomock.Any(), "req-1").
		Return(true, nil).Times(1)
	astroClient.EXPECT().
		GetAstroProfileContext(gomock.Any(), "1990-01-01", "Moscow").
		Return(alisa.AstroProfile{}, astroErr).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-1", requests.StatusRetry, gomock.Nil(), gomock.Any()).
		Return(nil).Times(1)

	p := handlers.NewProfileProcessor(userRepo, reqRepo, astroClient, zap.NewNop())
	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-1",
		"profile": map[string]any{
			"user_id":       validUserID,
			"birth_date":    "1990-01-01",
			"birth_place":   "Moscow",
			"consent_given": true,
		},
	})
	err := p.Handle(context.Background(), payload)
	require.ErrorIs(t, err, astroErr)
}

func TestRecommendProcessor_HappyPath(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	rulesRepo := rulemocks.NewMockRepository(ctrl)
	ai := almocks.NewMockGenerator(ctrl)

	reqRepo.EXPECT().
		Get(gomock.Any(), "req-2").
		Return(requests.Request{RequestID: "req-2", Status: requests.StatusPending}, nil).Times(1)
	reqRepo.EXPECT().
		StartProcessing(gomock.Any(), "req-2").
		Return(true, nil).Times(1)

	userRepo.EXPECT().Get(gomock.Any(), validUserID).
		Return(user.User{UserID: validUserID, BirthDate: "1990-01-01"}, nil).Times(1)
	rulesRepo.EXPECT().Match(gomock.Any(), gomock.Any()).Return([]string{"luxury"}, nil).Times(1)
	ai.EXPECT().Generate(gomock.Any(), gomock.Any()).Return("awesome", nil).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-2", "completed", gomock.Any(), "").
		Return(nil).Times(1)

	p := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, nil, zap.NewNop())

	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-2",
		"recommend": map[string]any{
			"user_id":  validUserID,
			"scenario": "personal_style",
			"context":  map[string]any{"triggers": []string{"Полнолуние"}},
		},
	})
	require.NoError(t, p.Handle(context.Background(), payload))
}

func TestRecommendProcessor_DuplicateCompletedIsSkipped(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	rulesRepo := rulemocks.NewMockRepository(ctrl)
	ai := almocks.NewMockGenerator(ctrl)

	reqRepo.EXPECT().
		Get(gomock.Any(), "req-dup").
		Return(requests.Request{RequestID: "req-dup", Status: requests.StatusCompleted, Result: []byte(`{"ok":true}`)}, nil).Times(1)

	p := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, nil, zap.NewNop())
	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-dup",
		"recommend": map[string]any{
			"user_id":  validUserID,
			"scenario": "personal_style",
			"context":  map[string]any{"triggers": []string{"Полнолуние"}},
		},
	})
	require.NoError(t, p.Handle(context.Background(), payload))
}

func TestRecommendProcessor_RedeliveredProcessingWithoutResultIsProcessed(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	rulesRepo := rulemocks.NewMockRepository(ctrl)
	ai := almocks.NewMockGenerator(ctrl)

	reqRepo.EXPECT().
		Get(gomock.Any(), "req-processing").
		Return(requests.Request{RequestID: "req-processing", Status: requests.StatusProcessing}, nil).Times(1)
	reqRepo.EXPECT().
		StartProcessing(gomock.Any(), "req-processing").
		Return(true, nil).Times(1)
	userRepo.EXPECT().Get(gomock.Any(), validUserID).
		Return(user.User{UserID: validUserID, BirthDate: "1990-01-01"}, nil).Times(1)
	rulesRepo.EXPECT().Match(gomock.Any(), gomock.Any()).Return([]string{"luxury"}, nil).Times(1)
	ai.EXPECT().Generate(gomock.Any(), gomock.Any()).Return("awesome", nil).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-processing", requests.StatusCompleted, gomock.Any(), "").
		Return(nil).Times(1)

	p := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, nil, zap.NewNop())
	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-processing",
		"recommend": map[string]any{
			"user_id":  validUserID,
			"scenario": "personal_style",
		},
	})
	require.NoError(t, p.Handle(context.Background(), payload))
}

func TestRecommendProcessor_RetryWithoutResultIsProcessed(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	rulesRepo := rulemocks.NewMockRepository(ctrl)
	ai := almocks.NewMockGenerator(ctrl)

	reqRepo.EXPECT().
		Get(gomock.Any(), "req-retry").
		Return(requests.Request{RequestID: "req-retry", Status: requests.StatusRetry, AttemptCount: 1}, nil).Times(1)
	reqRepo.EXPECT().
		StartProcessing(gomock.Any(), "req-retry").
		Return(true, nil).Times(1)
	userRepo.EXPECT().Get(gomock.Any(), validUserID).
		Return(user.User{UserID: validUserID, BirthDate: "1990-01-01"}, nil).Times(1)
	rulesRepo.EXPECT().Match(gomock.Any(), gomock.Any()).Return([]string{"luxury"}, nil).Times(1)
	ai.EXPECT().Generate(gomock.Any(), gomock.Any()).Return("ok after retry", nil).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-retry", requests.StatusCompleted, gomock.Any(), "").
		Return(nil).Times(1)

	p := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, nil, zap.NewNop())
	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-retry",
		"recommend": map[string]any{
			"user_id":  validUserID,
			"scenario": "personal_style",
		},
	})
	require.NoError(t, p.Handle(context.Background(), payload))
}

func TestRecommendProcessor_FifthFailureMarksFailed(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	rulesRepo := rulemocks.NewMockRepository(ctrl)
	ai := almocks.NewMockGenerator(ctrl)

	workerErr := errors.New("user repo unavailable")
	reqRepo.EXPECT().
		Get(gomock.Any(), "req-last").
		Return(requests.Request{RequestID: "req-last", Status: requests.StatusRetry, AttemptCount: 4}, nil).Times(2)
	reqRepo.EXPECT().
		StartProcessing(gomock.Any(), "req-last").
		Return(true, nil).Times(1)
	userRepo.EXPECT().Get(gomock.Any(), validUserID).
		Return(user.User{}, workerErr).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-last", requests.StatusFailed, gomock.Nil(), gomock.Any()).
		Return(nil).Times(1)

	p := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, nil, zap.NewNop())
	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-last",
		"recommend": map[string]any{
			"user_id":  validUserID,
			"scenario": "personal_style",
		},
	})
	err := p.Handle(context.Background(), payload)
	require.ErrorIs(t, err, workerErr)
}
