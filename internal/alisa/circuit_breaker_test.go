package alisa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"astroapi/config"
	"astroapi/internal/circutebreaker"
	"astroapi/internal/metrics"
	"astroapi/internal/resilience"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAlisaCircuitBreakerOpensAfterFiveFailuresAndStopsRequests(t *testing.T) {
	var requestsCount atomic.Int32
	cfg := config.Load()
	metrics.Initialize(cfg)
	core, observedLogs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	metricsRegistry := circutebreaker.NewRegistry()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
	}))
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test-key", "gpt://folder/model/latest", ClientOptions{
		HTTPClient: server.Client(),
		Logger:     logger,
		Metrics:    metricsRegistry,
		MaxRetries: 0,
	})

	for i := 0; i < 5; i++ {
		_, err := client.Generate(context.Background(), "test prompt")
		require.Error(t, err)
	}

	countAfterOpen := requestsCount.Load()
	require.Equal(t, int32(5), countAfterOpen)
	require.Contains(t, metricsRegistry.Render(), `circuit_breaker_state{service="alisa_ai"} 1`)

	_, err := client.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	require.True(t, resilience.IsServiceUnavailable(err))
	require.Equal(t, countAfterOpen, requestsCount.Load())
	require.True(t, hasTransition(observedLogs.AllUntimed(), "open"))
}

func TestAstroCircuitBreakerOpensAfterFiveFailuresAndStopsRequests(t *testing.T) {
	var requestsCount atomic.Int32
	core, observedLogs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsCount.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
	}))
	defer server.Close()

	client := NewAstroAPIClient(server.URL, nil, logger, AstroAPIClientOptions{
		HTTPClient: server.Client(),
		Metrics:    circutebreaker.NewRegistry(),
	})

	for i := 0; i < 5; i++ {
		_, err := client.GetAstroProfile("1992-06-26", "Moscow")
		require.Error(t, err)
	}

	countAfterOpen := requestsCount.Load()
	require.Equal(t, int32(5), countAfterOpen)

	_, err := client.GetAstroProfile("1992-06-26", "Moscow")
	require.Error(t, err)
	require.True(t, resilience.IsServiceUnavailable(err))
	require.Equal(t, countAfterOpen, requestsCount.Load())
	require.True(t, hasTransition(observedLogs.AllUntimed(), "open"))
}

func hasTransition(entries []observer.LoggedEntry, to string) bool {
	found := false
	for _, entry := range entries {
		if entry.Message == "circuit breaker state changed" {
			for _, field := range entry.Context {
				if field.Key == "to" && strings.EqualFold(field.String, to) {
					found = true
				}
			}
		}
	}
	return found
}
