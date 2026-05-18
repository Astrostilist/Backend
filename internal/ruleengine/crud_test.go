//go:build integration

package ruleengine

import (
	"astroapi/internal/logger"
	"context"
	"database/sql"
	"log"
	"maps"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"gotest.tools/assert"
)

func TestCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := []struct {
		name      string
		input     RuleInput
		want_uuid bool
		// want      Rule
	}{
		{
			name: "positive - create & delete a rule",
			input: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "sunrise",
					"sign": "aries",
				},
				ProductTags: []string{"romantic", "premium"},
				Priority:    5,
				IsActive:    true,
			},
			want_uuid: true,
		},
		{
			name: "positive -  create & delete a rule",
			input: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "sunrise",
					"sign": "Aries",
					"moon": "full",
				},
				ProductTags: []string{"romantic", "premium"},
				Priority:    2,
				IsActive:    true,
			},
			want_uuid: true,
		},
	}

	ctx := context.Background()

	dbName := "crud_test"
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
	rules := PostgresRepository{db, zapLogger}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdID, err := rules.Create(ctx, &tt.input)
			require.NoError(t, err)

			if createdID == uuid.Nil {
				t.Error(t, "uuid is nil")
			}

			dbRule, err := rules.Get(ctx, createdID.String())
			require.NoError(t, err)

			assert.Equal(t, tt.input.Name, dbRule.Name)
			assert.Equal(t, tt.input.IsActive, dbRule.IsActive)
			assert.Equal(t, tt.input.Priority, dbRule.Priority)
			if !maps.Equal(tt.input.AstroCondition, dbRule.AstroCondition) {
				t.Errorf("maps AstroCondition are not equal: \n\tWanted: %v . But get: %v", tt.input.AstroCondition, dbRule.AstroCondition)
			}
			if !slices.Equal(tt.input.ProductTags, dbRule.ProductTags) {
				t.Errorf("slices ProductTags are not equal: \n\tWanted: %v . But get: %v", tt.input.ProductTags, dbRule.ProductTags)
			}

			err = rules.Delete(ctx, createdID.String())
			require.NoError(t, err)
		})
	}

}

func TestUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := []struct {
		name       string
		input      RuleInput
		wantUpdate RuleInput
	}{
		{
			name: "positive - create & update a rule",
			input: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "sunrise",
					"sign": "aries",
				},
				ProductTags: []string{"romantic", "premium"},
				Priority:    5,
				IsActive:    true,
			},
			wantUpdate: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "sunrise",
					"sign": "aries",
				},
				ProductTags: []string{"luxery"},
				Priority:    15,
				IsActive:    true,
			},
		},
		{
			name: "positive -  create & update name a rule",
			input: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "sunrise",
					"sign": "aries",
					"moon": "full",
				},
				ProductTags: []string{"romantic", "premium"},
				Priority:    2,
				IsActive:    true,
			},
			wantUpdate: RuleInput{
				Name: "Марс в Близницах",
				AstroCondition: map[string]string{
					"mars": "test",
					"sign": "gemini",
				},
				ProductTags: []string{"luxery"},
				Priority:    15,
				IsActive:    true,
			},
		},
	}

	ctx := context.Background()

	dbName := "update_test"
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
	rules := PostgresRepository{db, zapLogger}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdID, err := rules.Create(ctx, &tt.input)
			require.NoError(t, err)

			if createdID == uuid.Nil {
				t.Error(t, "uuid is nil")
			}

			_, err = rules.Update(ctx, createdID.String(), &tt.wantUpdate)
			require.NoError(t, err)

			dbRule, err := rules.Get(ctx, createdID.String())
			require.NoError(t, err)

			assert.Equal(t, tt.wantUpdate.Name, dbRule.Name)
			assert.Equal(t, tt.wantUpdate.IsActive, dbRule.IsActive)
			assert.Equal(t, tt.wantUpdate.Priority, dbRule.Priority)
			if !maps.Equal(tt.wantUpdate.AstroCondition, dbRule.AstroCondition) {
				t.Errorf("maps AstroCondition are not equal: \n\tWanted: %v . But get: %v", tt.input.AstroCondition, dbRule.AstroCondition)
			}
			if !slices.Equal(tt.wantUpdate.ProductTags, dbRule.ProductTags) {
				t.Errorf("slices ProductTags are not equal: \n\tWanted: %v . But get: %v", tt.input.ProductTags, dbRule.ProductTags)
			}

			err = rules.Delete(ctx, createdID.String())
			require.NoError(t, err)

		})
	}

}

