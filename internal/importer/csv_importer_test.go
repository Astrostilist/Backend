//go:build integration

package importer

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestRunImportCSV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "positive - read csv",
			filePath: "file1.csv",
			want:     true,
		},
	}

	ctx := context.Background()

	dbName := "test"
	dbUser := "user"
	DBPassword := "password"

	ctr, err := postgres.Run(
		ctx,
		"postgres:17",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(DBPassword),
		postgres.WithSQLDriver("pq"),
		postgres.BasicWaitStrategies(),
	)

	defer func() {
		if err = ctr.Terminate(ctx); err != nil {
			log.Printf("failed to clode test Container: %v", err)
		}
	}()
	require.NoError(t, err)
	dbURL, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer func() {
		if err = db.Close(); err != nil {
			log.Printf("failed ti close db: %v\n", err)
		}
	}()

	err = goose.Up(db, "../../migrations")
	require.NoError(t, err)
	zapLogger, err := logger.NewLogger("test", "debug")
	if err != nil {
		return
	}
	impr := PostgresRepository{db, zapLogger}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.filePath // путь к вашему файлу
			file, err := os.Open(filePath)
			require.NoError(t, err)

			// 2. Обязательно закрываем файл после завершения работы метода
			defer file.Close()

			createdID, err := impr.RunImportCSV(ctx, file)
			require.NoError(t, err)

			// if createdID == uuid.Nil {
			// 	t.Error(t, "uuid is nil")
			// }

			// dbRule, err := rules.Get(ctx, createdID.String())
			// require.NoError(t, err)

			// assert.Equal(t, tt.input.Name, dbRule.Name)
			// assert.Equal(t, tt.input.IsActive, dbRule.IsActive)
			// assert.Equal(t, tt.input.Priority, dbRule.Priority)
			// if !maps.Equal(tt.input.AstroCondition, dbRule.AstroCondition) {
			// 	t.Errorf("maps AstroCondition are not equal: \n\tWanted: %v . But get: %v", tt.input.AstroCondition, dbRule.AstroCondition)
			// }
			// if !slices.Equal(tt.input.ProductTags, dbRule.ProductTags) {
			// 	t.Errorf("slices ProductTags are not equal: \n\tWanted: %v . But get: %v", tt.input.ProductTags, dbRule.ProductTags)
			// }

			// err = rules.Delete(ctx, createdID.String())
			// require.NoError(t, err)
		})
	}

}

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithStartupTimeout(30 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Skipf("Cannot get mapped port: %v", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		t.Skipf("Cannot get container host: %v", err)
	}
	connStr := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, mappedPort.Port())
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Skipf("Cannot open db: %v", err)
	}

	for i := 0; i < 10; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if i == 9 {
			t.Skipf("Cannot ping database after retries: %v", err)
		}
	}

	createTableSQL := `
    CREATE TABLE products (
        sku TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        price DECIMAL(10,2) NOT NULL,
        tags JSONB,
        category TEXT
    );`
	if _, err := db.Exec(createTableSQL); err != nil {
		t.Fatalf("Cannot create table: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = container.Terminate(ctx)
	}
	return db, cleanup
}

func TestImportPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in short mode")
	}
	db, cleanup := setupTestDB(t)
	defer cleanup()

	csvData := &bytes.Buffer{}
	csvData.WriteString("sku,name,description,price,tags,category\n")
	for i := 1; i <= 1000; i++ {
		line := fmt.Sprintf("p%d,Товар%d,Описание%d,%d.99,\"[\"\"tag1\"\",\"\"tag2\"\"]\",cat%d\n",
			i, i, i, (i%1000)+1, (i%5)+1)
		csvData.WriteString(line)
	}

	start := time.Now()
	result, err := RunImport(context.Background(), db, csvData)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, 1000, result.Imported)
	assert.Equal(t, 0, result.Skipped)
	assert.LessOrEqual(t, elapsed, 5*time.Second, "Import took %v, expected <5s", elapsed)
}

func TestImportSkipsInvalidRows(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in short mode")
	}
	db, cleanup := setupTestDB(t)
	defer cleanup()

	csvData := bytes.NewBufferString(`sku,name,description,price,tags,category
    p1,good1,desc1,10.5,"[""a""]",cat1
    p2,bad,desc2,-1,"[]",cat2
    p3,good2,desc3,20.0,"[]",cat3`)

	result, err := RunImport(context.Background(), db, csvData)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Imported)
	assert.Equal(t, 1, result.Skipped)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "price = \"-1\" должен быть >0")
}
