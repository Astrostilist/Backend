package products

import (
    "context"
    "database/sql"
    "testing"

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
        WaitingFor: wait.ForLog("database system is ready to accept connections"),
    }
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        t.Skip("Docker not available, skipping test")
    }
    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "5432")
    connStr := "postgres://test:test@" + host + ":" + port.Port() + "/testdb?sslmode=disable"
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        t.Fatal(err)
    }
    _, err = db.Exec(`
        CREATE TABLE products (
            sku TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            description TEXT,
            price DECIMAL(10,2) NOT NULL,
            tags JSONB,
            category TEXT,
            rating DECIMAL(3,2) DEFAULT 0
        );
        CREATE INDEX idx_products_tags ON products USING GIN (tags);
    `)
    if err != nil {
        t.Fatal(err)
    }
    return db, func() { db.Close(); container.Terminate(ctx) }
}

func TestFindByTagsOrder(t *testing.T) {
    db, cleanup := setupTestDB(t)
    defer cleanup()

    insertSQL := `
        INSERT INTO products (sku, name, description, price, tags, category, rating) VALUES
        ('p1', 'A', '', 10, '["a","b","c"]', 'cat1', 5.0),
        ('p2', 'B', '', 20, '["a","b"]', 'cat1', 4.5),
        ('p3', 'C', '', 30, '["a"]', 'cat1', 5.0),
        ('p4', 'D', '', 40, '["x","y"]', 'cat1', 3.0);
    `
    _, err := db.Exec(insertSQL)
    assert.NoError(t, err)

    products, err := FindByTags(context.Background(), db, []string{"a", "b"}, 10, 0)
    assert.NoError(t, err)
    assert.Len(t, products, 3) // p1, p2, p3
    expected := []string{"p1", "p2", "p3"}
    for i, p := range products {
        assert.Equal(t, expected[i], p.SKU)
    }
}