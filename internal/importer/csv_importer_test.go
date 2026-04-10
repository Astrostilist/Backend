package importer

import (
    "bytes"
    "context"
    "database/sql"
    "fmt"
    "os"
    "testing"
    "time"

    _ "github.com/lib/pq"
    "github.com/joho/godotenv"
    "github.com/stretchr/testify/assert"
)

func init() {
    _ = godotenv.Load("../.env") 
}

func getTestDBParams() (host, port, user, password string) {
    host = os.Getenv("DB_HOST")
    if host == "" {
        host = "localhost"
    }
    port = os.Getenv("DB_PORT")
    if port == "" {
        port = "5432"
    }
    user = os.Getenv("DB_USER")
    if user == "" {
        user = "postgres"
    }
    password = os.Getenv("DB_PASSWORD")
    return
}

func getTestDB(t *testing.T) *sql.DB {
    host, port, user, password := getTestDBParams()
    adminConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
        host, port, user, password)
    adminDB, err := sql.Open("postgres", adminConnStr)
    if err != nil {
        t.Fatalf("Cannot connect to admin db: %v", err)
    }
    defer adminDB.Close()

    _, _ = adminDB.Exec("DROP DATABASE IF EXISTS testdb")
    _, err = adminDB.Exec("CREATE DATABASE testdb")
    if err != nil {
        t.Fatalf("Cannot create testdb: %v", err)
    }

    testConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=testdb sslmode=disable",
        host, port, user, password)
    testDB, err := sql.Open("postgres", testConnStr)
    if err != nil {
        t.Fatalf("Cannot connect to testdb: %v", err)
    }

    _, _ = testDB.Exec("DROP TABLE IF EXISTS products")
    createTableSQL := `
    CREATE TABLE products (
        sku TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        price DECIMAL(10,2) NOT NULL,
        tags JSONB,
        category TEXT
    );`
    if _, err := testDB.Exec(createTableSQL); err != nil {
        t.Fatalf("Cannot create table: %v", err)
    }
    return testDB
}

func cleanupTestDB(t *testing.T, db *sql.DB) {
    db.Close()
    host, port, user, password := getTestDBParams()
    adminConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
        host, port, user, password)
    adminDB, err := sql.Open("postgres", adminConnStr)
    if err != nil {
        t.Logf("Warning: cannot connect to admin db for cleanup: %v", err)
        return
    }
    defer adminDB.Close()
    _, err = adminDB.Exec("DROP DATABASE IF EXISTS testdb")
    if err != nil {
        t.Logf("Warning: cannot drop testdb: %v", err)
    }
}

func TestImportPerformance(t *testing.T) {
    db := getTestDB(t)
    defer cleanupTestDB(t, db)

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
    db := getTestDB(t)
    defer cleanupTestDB(t, db)

    var buf bytes.Buffer
    buf.WriteString("sku,name,description,price,tags,category\n")
    buf.WriteString("p1,good1,desc1,10.5,\"[\"\"a\"\"]\",cat1\n")
    buf.WriteString("p2,bad,desc2,-1,\"[]\",cat2\n")
    buf.WriteString("p3,good2,desc3,20.0,\"[]\",cat3") 

    result, err := RunImport(context.Background(), db, &buf)
    assert.NoError(t, err)
    assert.Equal(t, 2, result.Imported)
    assert.Equal(t, 1, result.Skipped)
    assert.Len(t, result.Errors, 1, "expected exactly one error")
    assert.Contains(t, result.Errors[0], "price = \"-1\" должен быть >0")
}