package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	almocks "astroapi/internal/alisa/mocks"
	"astroapi/internal/handlers"
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

	userRepo.EXPECT().
		Save(gomock.Any(), gomock.AssignableToTypeOf(user.User{})).
		Return(nil).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-1", "completed", gomock.Nil(), "").
		Return(nil).Times(1)

	p := handlers.NewProfileProcessor(userRepo, reqRepo, zap.NewNop())

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
	// Save и UpdateStatus не должны вызываться

	p := handlers.NewProfileProcessor(userRepo, reqRepo, zap.NewNop())

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

func TestProfileProcessor_SaveFailureMarksRequestFailed(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)

	saveErr := errors.New("db down")
	userRepo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(saveErr).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-1", "failed", gomock.Nil(), gomock.Any()).
		Return(nil).Times(1)

	p := handlers.NewProfileProcessor(userRepo, reqRepo, zap.NewNop())
	payload, _ := json.Marshal(map[string]any{
		"request_id": "req-1",
		"profile": map[string]any{
			"user_id":       validUserID,
			"birth_date":    "1990-01-01",
			"consent_given": true,
		},
	})
	err := p.Handle(context.Background(), payload)
	require.ErrorIs(t, err, saveErr)
}

func TestRecommendProcessor_HappyPath(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	userRepo := usermocks.NewMockRepository(ctrl)
	reqRepo := reqmocks.NewMockRepository(ctrl)
	rulesRepo := rulemocks.NewMockRepository(ctrl)
	ai := almocks.NewMockGenerator(ctrl)

	userRepo.EXPECT().Get(gomock.Any(), validUserID).
		Return(user.User{UserID: validUserID, BirthDate: "1990-01-01"}, nil).Times(1)
	rulesRepo.EXPECT().Match(gomock.Any(), gomock.Any()).Return([]string{"luxury"}, nil).Times(1)
	ai.EXPECT().Generate(gomock.Any(), gomock.Any()).Return("awesome", nil).Times(1)
	reqRepo.EXPECT().
		UpdateStatus(gomock.Any(), "req-2", "completed", gomock.Any(), "").
		Return(nil).Times(1)

	p := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, zap.NewNop())

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