func TestList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := []struct {
		name         string
		input        []RuleInput
		opt          ListOptions
		wantList     []RuleInput
		wantMetadata Metadata
	}{
		{
			name: "positive - get list of rules with pagination",
			input: []RuleInput{
				{
					Name: "а Меркурий в Стрельце",
					AstroCondition: map[string]string{
						"sun":  "sumrise",
						"sign": "Aries",
					},
					ProductTags: []string{"romantic", "premium"},
					Priority:    1,
					IsActive:    true,
				},
				{
					Name: "б Венера в Стрельце",
					AstroCondition: map[string]string{
						"venus": "test",
						"sign":  "aries",
					},
					ProductTags: []string{"premium"},
					Priority:    3,
					IsActive:    true,
				},
				{
					Name: "в Юпитер в Стрельце",
					AstroCondition: map[string]string{
						"jupiter": "test",
						"sign":    "aries",
					},
					ProductTags: []string{"premium"},
					Priority:    3,
					IsActive:    true,
				},
				{
					Name: "г Юпитер в Стрельце",
					AstroCondition: map[string]string{
						"saturn": "test",
						"sign":   "aries",
					},
					ProductTags: []string{"agree"},
					Priority:    5,
					IsActive:    true,
				},
				{
					Name: "д Уран в Стрельце",
					AstroCondition: map[string]string{
						"uranus": "test",
						"sign":   "aries",
					},
					ProductTags: []string{"agree"},
					Priority:    6,
					IsActive:    true,
				},
				{
					Name: "е Нептун в Стрельце",
					AstroCondition: map[string]string{
						"neptune": "test",
						"sign":    "aries",
					},
					ProductTags: []string{"agree"},
					Priority:    7,
					IsActive:    true,
				},
				{
					Name: "ж Плутон в Стрельце",
					AstroCondition: map[string]string{
						"pluto": "test",
						"sign":  "aries",
					},
					ProductTags: []string{"agree"},
					Priority:    10,
					IsActive:    true,
				},
				{
					Name: "з Солнце в Стрельце",
					AstroCondition: map[string]string{
						"sun":  "test",
						"sign": "aries",
					},
					ProductTags: []string{"agree"},
					Priority:    10,
					IsActive:    false,
				},
			},
			opt: ListOptions{
				IsActive: func(b bool) *bool { return &b }(true),
				PageSize: 10,
				Page:     1,
			},
			wantList: []RuleInput{
				{
					Name: "в Юпитер в Стрельце",
					AstroCondition: map[string]string{
						"jupiter": "test",
						"sign":    "aries",
					},
					ProductTags: []string{"premium"},
					Priority:    3,
					IsActive:    true,
				},
				{
					Name: "г Юпитер в Стрельце",
					AstroCondition: map[string]string{
						"saturn": "test",
						"sign":   "aries",
					},
					ProductTags: []string{"agree"},
					Priority:    5,
					IsActive:    true,
				},
			},
			wantMetadata: Metadata{
				CurrentPage:  2,
				PageSize:     2,
				FirstPage:    1,
				LastPage:     4,
				TotalRecords: 7,
			},
		},
	}

	ctx := context.Background()

	dbName := "list_test"
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
	rules := PostgresRepository{db, zapLogger}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var listID []uuid.UUID
			for _, elem := range tt.input {
				createdID, err := rules.Create(ctx, &elem)
				require.NoError(t, err)
				listID = append(listID, createdID)
			}

			dbRules, mtdata, err := rules.List(ctx, tt.opt)
			require.NoError(t, err)

			for k, item := range dbRules {
				assert.Equal(t, tt.wantList[k].Name, item.Name)
				assert.Equal(t, tt.wantList[k].IsActive, item.IsActive)
				assert.Equal(t, tt.wantList[k].Priority, item.Priority)
				if !maps.Equal(tt.wantList[k].AstroCondition, item.AstroCondition) {
					t.Errorf("maps AstroCondition are not equal: \n\tWanted: %v . But get: %v", tt.wantList[k].AstroCondition, item.AstroCondition)
				}
				if !slices.Equal(tt.wantList[k].ProductTags, item.ProductTags) {
					t.Errorf("slices ProductTags are not equal: \n\tWanted: %v . But get: %v", tt.wantList[k].ProductTags, item.ProductTags)
				}
			}

			assert.Equal(t, tt.wantMetadata, mtdata)

		})
	}

}

func TestDeactivate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := []struct {
		name       string
		input      RuleInput
		wantUpdate RuleInput
	}{
		{
			name: "positive - deactivate a rule",
			input: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "test",
					"sign": "aries",
				},
				ProductTags: []string{"romantic", "premium"},
				Priority:    5,
				IsActive:    true,
			},
			wantUpdate: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "test",
					"sign": "aries",
				},
				ProductTags: []string{"romantic", "premium"},
				Priority:    5,
				IsActive:    false,
			},
		},
	}

	ctx := context.Background()

	dbName := "update_test"
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
	rules := PostgresRepository{db, zapLogger}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdID, err := rules.Create(ctx, &tt.input)
			require.NoError(t, err)

			if createdID == uuid.Nil {
				t.Error(t, "uuid is nil")
			}

			_, err = rules.Deactivate(ctx, createdID.String())
			require.NoError(t, err)

			dbRule, err := rules.Get(ctx, createdID.String())
			require.NoError(t, err)

			assert.Equal(t, tt.wantUpdate.Name, dbRule.Name)
			assert.Equal(t, false, dbRule.IsActive)
			assert.Equal(t, tt.wantUpdate.Priority, dbRule.Priority)
			if !maps.Equal(tt.wantUpdate.AstroCondition, dbRule.AstroCondition) {
				t.Errorf("maps AstroCondition are not equal: \n\tWanted: %v . But get: %v", tt.input.AstroCondition, dbRule.AstroCondition)
			}
			if !slices.Equal(tt.wantUpdate.ProductTags, dbRule.ProductTags) {
				t.Errorf("slices ProductTags are not equal: \n\tWanted: %v . But get: %v", tt.input.ProductTags, dbRule.ProductTags)
			}

			err = rules.Delete(ctx, createdID.String())
			require.NoError(t, err)

		})
	}

}

func TestDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := []struct {
		name      string
		input     RuleInput
		want_uuid bool
		// want      Rule
	}{
		{
			name: "positive - create & delete a rule",
			input: RuleInput{
				Name: "Меркурий в Стрельце",
				AstroCondition: map[string]string{
					"sun":  "test",
					"sign": "aries",
				},
				ProductTags: []string{"romantic", "premium"},
				Priority:    5,
				IsActive:    true,
			},
			want_uuid: true,
		},
	}

	ctx := context.Background()

	dbName := "delete_test"
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
	rules := PostgresRepository{db, zapLogger}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdID, err := rules.Create(ctx, &tt.input)
			require.NoError(t, err)

			if createdID == uuid.Nil {
				t.Error(t, "uuid is nil")
			}

			dbRule, err := rules.Get(ctx, createdID.String())
			require.NoError(t, err)

			assert.Equal(t, tt.input.Name, dbRule.Name)
			assert.Equal(t, tt.input.IsActive, dbRule.IsActive)
			assert.Equal(t, tt.input.Priority, dbRule.Priority)
			if !maps.Equal(tt.input.AstroCondition, dbRule.AstroCondition) {
				t.Errorf("maps AstroCondition are not equal: \n\tWanted: %v . But get: %v", tt.input.AstroCondition, dbRule.AstroCondition)
			}
			if !slices.Equal(tt.input.ProductTags, dbRule.ProductTags) {
				t.Errorf("slices ProductTags are not equal: \n\tWanted: %v . But get: %v", tt.input.ProductTags, dbRule.ProductTags)
			}

			err = rules.Delete(ctx, createdID.String())
			require.NoError(t, err)

			err = rules.Delete(ctx, createdID.String())
			require.Error(t, err)
		})
	}

}

func TestCreateAndMatch(t *testing.T) {
	tests := []struct {
		name          string
		input         []RuleInput
		inputTriggers []string
		want          []string
	}{
		{
			name: "positive - create & match && delete a rule",
			input: []RuleInput{
				{
					Name: "Меркурий в Стрельце",
					AstroCondition: map[string]string{
						"sun":  "test",
						"sign": "aries",
					},
					ProductTags: []string{"romantic", "premium"},
					Priority:    5,
					IsActive:    true,
				},
				{
					Name: "Меркурий в Водолее",
					AstroCondition: map[string]string{
						"moon": "test",
						"sign": "gemini",
					},
					ProductTags: []string{"active", "premium"},
					Priority:    2,
					IsActive:    true,
				},
				{
					Name: "Скорпион в Водолее",
					AstroCondition: map[string]string{
						"sun":  "test",
						"sign": "gemini",
					},
					ProductTags: []string{"red"},
					Priority:    1,
					IsActive:    true,
				},
			},
			inputTriggers: []string{"Меркурий в Стрельце", "Меркурий в Водолее", "Скорпион в Водолее"},
			want:          []string{"red", "active", "premium", "romantic"},
		},
		{
			name:          "positive - triggers not exist",
			input:         []RuleInput{},
			inputTriggers: []string{"Меркурий в Стрельце"},
			want:          []string{},
		},
		{
			name: "positive - create & match && delete a rule",
			input: []RuleInput{
				{
					Name: "Меркурий в Стрельце",
					AstroCondition: map[string]string{
						"sun":  "test",
						"sign": "Aries",
					},
					ProductTags: []string{"lux"},
					Priority:    5,
					IsActive:    true,
				},
			},
			inputTriggers: []string{"Меркурий в Стрельце", "Меркурий в Стрельце"},
			want:          []string{"lux"},
		},
	}

	ctx := context.Background()

	dbName := "crud_test"
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
	rules := PostgresRepository{db, zapLogger}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ids []uuid.UUID
			for _, item := range tt.input {
				createdID, err := rules.Create(ctx, &item)
				require.NoError(t, err)

				if createdID == uuid.Nil {
					t.Error(t, "uuid is nil")
				}
				ids = append(ids, createdID)

			}
			list, err := rules.Match(ctx, tt.inputTriggers)
			assert.NilError(t, err)

			if !slices.Equal(tt.want, list) {
				t.Errorf("slices  are not equal: \n\tWanted: %v . But get: %v", tt.want, list)
			}

			if len(list) > 0 {
				for _, item := range ids {
					err = rules.Delete(ctx, item.String())
					require.NoError(t, err)
				}
			}

		})
	}
}
