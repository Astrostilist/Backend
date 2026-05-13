package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	TTL = 5 * time.Second
)

var services map[string]Pinger

//go:generate mockgen -source=health.go -destination=mocks/mock_health.go -package=mocks

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthServiceRepo struct {
	mu   sync.RWMutex
	data map[string]ServiceState
}

type ServiceState struct {
	State   bool
	Error   error
	Updated time.Time
}

func NewHealthServiceRepo(db Pinger, msg Pinger, ai Pinger) *HealthServiceRepo {
	services = map[string]Pinger{
		"db":       db,
		"nats":     msg,
		"alisa_ai": ai,
	}
	data := make(map[string]ServiceState, len(services))
	for name := range services {
		data[name] = ServiceState{}
	}

	return &HealthServiceRepo{data: data}
}

func (s *HealthServiceRepo) GetInfraStatus(ctx context.Context) (string, error) {
	if s == nil || s.data == nil || services == nil {
		return "", fmt.Errorf("health service not initialized")
	}

	return "connected", nil
}

func (s *HealthServiceRepo) GetSrvStatus(ctx context.Context, srvId string) (ServiceState, error) {

	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.data[srvId]

	if !ok {
		return ServiceState{}, fmt.Errorf("unknown service id: %s", srvId)
	}

	if time.Since(state.Updated) < TTL {
		return state, nil
	}
	return ServiceState{}, fmt.Errorf("last status update: %s", state.Updated.Format(time.RFC3339))

}

func (s *HealthServiceRepo) PingSrvAndStoreStatus(ctx context.Context, srvId string) (ServiceState, error) {

	pingctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.pingStatus(pingctx, srvId)

	state := ServiceState{
		State:   err == nil,
		Error:   err,
		Updated: time.Now(),
	}

	s.mu.Lock()
	s.data[srvId] = state
	s.mu.Unlock()

	return state, nil

}

func (s *HealthServiceRepo) pingStatus(ctx context.Context, srvId string) error {

	service, ok := services[srvId]
	if !ok {
		return fmt.Errorf("unknown service  %s", srvId)
	}
	return service.Ping(ctx)

}
