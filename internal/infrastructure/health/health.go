package health

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	TTL = 15 * time.Second
)

//go:generate mockgen -source=health.go -destination=mocks/mock_health.go -package=mocks

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthServiceRepo struct {
	services map[string]Pinger
	mu       sync.RWMutex
	data     map[string]ServiceState
}

type ServiceState struct {
	State   bool
	Error   error
	Updated time.Time
}

func NewHealthServiceRepo(db Pinger, msg Pinger) *HealthServiceRepo {
	services := map[string]Pinger{
		"db":   db,
		"nats": msg,
	}
	data := make(map[string]ServiceState, len(services))
	for name := range services {
		data[name] = ServiceState{}
	}

	return &HealthServiceRepo{services: services, mu: sync.RWMutex{}, data: data}
}

func (s *HealthServiceRepo) GetInfraStatus(ctx context.Context) (any, error) {
	if s == nil || s.data == nil || s.services == nil {

		return "", fmt.Errorf("health service not initialized")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ok := true
	resp := make(map[string]ServiceState, len(s.services))

	for srvID := range s.services {
		state, okInMap := s.data[srvID]
		if !okInMap {
			return nil, fmt.Errorf("internal error: cached state missing for known service %s", srvID)
		}
		resp[srvID] = state
		if state.State == false || time.Since(state.Updated) > TTL {
			ok = false
		}
	}

	var err error
	if !ok {
		err = errors.New("one or more services are unavailable")
	}

	return resp, err
}

func (s *HealthServiceRepo) GetSrvCachedStatus(ctx context.Context, srvID string) (ServiceState, error) {

	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.data[srvID]

	if !ok {
		return ServiceState{}, fmt.Errorf("unknown service id: %s", srvID)
	}

	return ServiceState{}, fmt.Errorf("last status update: %s", state.Updated.Format(time.RFC3339))

}

func (s *HealthServiceRepo) PingInfra(ctx context.Context) {

	if s == nil || s.data == nil || s.services == nil {
		return
	}
	for k := range s.services {
		s.PingSrv(ctx, k)
	}
}

func (s *HealthServiceRepo) PingSrv(ctx context.Context, srvID string) {

	pingctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.pingStatus(pingctx, srvID)

	state := ServiceState{
		State:   err == nil,
		Error:   err,
		Updated: time.Now(),
	}

	s.mu.Lock()
	s.data[srvID] = state
	s.mu.Unlock()

}

func (s *HealthServiceRepo) pingStatus(ctx context.Context, srvID string) error {

	service, ok := s.services[srvID]
	if !ok {
		return fmt.Errorf("unknown service  %s", srvID)
	}
	return service.Ping(ctx)

}
