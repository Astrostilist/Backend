package products

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

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
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithStartupTimeout(60 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skip("Docker not available, skipping test")
	}
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Cannot get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Cannot get mapped port: %v", err)
	}
	connStr := "postgres://test:test@" + host + ":" + port.Port() + "/testdb?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("Cannot open db: %v", err)
	}
	// Проверка соединения с повторными попытками
	var pingErr error
	for i := 0; i < 10; i++ {
		if pingErr = db.Ping(); pingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if pingErr != nil {
		t.Fatalf("Cannot ping db after retries: %v", pingErr)
	}
	// Создаём таблицу и индекс
	_, err = db.Exec(`
        CREATE TABLE products (
            sku VARCHAR(255) PRIMARY KEY,
            ext_product_id VARCHAR(255) NOT NULL UNIQUE,
            title VARCHAR(500) NOT NULL DEFAULT '',
            article TEXT  NOT NULL DEFAULT '',
            category TEXT NOT NULL DEFAULT '',
            price NUMERIC(10,2) NOT NULL,
            url TEXT NOT NULL DEFAULT '',
            images TEXT[] NOT NULL DEFAULT '{}',
            tags JSONB NOT NULL DEFAULT '[]'::jsonb,
            rating DECIMAL(3,2) NOT NULL DEFAULT 0,
    
            created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
        );
        CREATE INDEX idx_products_tags ON products USING GIN (tags);
    `)
	if err != nil {
		t.Fatalf("Cannot create schema: %v", err)
	}
	return db, func() {
		db.Close()
		container.Terminate(ctx)
	}
}

func TestFindByTagsOrder(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertSQL := `
    
        INSERT INTO products (sku, ext_product_id, title, price, tags, category, url, rating) VALUES
        ('51ccb1ab-b14c-11ef-8e11-005056b6c1cd#f72cc217-e140-11ee-8e0e-005056b6c1cd', 'p1', 'A', 10, '["a","b","c"]', 'cat1', 'https://lina.com/hidey_1/', 5.0),
        ('c4bd8b48-02e2-11ef-8e0e-005056b6c1cd#f72cc217-e140-11ee-8e0e-005056b6c1cd', 'p2', 'B', 20, '["a","b"]', 'cat1', 'https://lina.com/hidey_2/', 4.5),
        ('176d8575-0221-11ef-8e0e-005056b6c1cd#f72cc218-e140-11ee-8e0e-005056b6c1cd', 'p3', 'C', 30, '["a"]', 'cat1', 'https://lina.com/hidey_2/', 5.0),
        ('4f3dd165-954a-11f0-a137-005056b6f117#ac39892a-5b2f-11f0-a108-005056b6f117', 'p4', 'D', 40, '["x","y"]', 'cat1', 'https://lina.com/hidey_2/', 3.0);
    `
	_, err := db.Exec(insertSQL)
	assert.NoError(t, err)

	products, err := FindByTags(context.Background(), db, []string{"a", "b"}, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, products, 3) // p1, p2, p3
	expected := []string{
		"51ccb1ab-b14c-11ef-8e11-005056b6c1cd#f72cc217-e140-11ee-8e0e-005056b6c1cd",
		"c4bd8b48-02e2-11ef-8e0e-005056b6c1cd#f72cc217-e140-11ee-8e0e-005056b6c1cd",
		"176d8575-0221-11ef-8e0e-005056b6c1cd#f72cc218-e140-11ee-8e0e-005056b6c1cd"}
	for i, p := range products {
		assert.Equal(t, expected[i], p.SKU)
	}
}
