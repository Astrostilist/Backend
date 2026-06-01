package importer

import (
	"astroapi/internal/infrastructure/logger"
	"context"
	"database/sql"
	"log"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestRunImportCSV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := []struct {
		name             string
		filePath         string
		want             bool
		wantCountGoodStr int
		wantCountBadStr  int
	}{
		{
			name:             "read csv",
			filePath:         "testdata/file1.csv",
			want:             true,
			wantCountGoodStr: 3,
			wantCountBadStr:  1,
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
			filePath := tt.filePath
			file, err := os.Open(filePath)
			require.NoError(t, err)
			defer file.Close()

			ctx := context.Background()

			res, err := impr.RunImportCSV(ctx, file)
			//require.NoError(t, err)

			// Проверяем результаты
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCountGoodStr, res.Imported, "Должно быть 3 хорошие строки") // TODO 3 заполнить
			assert.Equal(t, tt.wantCountBadStr, res.Skipped, "Должна быть 1 плохая строка")    // TODO 1 заполнить

			for _, item := range res.Errors {
				assert.Equal(t, item.Row, 4)
			}
		})
	}

}
