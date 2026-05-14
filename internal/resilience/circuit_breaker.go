package resilience

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	StateClosed = iota
	StateOpen
	StateHalfOpen
)

const (
	AlisaServiceName = "alisa_ai"
)

var ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

type ServiceUnavailableError struct {
	Service string
	Cause   error
}

func (e *ServiceUnavailableError) Error() string {
	if e == nil {
		return "service unavailable"
	}

	if e.Cause != nil {
		return fmt.Sprintf("service %s unavailable: %v", e.Service, e.Cause)
	}

	return fmt.Sprintf("service %s unavailable", e.Service)
}

func (e *ServiceUnavailableError) Unwrap() error {
	if e == nil {
		return nil
	}

	if e.Cause != nil {
		return e.Cause
	}

	return ErrCircuitBreakerOpen
}

func IsServiceUnavailable(err error) bool {
	var serviceErr *ServiceUnavailableError

	return errors.As(err, &serviceErr) || errors.Is(err, ErrCircuitBreakerOpen)
}

type StateReporter interface {
	SetCircuitBreakerState(service string, state int)
}

type CircuitBreaker struct {
	serviceName      string
	failureThreshold int
	halfOpenTimeout  time.Duration
	logger           *zap.Logger
	reporter         StateReporter
	now              func() time.Time

	mu                  sync.Mutex
	state               int
	consecutiveFailures int
	openedAt            time.Time
}

func NewCircuitBreaker(
	serviceName string,
	failureThreshold int,
	halfOpenTimeout time.Duration,
	logger *zap.Logger,
	reporter StateReporter,
) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}

	if halfOpenTimeout <= 0 {
		halfOpenTimeout = 30 * time.Second
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	cb := &CircuitBreaker{
		serviceName:      serviceName,
		failureThreshold: failureThreshold,
		halfOpenTimeout:  halfOpenTimeout,
		logger:           logger,
		reporter:         reporter,
		now:              time.Now,
		state:            StateClosed,
	}

	cb.reportStateLocked(StateClosed)

	return cb
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn()
	cb.afterRequest(err)

	return err
}

func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := cb.now()

	if cb.state == StateOpen {
		if now.Sub(cb.openedAt) >= cb.halfOpenTimeout {
			cb.transitionLocked(StateHalfOpen, "half-open timeout elapsed")
		} else {
			return &ServiceUnavailableError{
				Service: cb.serviceName,
				Cause:   ErrCircuitBreakerOpen,
			}
		}
	}

	return nil
}

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		cb.consecutiveFailures = 0

		if cb.state != StateClosed {
			cb.transitionLocked(StateClosed, "request succeeded")
		} else {
			cb.reportStateLocked(StateClosed)
		}

		return
	}

	cb.consecutiveFailures++

	if cb.state == StateHalfOpen || cb.consecutiveFailures >= cb.failureThreshold {
		cb.openedAt = cb.now()
		cb.transitionLocked(StateOpen, "failure threshold reached")

		return
	}

	cb.reportStateLocked(cb.state)
}

func (cb *CircuitBreaker) CurrentState() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen && cb.now().Sub(cb.openedAt) >= cb.halfOpenTimeout {
		return StateHalfOpen
	}

	return cb.state
}

func (cb *CircuitBreaker) transitionLocked(newState int, reason string) {
	previous := cb.state
	cb.state = newState

	if newState != StateOpen {
		cb.openedAt = time.Time{}
	}

	cb.reportStateLocked(newState)

	cb.logger.Info(
		"circuit breaker state changed",
		zap.String("service", cb.serviceName),
		zap.String("from", stateName(previous)),
		zap.String("to", stateName(newState)),
		zap.String("reason", reason),
	)
}

func (cb *CircuitBreaker) reportStateLocked(state int) {
	if cb.reporter == nil {
		return
	}

	cb.reporter.SetCircuitBreakerState(cb.serviceName, state)
}

func stateName(state int) string {
	switch state {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}
