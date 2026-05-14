package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://postgres:test@127.0.0.1:%s/testdb?sslmode=disable", port.Port())
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	err = db.PingContext(ctx)
	require.NoError(t, err)

	createTables(t, db)

	return db, func() {
		db.Close()
		container.Terminate(ctx)
	}
}

func createTables(t *testing.T, db *sql.DB) {
	schema := `
    CREATE TABLE users (user_id TEXT PRIMARY KEY);
    CREATE TABLE user_consents (id SERIAL, user_id TEXT REFERENCES users(user_id));
    CREATE TABLE generation_results (id SERIAL, user_id TEXT REFERENCES users(user_id));
    CREATE TABLE feedback (id SERIAL, user_id TEXT REFERENCES users(user_id));
    `
	_, err := db.Exec(schema)
	require.NoError(t, err)
}

// успешное удаление
func TestDeleteUser_Success(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	// заполняем данные
	userID := "user-123"
	_, err := db.Exec(`INSERT INTO users (user_id) VALUES ($1)`, userID)
	require.NoError(t, err)
	// добавляем связанные записи
	_, _ = db.Exec(`INSERT INTO user_consents (user_id) VALUES ($1)`, userID)
	_, _ = db.Exec(`INSERT INTO generation_results (user_id) VALUES ($1)`, userID)
	_, _ = db.Exec(`INSERT INTO feedback (user_id) VALUES ($1)`, userID)

	repo := NewUserRepository(db)
	found, err := repo.DeleteUsers(context.Background(), userID)
	require.NoError(t, err)
	require.True(t, found)

	// проверяем, что записей нет
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE user_id = $1`, userID).Scan(&count)
	require.Equal(t, 0, count)
	db.QueryRow(`SELECT COUNT(*) FROM user_consents WHERE user_id = $1`, userID).Scan(&count)
	require.Equal(t, 0, count)
	db.QueryRow(`SELECT COUNT(*) FROM generation_results WHERE user_id = $1`, userID).Scan(&count)
	require.Equal(t, 0, count)
	db.QueryRow(`SELECT COUNT(*) FROM feedback WHERE user_id = $1`, userID).Scan(&count)
	require.Equal(t, 0, count)
}

// удаление несуществующего → возвращает found=false
func TestDeleteUser_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewUserRepository(db)
	found, err := repo.DeleteUsers(context.Background(), "user-456")
	require.NoError(t, err)
	require.False(t, found)
}

// rollback (симулируем ошибку удалив колонку user_id в feadback)
func TestDeleteUser_RollbackOnError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	userID := "user-123"
	_, err := db.Exec(`INSERT INTO users (user_id) VALUES ($1)`, userID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO feedback (user_id) VALUES ($1)`, userID)
	require.NoError(t, err)

	// удаляем колонку
	_, err = db.Exec(`ALTER TABLE feedback DROP COLUMN user_id`)
	require.NoError(t, err)

	repo := NewUserRepository(db)
	found, err := repo.DeleteUsers(context.Background(), userID)
	require.Error(t, err)
	require.False(t, found)

	// проверяем, что данные остались нетронутыми
	var uesersCount int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE user_id = $1`, userID).Scan(&uesersCount)
	require.Equal(t, 1, uesersCount)
	var feedbackCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM feedback`).Scan(&feedbackCount)
	require.NoError(t, err)
	require.Equal(t, 1, feedbackCount, "строка в feedback должна остаться")
}
