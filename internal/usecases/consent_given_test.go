package usecases

import (
	"astroapi/internal/usecases/repositories"
	"astroapi/internal/usecases/repositories/domain"
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func Test_ProcessPersonalData_NoConsent(t *testing.T) {
	ctx := context.Background()

	db := setupTestDB(t, ctx)
	defer db.Close()

	dbRepo := repositories.NewDBPersonalDataRepository(
		db,
		[]byte("12345678901234567890123456789012"),
	)

	cacheRepo := &fakeCacheRepo{}

	uc := NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

	input := ProcessPersonalDataInput{
		PersonalData: domain.PersonalData{
			UserID:       "telegram_12345",
			DOB:          "1990-05-15",
			ConsentGiven: false,
		},
	}

	err := uc.Execute(ctx, input)
	require.NoError(t, err)

	var count int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE user_id = $1`,
		input.PersonalData.UserID,
	).Scan(&count)

	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func Test_ProcessPersonalData_WithConsent(t *testing.T) {
	ctx := context.Background()

	db := setupTestDB(t, ctx)
	defer db.Close()

	dbRepo := repositories.NewDBPersonalDataRepository(
		db,
		[]byte("12345678901234567890123456789012"),
	)

	cacheRepo := &fakeCacheRepo{}

	uc := NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

	input := ProcessPersonalDataInput{
		PersonalData: domain.PersonalData{
			UserID:       "telegram_12345",
			DOB:          "1990-05-15",
			ConsentGiven: true,
		},
	}

	err := uc.Execute(ctx, input)
	require.NoError(t, err)

	var userID string
	err = db.QueryRowContext(ctx,
		`SELECT user_id FROM users WHERE user_id = $1`,
		input.PersonalData.UserID,
	).Scan(&userID)

	require.NoError(t, err)
	require.Equal(t, input.PersonalData.UserID, userID)
}

func setupTestDB(t *testing.T, ctx context.Context) *sql.DB {
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

	// ждём пока Postgres поднимется
	require.Eventually(t, func() bool {
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			return false
		}
		return db.Ping() == nil
	}, 10*time.Second, 500*time.Millisecond)

	_, err = db.Exec(`
		CREATE TABLE users (
			user_id TEXT PRIMARY KEY,
			encrypted_dob BYTEA NOT NULL,
			consent_given BOOLEAN,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)
	`)
	require.NoError(t, err)

	return db
}

type fakeCacheRepo struct{}

func (f *fakeCacheRepo) Save(ctx context.Context, data domain.PersonalData) error {
	return nil
}
