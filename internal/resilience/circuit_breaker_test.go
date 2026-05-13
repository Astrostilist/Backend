package resilience

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCircuitBreakerInitialState(t *testing.T) {
	logger := zap.NewNop()

	cb := NewCircuitBreaker(
		"test",
		5,
		30*time.Second,
		logger,
		nil,
	)

	if cb == nil {
		t.Fatal("expected circuit breaker instance")
	}
}
