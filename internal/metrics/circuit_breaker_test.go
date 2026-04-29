package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricsHandlerExposesCircuitBreakerState(t *testing.T) {
	registry := NewRegistry()
	registry.SetCircuitBreakerState("alisa_ai", 0)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	registry.Handler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `circuit_breaker_state{service="alisa_ai"} 0`)
}
