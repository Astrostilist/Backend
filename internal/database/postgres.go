package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"astroapi/config"

	_ "github.com/lib/pq"
	"github.com/pressly/goose"
)

const migrationsDir = "migrations"

// PostgresDB - тонкая обёртка над *sql.DB, создаётся через New.
// Глобальной переменной DB больше нет — всё явно прокидывается через зависимости.
type PostgresDB struct {
	DB *sql.DB
}

// New открывает соединение, делает ping и прогоняет goose-миграции.
func New(ctx context.Context, cfg *config.Config) (*PostgresDB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(int(cfg.DBMaxConns))
	db.SetMaxIdleConns(int(cfg.DBMinConns))
	db.SetConnMaxLifetime(cfg.DBMaxConnLifetime)
	db.SetConnMaxIdleTime(cfg.DBMaxConnIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return &PostgresDB{DB: db}, nil
}

// Close закрывает соединение.
func (p *PostgresDB) Close() error {
	if p == nil || p.DB == nil {
		return nil
	}
	return p.DB.Close()
}

// Ping проверяет доступность базы.
func (p *PostgresDB) Ping(ctx context.Context) error {
	if p == nil || p.DB == nil {
		return fmt.Errorf("database connection is nil")
	}
	return p.DB.PingContext(ctx)
}
