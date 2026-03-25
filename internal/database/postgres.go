package database

import (
    "database/sql"
    "fmt"
    "log"

    _ "github.com/lib/pq"
    "astroapi/config"
)

type PostgresDB struct {
    DB *sql.DB
}

var DB *PostgresDB


func InitDB(cfg *config.Config) error {
    // Формируем строку подключения
    connStr := fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
        cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
    )

    // Подключаемся к базе данных
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        return fmt.Errorf("failed to open database connection: %w", err)
    }

    // Проверяем подключение
    if err := db.Ping(); err != nil {
        return fmt.Errorf("failed to ping database: %w", err)
    }

    DB = &PostgresDB{DB: db}
    log.Printf("Successfully connected to PostgreSQL database '%s' on %s:%s",
        cfg.DBName, cfg.DBHost, cfg.DBPort)

    return nil
}

// Close закрывает подключение к базе данных
func (p *PostgresDB) Close() error {
    if p.DB != nil {
        return p.DB.Close()
    }
    return nil
}

// Ping проверяет доступность базы данных
func (p *PostgresDB) Ping() error {
    if p.DB == nil {
        return fmt.Errorf("database connection is nil")
    }
    return p.DB.Ping()
}

