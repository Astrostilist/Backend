package resilience

import (
	"testing"
	"time"
)

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := NewCircuitBreaker("test", 5, 30*time.Second, nil)

	if cb == nil {
		t.Fatal("expected circuit breaker instance")
	}
}
