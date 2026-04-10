package importer

import (
    "bytes"
    "context"
    "database/sql"
    "fmt"
    "testing"
    "time"

    _ "github.com/lib/pq"
    "github.com/stretchr/testify/assert"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

func TestImportPerformance(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping performance test in short mode")
    }

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
    postgresContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        t.Skip("Docker not available, skipping test")
    }
    defer func() { _ = postgresContainer.Terminate(ctx) }()

    mappedPort, err := postgresContainer.MappedPort(ctx, "5432")
    if err != nil {
        t.Fatal(err)
    }
    connStr := fmt.Sprintf("postgres://test:test@127.0.0.1:%s/testdb?sslmode=disable", mappedPort.Port())
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        t.Fatal(err)
    }
    defer func() { _ = db.Close() }()

    if err := db.Ping(); err != nil {
        t.Fatal("Cannot connect to database:", err)
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
        t.Fatal(err)
    }

    csvData := &bytes.Buffer{}
    csvData.WriteString("sku,name,description,price,tags,category\n")
    for i := 1; i <= 1000; i++ {
        line := fmt.Sprintf("p%d,Товар%d,Описание%d,%d.99,\"[\"\"tag1\"\",\"\"tag2\"\"]\",cat%d\n",
            i, i, i, (i%1000)+1, (i%5)+1)
        csvData.WriteString(line)
    }

    start := time.Now()
    result, err := RunImport(ctx, db, csvData)
    elapsed := time.Since(start)

    assert.NoError(t, err)
    assert.Equal(t, 1000, result.Imported)
    assert.Equal(t, 0, result.Skipped)
    assert.LessOrEqual(t, elapsed, 5*time.Second, "Import took %v, expected <5s", elapsed)
}

func TestImportSkipsInvalidRows(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping test in short mode")
    }

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
    postgresContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        t.Skip("Docker not available, skipping test")
    }
    defer func() { _ = postgresContainer.Terminate(ctx) }()

    mappedPort, err := postgresContainer.MappedPort(ctx, "5432")
    if err != nil {
        t.Fatal(err)
    }
    connStr := fmt.Sprintf("postgres://test:test@127.0.0.1:%s/testdb?sslmode=disable", mappedPort.Port())
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        t.Fatal(err)
    }
    defer func() { _ = db.Close() }()

    if err := db.Ping(); err != nil {
        t.Fatal("Cannot connect to database:", err)
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
        t.Fatal(err)
    }

    csvData := bytes.NewBufferString(`sku,name,description,price,tags,category
    p1,good1,desc1,10.5,"[""a""]",cat1
    p2,bad,desc2,-1,"[]",cat2
    p3,good2,desc3,20.0,"[]",cat3`)

    result, err := RunImport(ctx, db, csvData)
    assert.NoError(t, err)
    assert.Equal(t, 2, result.Imported)
    assert.Equal(t, 1, result.Skipped)
    assert.Len(t, result.Errors, 1)
    assert.Contains(t, result.Errors[0], "price = \"-1\" должен быть >0")
}