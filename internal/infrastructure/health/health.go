package health

import (
	"context"
	"fmt"
)

//go:generate mockgen -source=health.go -destination=mocks/mock_health.go -package=mocks

type AIClient interface {
	Ping(ctx context.Context) error
}

type DB interface {
	Ping(ctx context.Context) error
}

type MsgClient interface {
	Ping(ctx context.Context) error
}

type HealthServiceRepo struct {
	db  DB
	msg MsgClient
	ai  AIClient
}

func NewHealthServiceRepo(db DB, msg MsgClient, ai AIClient) *HealthServiceRepo {
	return &HealthServiceRepo{db: db, msg: msg, ai: ai}
}

func (s *HealthServiceRepo) GetInfraStatus(ctx context.Context) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("health service not initialized")
	}

	return "connected", nil
}
