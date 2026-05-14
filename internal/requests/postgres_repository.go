package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PostgresRepository — реализация Repository поверх PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository создает PostgreSQL-репозиторий requests_log.
// На вход принимает соединение с БД, на выход возвращает готовый репозиторий.
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Create создает новую запись запроса в requests_log.
// На вход принимает контекст и данные запроса, на выход возвращает ошибку записи.
func (r *PostgresRepository) Create(ctx context.Context, req Request) error {
	const query = `
		INSERT INTO requests_log (request_id, user_id, scenario, status, attempt_count)
		VALUES ($1, $2, $3, $4, $5)
	`
	if _, err := r.db.ExecContext(ctx, query,
		req.RequestID, req.UserID, req.Scenario, req.Status, req.AttemptCount,
	); err != nil {
		return fmt.Errorf("insert requests_log: %w", err)
	}
	return nil
}

// StartProcessing атомарно переводит запрос из pending в processing.
// На вход принимает контекст и request_id, на выход возвращает флаг успешного старта и ошибку.
func (r *PostgresRepository) StartProcessing(ctx context.Context, requestID string) (bool, error) {
	const query = `
		UPDATE requests_log
		SET status     = $2,
		    updated_at = CURRENT_TIMESTAMP,
		    completed_at = NULL
		WHERE request_id = $1 AND status = $3
	`
	res, err := r.db.ExecContext(ctx, query, requestID, StatusProcessing, StatusPending)
	if err != nil {
		return false, fmt.Errorf("start processing requests_log: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return rows > 0, nil
}

// UpdateStatus обновляет статус, результат и ошибку запроса в requests_log.
// На вход принимает контекст, request_id, новый статус, JSON-результат и текст ошибки; на выход возвращает ошибку обновления.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, requestID, status string, result []byte, errReason string) error {
	var resultArg any
	if len(result) > 0 {
		resultArg = string(result)
	}

	const query = `
		UPDATE requests_log
		SET status         = $2,
		    result_payload = COALESCE($3::jsonb, result_payload),
		    error_reason   = NULLIF($4, ''),
		    attempt_count  = attempt_count + 1,
		    updated_at     = CURRENT_TIMESTAMP,
		    completed_at   = CASE
		        WHEN $2 IN ('completed', 'failed') THEN CURRENT_TIMESTAMP
		        ELSE NULL
		    END
		WHERE request_id = $1
	`
	res, err := r.db.ExecContext(ctx, query, requestID, status, resultArg, errReason)
	if err != nil {
		return fmt.Errorf("update requests_log: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected requests_log: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// Get возвращает состояние запроса из requests_log по request_id.
// На вход принимает контекст и request_id, на выход возвращает данные запроса или ошибку.
func (r *PostgresRepository) Get(ctx context.Context, requestID string) (Request, error) {
	const query = `
		SELECT request_id, user_id, scenario, status, attempt_count,
		       COALESCE(error_reason, ''), COALESCE(result_payload::text, '')
		FROM requests_log
		WHERE request_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, requestID)

	var (
		req        Request
		resultText string
	)
	if err := row.Scan(&req.RequestID, &req.UserID, &req.Scenario, &req.Status,
		&req.AttemptCount, &req.ErrorReason, &resultText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Request{}, ErrNotFound
		}
		return Request{}, fmt.Errorf("scan requests_log: %w", err)
	}
	if resultText != "" {
		req.Result = []byte(resultText)
	}
	return req, nil
}
