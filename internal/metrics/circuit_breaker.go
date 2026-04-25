package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	states map[string]int
}

func NewRegistry() *Registry {
	return &Registry{states: make(map[string]int)}
}

func (r *Registry) SetCircuitBreakerState(service string, state int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[service] = state
}

func (r *Registry) Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(r.Render()))
}

func (r *Registry) Render() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	services := make([]string, 0, len(r.states))
	for service := range r.states {
		services = append(services, service)
	}
	sort.Strings(services)

	var builder strings.Builder
	builder.WriteString("# HELP circuit_breaker_state Current state of the circuit breaker (0=closed, 1=open, 2=half-open)\n")
	builder.WriteString("# TYPE circuit_breaker_state gauge\n")
	for _, service := range services {
		builder.WriteString(fmt.Sprintf("circuit_breaker_state{service=\"%s\"} %d\n", service, r.states[service]))
	}
	return builder.String()
}
