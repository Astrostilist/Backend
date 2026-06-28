package usecases

import (
	"astroapi/internal/repositories"
	"astroapi/internal/repositories/domain"
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDbProfile(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf(
		"postgres://test:test@%s:%s/testdb?sslmode=disable",
		host,
		port.Port(),
	)

	var db *sql.DB

	require.Eventually(t, func() bool {
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			return false
		}
		return db.Ping() == nil
	}, 10*time.Second, 500*time.Millisecond)

	_, err = db.Exec(`
	CREATE TABLE astro_profiles (
	id UUID PRIMARY KEY,
	user_id VARCHAR(255) NOT NULL,
	profile_hash VARCHAR(64) NOT NULL,
	dob_encrypted BYTEA,
	consent_given BOOLEAN DEFAULT false,
	profile_data JSONB NOT NULL,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	UNIQUE(user_id, profile_hash)
	)
	`)
	require.NoError(t, err)

	return db
}

// startMemcached запускает контейнер и возвращает адрес "host:port".
func startMemcached(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "memcached:1.6-alpine",
			ExposedPorts: []string{"11211/tcp"},
			WaitingFor:   wait.ForListeningPort("11211/tcp"),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "11211/tcp")
	require.NoError(t, err)

	return fmt.Sprintf("%s:%s", host, port.Port())
}

// stubDBRepo используется в тестах кеша, где БД не нужна.
type stubDBRepo struct{}

func (s *stubDBRepo) Save(_ context.Context, _ domain.AstroProfile) error { return nil }
func (s *stubDBRepo) ReceivingByHash(_ context.Context, _ string) (*domain.AstroProfile, error) {
	return nil, nil
}

// TestAstroProfileUseCaseBD проверяет сохранение и получение через БД.
func TestAstroProfileUseCaseDB(t *testing.T) {
	ctx := context.Background()

	db := setupTestDbProfile(t, ctx)
	defer db.Close()

	dbRepo := repositories.NewDbAstroProfileRepo(
		db,
		[]byte("12345678901234567890123456789012"),
	)

	addr := startMemcached(t)
	cacheRepo := repositories.NewCacheAstroProfileRepo([]string{addr}, 5*time.Minute)

	uc := NewProcessAstroProfileUseCase(dbRepo, cacheRepo)

	profile := domain.AstroProfile{
		ID:          "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
		UserID:      "user-123",
		ProfileHash: "hash-bd-test",
		DOB:         "01-01-1999",
		ProfileData: domain.ProfileData{
			Sun:     "sun-sing",
			Moon:    "moon-sing",
			Venus:   "venus-sing",
			Mars:    "mars-sing",
			Jupiter: "jupiter-sing",
			Saturn:  "saturn-sing",
			Mercury: "mercury-sing",
			Neptune: "neptune-sing",
			Uranus:  "uranus-sing",
			Pluto:   "pluto-sing",
		},
		ConsentGiven: true,
	}

	err := uc.ExecuteSave(ctx, profile)
	require.NoError(t, err)

	got, err := uc.ExecuteReceivingByHash(ctx, profile.ProfileHash)
	require.NoError(t, err)
	require.Equal(t, &profile, got)
}

// TestAstroProfileUseCaseCache проверяет сохранение и получение через кеш (без согласия).
func TestAstroProfileUseCaseCache(t *testing.T) {
	ctx := context.Background()

	addr := startMemcached(t)
	cacheRepo := repositories.NewCacheAstroProfileRepo([]string{addr}, 5*time.Minute)

	uc := NewProcessAstroProfileUseCase(&stubDBRepo{}, cacheRepo)

	profile := domain.AstroProfile{
		ID:          "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
		UserID:      "user-456",
		ProfileHash: "hash-cache-test",
		DOB:         "15-06-1990",
		ProfileData: domain.ProfileData{
			Sun:     "sun-sing",
			Moon:    "moon-sing",
			Venus:   "venus-sing",
			Mars:    "mars-sing",
			Jupiter: "jupiter-sing",
			Saturn:  "saturn-sing",
			Mercury: "mercury-sing",
			Neptune: "neptune-sing",
			Uranus:  "uranus-sing",
			Pluto:   "pluto-sing",
		},
		ConsentGiven: false,
	}

	err := uc.ExecuteSave(ctx, profile)
	require.NoError(t, err)

	got, err := uc.ExecuteReceivingByHash(ctx, profile.ProfileHash)
	require.NoError(t, err)
	require.Equal(t, &profile, got)
}
