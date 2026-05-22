package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
)

const processingStaleInterval = "2 minutes"

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
	tracer := otel.Tracer("db-repo")
	repoctx, repoSpan := tracer.Start(ctx, "request.Create")
	defer repoSpan.End()

	const query = `
		INSERT INTO requests_log (request_id, user_id, scenario, status, attempt_count)
		VALUES ($1, $2, $3, $4, $5)
	`
	if _, err := r.db.ExecContext(repoctx, query,
		req.RequestID, req.UserID, req.Scenario, req.Status, req.AttemptCount,
	); err != nil {
		repoSpan.RecordError(err)
		return fmt.Errorf("insert requests_log: %w", err)
	}
	return nil
}

// StartProcessing атомарно переводит запрос в processing, если нет готового результата.
// Повторная доставка может продолжить pending/retry; processing разрешён только
// после stale-интервала, чтобы не запустить параллельный дубль обработки. Старые
// failed записи восстанавливаются до достижения лимита попыток.
func (r *PostgresRepository) StartProcessing(ctx context.Context, requestID string) (bool, error) {
	tracer := otel.Tracer("db-repo")
	repoctx, repoSpan := tracer.Start(ctx, "request.StartProcessing")
	defer repoSpan.End()

	const query = `
		UPDATE requests_log
		SET status = $2,
		    updated_at = CURRENT_TIMESTAMP,
		    completed_at = NULL
		WHERE request_id = $1
		  AND result_payload IS NULL
		  AND (
			status IN ($3, $4)
			OR (
				status = $5
				AND updated_at < CURRENT_TIMESTAMP - ($6::text)::interval
			)
			OR (status = $7 AND attempt_count < $8)
		  )
	`
	res, err := r.db.ExecContext(ctx, query,
		requestID,
		StatusProcessing,
		StatusPending,
		StatusRetry,
		StatusProcessing,
		processingStaleInterval,
		StatusFailed,
		MaxProcessingAttempts,
	)
	if err != nil {
		repoSpan.RecordError(err)
		return false, fmt.Errorf("start processing requests_log: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		repoSpan.RecordError(err)
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return rows > 0, nil
}

// UpdateStatus обновляет статус, результат и ошибку запроса в requests_log.
// На вход принимает контекст, request_id, новый статус, JSON-результат и текст ошибки; на выход возвращает ошибку обновления.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, requestID, status string, result []byte, errReason string) error {
	tracer := otel.Tracer("db-repo")
	repoctx, repoSpan := tracer.Start(ctx, "request.UpdateStatus")
	defer repoSpan.End()

	var resultArg any
	if len(result) > 0 {
		resultArg = string(result)
	}

	const query = `
		UPDATE requests_log
		SET status = CAST($2 AS VARCHAR),
			result_payload = COALESCE($3::jsonb, result_payload),
			error_reason = NULLIF($4, ''),
			attempt_count = attempt_count + 1,
			updated_at = CURRENT_TIMESTAMP,
			completed_at = CASE
				WHEN CAST($2 AS VARCHAR) IN ('completed', 'failed') THEN CURRENT_TIMESTAMP
				ELSE NULL
			END
		WHERE request_id = $1
	`
	res, err := r.db.ExecContext(repoctx, query, requestID, status, resultArg, errReason)
	if err != nil {
		repoSpan.RecordError(err)
		return fmt.Errorf("update requests_log: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		repoSpan.RecordError(err)
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
	tracer := otel.Tracer("db-repo")
	repoctx, repoSpan := tracer.Start(ctx, "request.Get")
	defer repoSpan.End()

	const query = `
		SELECT request_id, user_id, scenario, status, attempt_count,
		       COALESCE(error_reason, ''), COALESCE(result_payload::text, '')
		FROM requests_log
		WHERE request_id = $1::uuid
	`
	row := r.db.QueryRowContext(repoctx, query, requestID)

	var (
		req        Request
		resultText string
	)
	if err := row.Scan(&req.RequestID, &req.UserID, &req.Scenario, &req.Status,
		&req.AttemptCount, &req.ErrorReason, &resultText); err != nil {
		repoSpan.RecordError(err)
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
