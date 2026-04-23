package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	astromidware "astroapi/internal/middleware"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLogger_LogsMethodPathStatus(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	mw := astromidware.RequestLogger(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("brew"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/profile", nil)
	req.Header.Set("User-Agent", "go-test")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusTeapot, rr.Code)

	entries := logs.All()
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, "HTTP request", entry.Message)

	fields := entry.ContextMap()
	require.Equal(t, http.MethodPost, fields["method"])
	require.Equal(t, "/api/v1/astro/profile", fields["path"])
	require.EqualValues(t, http.StatusTeapot, fields["status"])
	require.Equal(t, "go-test", fields["user_agent"])
	require.Contains(t, fields, "duration")
}

func TestRequestLogger_DefaultsToHandlerStatus(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	mw := astromidware.RequestLogger(logger)
	// handler ничего не пишет — должен записаться 200 по умолчанию
	handler := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	entries := logs.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.EqualValues(t, http.StatusOK, fields["status"])
}
