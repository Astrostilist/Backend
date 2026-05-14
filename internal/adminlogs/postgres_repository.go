package adminlogs

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PostgresRepository читает административный журнал генераций из PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

// NewPostgresRepository создает PostgreSQL-репозиторий административного журнала.
// На вход принимает соединение с БД, на выход возвращает готовый репозиторий.
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// List возвращает записи requests_log по фильтрам и общий счетчик.
// На вход принимает контекст и параметры фильтрации, на выход возвращает список записей или ошибку.
func (r *PostgresRepository) List(ctx context.Context, options ListOptions) (ListResult, error) {
	const countQuery = `
		SELECT COUNT(*)
		FROM requests_log
		WHERE ($1 = '' OR status = $1)
		  AND ($2::timestamptz IS NULL OR created_at >= $2::timestamptz)
		  AND ($3::timestamptz IS NULL OR created_at <= $3::timestamptz)
	`

	fromArg := timeArg(options.From)
	toArg := timeArg(options.To)

	var totalCount int
	if err := r.db.QueryRowContext(ctx, countQuery, options.Status, fromArg, toArg).Scan(&totalCount); err != nil {
		return ListResult{}, fmt.Errorf("count requests_log: %w", err)
	}

	const listQuery = `
		SELECT request_id, user_id, status, COALESCE(error_reason, ''), created_at, completed_at
		FROM requests_log
		WHERE ($1 = '' OR status = $1)
		  AND ($2::timestamptz IS NULL OR created_at >= $2::timestamptz)
		  AND ($3::timestamptz IS NULL OR created_at <= $3::timestamptz)
		ORDER BY created_at DESC
		LIMIT $4
	`

	rows, err := r.db.QueryContext(ctx, listQuery, options.Status, fromArg, toArg, options.Limit)
	if err != nil {
		return ListResult{}, fmt.Errorf("list requests_log: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	items := make([]LogEntry, 0)
	for rows.Next() {
		item, scanErr := scanLogEntry(rows)
		if scanErr != nil {
			return ListResult{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate requests_log: %w", err)
	}

	return ListResult{Items: items, TotalCount: totalCount}, nil
}

// timeArg преобразует указатель времени в значение для SQL-параметра.
// На вход принимает указатель на время, на выход возвращает время или nil.
func timeArg(value *time.Time) any {
	if value == nil {
		return nil
	}

	return *value
}

// scanLogEntry сканирует строку БД в LogEntry.
// На вход принимает rowScanner, на выход возвращает запись журнала или ошибку.
func scanLogEntry(scanner rowScanner) (LogEntry, error) {
	var (
		item        LogEntry
		completedAt sql.NullTime
	)

	if err := scanner.Scan(
		&item.RequestID,
		&item.UserID,
		&item.Status,
		&item.ErrorMessage,
		&item.CreatedAt,
		&completedAt,
	); err != nil {
		return LogEntry{}, fmt.Errorf("scan requests_log: %w", err)
	}
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	item.ErrorMessage = TruncateErrorMessage(item.ErrorMessage)

	return item, nil
}
