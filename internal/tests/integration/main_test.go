//go:build integration

package integration

import (
	"astroapi/internal/logger"
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

type TestEnv struct {
	Nats    tc.Container
	NatsURL string

	Postgres    tc.Container
	PostgresDSN string
}

var testEnv *TestEnv

func TestMain(m *testing.M) {
	ctx := context.Background()

	// 0. Инициализируем логгер
	logger, err := logger.NewLogger("integration_tests", "debug")
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Printf("Failed to sync logger: %v", err)
		}
	}()

	// 1. Инициализация окружения
	env, err := setupIntegrationEnv(ctx)
	if err != nil {
		logger.Fatal("Failed to setup env", zap.Error(err))
	}
	testEnv = env

	// 2. Запуск тестов
	code := m.Run()

	// 3. Очистка (Terminate удаляет контейнеры)
	if err := testEnv.Nats.Terminate(ctx); err != nil {
		logger.Error("Failed to terminate Nats", zap.Error(err))
	}
	if err := testEnv.Postgres.Terminate(ctx); err != nil {
		logger.Error("Failed to terminate Postgres", zap.Error(err))
	}

	os.Exit(code)
}

func setupIntegrationEnv(ctx context.Context) (*TestEnv, error) {
	env := &TestEnv{}

	// --- NATS JetStream ---
	natsContainer, err := nats.Run(ctx, "nats:alpine",
		testcontainers.WithCmd("-js"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start Nats container: %w", err)
	}

	natsURL, err := natsContainer.ConnectionString(ctx)
	if err != nil {
		return nil, err
	}
	env.Nats = natsContainer
	env.NatsURL = natsURL

	// --- PostgreSQL ---
	postgresContainer, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(1).
				WithStartupTimeout(5*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start Postgres container: %w", err)
	}

	postgresDSN, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}
	env.Postgres = postgresContainer
	env.PostgresDSN = postgresDSN

	return env, nil
}
