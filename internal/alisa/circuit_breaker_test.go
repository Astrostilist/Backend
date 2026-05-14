package alisa

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"astroapi/internal/resilience"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type testStateReporter struct {
	states sync.Map
}

func (r *testStateReporter) SetCircuitBreakerState(service string, state int) {
	r.states.Store(service, state)
}

func (r *testStateReporter) State(service string) (int, bool) {
	value, ok := r.states.Load(service)
	if !ok {
		return 0, false
	}

	state, ok := value.(int)
	return state, ok
}

func (r *testStateReporter) Render() string {
	var builder strings.Builder

	r.states.Range(func(service, state any) bool {
		_, _ = fmt.Fprintf(
			&builder,
			"circuit_breaker_state{service=%q} %d\n",
			service,
			state,
		)
		return true
	})

	return builder.String()
}

func TestAlisaCircuitBreakerOpensAfterFiveFailuresAndStopsRequests(t *testing.T) {
	var requestsCount atomic.Int32

	core, observedLogs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	metricsReporter := &testStateReporter{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
	}))
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test-key", "gpt://folder/model/latest", ClientOptions{
		HTTPClient: server.Client(),
		Logger:     logger,
		Metrics:    metricsReporter,
		MaxRetries: 0,
	})

	for i := 0; i < 5; i++ {
		_, err := client.Generate(context.Background(), "test prompt")
		require.Error(t, err)
	}

	countAfterOpen := requestsCount.Load()
	require.Equal(t, int32(5), countAfterOpen)
	require.Contains(t, metricsReporter.Render(), `circuit_breaker_state{service="alisa_ai"}`)

	state, ok := metricsReporter.State("alisa_ai")
	require.True(t, ok)
	require.Equal(t, resilience.StateOpen, state)

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
	metricsReporter := &testStateReporter{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsCount.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
	}))
	defer server.Close()

	client := NewAstroAPIClient(server.URL, nil, logger, AstroAPIClientOptions{
		HTTPClient: server.Client(),
		Metrics:    metricsReporter,
	})

	for i := 0; i < 5; i++ {
		_, err := client.GetAstroProfile("1992-06-26", "Moscow")
		require.Error(t, err)
	}

	countAfterOpen := requestsCount.Load()
	require.Equal(t, int32(5), countAfterOpen)

	state, ok := metricsReporter.State("astro_api")
	require.True(t, ok)
	require.Equal(t, resilience.StateOpen, state)

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
