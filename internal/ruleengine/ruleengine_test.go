package ruleengine

import (
	"context"
	"database/sql"
	"log"
	"path/filepath"
	"slices"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"gotest.tools/assert"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name     string
		triggers []string
		want     []string
	}{
		{
			name:     "single trigger single tag",
			triggers: []string{"Полнолуние"},
			want:     []string{"luxury"},
		},
		{
			name:     "single trigger several tags",
			triggers: []string{"Венера в Тельце"},
			want:     []string{"romantic", "cheap", "luxury"},
		},
		{
			name:     "several triggers single tag",
			triggers: []string{"Новолуние", "Полнолуние"},
			want:     []string{"luxury"},
		},
		{
			name:     "repeat triggers",
			triggers: []string{"Новолуние", "Новолуние"},
			want:     []string{"luxury"},
		},
		{
			name:     "unknown trigger",
			triggers: []string{"Что-то неизвестное"},
			want:     []string{},
		},
		{
			name:     "many triggers",
			triggers: []string{"Полнолуние", "Новолуние", "Венера в Тельце", "Что-то неизвестное", "Меркурий в Т"},
			want:     []string{"mem1", "romantic", "cheap", "luxury", "mem_prior10"},
		},
		{
			name:     "not active triggers",
			triggers: []string{"Марс в Стрельце"},
			want:     []string{},
		},
	}

	// db, err := newTestDB(t)
	ctx := context.Background()

	dbName := "ab_test"
	dbUser := "user"
	DBPassword := "password"

	ctr, err := postgres.Run(
		ctx,
		"postgres:17-alpine",
		postgres.WithInitScripts(filepath.Join("testdata", "setup.sql")),
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(DBPassword),
		postgres.WithSQLDriver("pq"),
		postgres.BasicWaitStrategies(),
	)
	defer func() {
		if err = ctr.Terminate(ctx); err != nil {
			log.Printf("failed to close test container: %v", err)
		}
	}()
	require.NoError(t, err)

	dbURL, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer func() {
		if err = db.Close(); err != nil {
			log.Printf("failed to close db: %v", err)
		}
	}()
	r := PostgresRepository{db}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			list, err := r.Match(ctx, tt.triggers)
			assert.NilError(t, err)
			if !slices.Equal(list, tt.want) {
				t.Errorf("slices are not equal: \n\tWanted: %v . But get: %v", tt.want, list)
			}
		})
	}

}
