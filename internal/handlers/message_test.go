package handlers_test

import (
	"context"
	"errors"
	"testing"

	"astroapi/internal/handlers"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMsgRouter_DispatchesToRegisteredHandler(t *testing.T) {
	t.Parallel()
	router := handlers.NewMsgRouter(zap.NewNop())

	var gotSubject string
	var gotData []byte
	router.Register("astro.events.profile", handlers.HandlerFunc(func(_ context.Context, data []byte) error {
		gotSubject = "astro.events.profile"
		gotData = data
		return nil
	}))

	payload := []byte("hello")
	err := router.Dispatch(context.Background(), "astro.events.profile", payload)
	require.NoError(t, err)
	require.Equal(t, "astro.events.profile", gotSubject)
	require.Equal(t, payload, gotData)
}

func TestMsgRouter_NoHandlerReturnsError(t *testing.T) {
	t.Parallel()
	router := handlers.NewMsgRouter(zap.NewNop())

	err := router.Dispatch(context.Background(), "unknown.subject", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no handler found")
}

func TestMsgRouter_HandlerErrorPropagates(t *testing.T) {
	t.Parallel()
	router := handlers.NewMsgRouter(zap.NewNop())

	sentinel := errors.New("boom")
	router.Register("s", handlers.HandlerFunc(func(context.Context, []byte) error {
		return sentinel
	}))

	err := router.Dispatch(context.Background(), "s", nil)
	require.ErrorIs(t, err, sentinel)
}

func TestMsgRouter_LatestRegistrationWins(t *testing.T) {
	t.Parallel()
	router := handlers.NewMsgRouter(zap.NewNop())

	calls := 0
	router.Register("s", handlers.HandlerFunc(func(context.Context, []byte) error {
		calls = 1
		return nil
	}))
	router.Register("s", handlers.HandlerFunc(func(context.Context, []byte) error {
		calls = 2
		return nil
	}))

	require.NoError(t, router.Dispatch(context.Background(), "s", nil))
	require.Equal(t, 2, calls)
}
