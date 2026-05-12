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

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, req Request) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create request tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const requestQuery = `
		INSERT INTO requests_log (request_id, user_id, scenario, status, attempt_count)
		VALUES ($1, $2, $3, $4, $5)
	`
	if _, err = tx.ExecContext(ctx, requestQuery,
		req.RequestID, req.UserID, req.Scenario, req.Status, req.AttemptCount,
	); err != nil {
		return fmt.Errorf("insert requests_log: %w", err)
	}

	const resultQuery = `
		INSERT INTO generation_results (request_id, status)
		VALUES ($1, $2)
	`
	if _, err = tx.ExecContext(ctx, resultQuery, req.RequestID, StatusPending); err != nil {
		return fmt.Errorf("insert generation_results: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit create request tx: %w", err)
	}
	committed = true
	return nil
}

func (r *PostgresRepository) StartProcessing(ctx context.Context, requestID string) (bool, error) {
	const query = `
		UPDATE generation_results
		SET status     = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE request_id = $1 AND status = $3
	`
	res, err := r.db.ExecContext(ctx, query, requestID, StatusProcessing, StatusPending)
	if err != nil {
		return false, fmt.Errorf("start processing generation_results: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return rows > 0, nil
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, requestID, status string, result []byte, errReason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update request tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var resultArg any
	if len(result) > 0 {
		resultArg = string(result)
	}

	const generationQuery = `
		UPDATE generation_results
		SET status         = $2,
		    result_payload = COALESCE($3::jsonb, result_payload),
		    error_reason   = NULLIF($4, ''),
		    updated_at     = CURRENT_TIMESTAMP
		WHERE request_id = $1
	`
	res, err := tx.ExecContext(ctx, generationQuery, requestID, status, resultArg, errReason)
	if err != nil {
		return fmt.Errorf("update generation_results: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected generation_results: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	const requestQuery = `
		UPDATE requests_log
		SET status         = $2,
		    result_payload = COALESCE($3::jsonb, result_payload),
		    error_reason   = NULLIF($4, ''),
		    attempt_count  = attempt_count + 1,
		    updated_at     = CURRENT_TIMESTAMP
		WHERE request_id = $1
	`
	res, err = tx.ExecContext(ctx, requestQuery, requestID, status, resultArg, errReason)
	if err != nil {
		return fmt.Errorf("update requests_log: %w", err)
	}
	rows, err = res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected requests_log: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit update request tx: %w", err)
	}
	committed = true
	return nil
}

func (r *PostgresRepository) Get(ctx context.Context, requestID string) (Request, error) {
	const query = `
		SELECT rl.request_id, rl.user_id, rl.scenario, gr.status, rl.attempt_count,
		       COALESCE(gr.error_reason, ''), COALESCE(gr.result_payload::text, '')
		FROM generation_results gr
		JOIN requests_log rl ON rl.request_id = gr.request_id
		WHERE gr.request_id = $1
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
		return Request{}, fmt.Errorf("scan generation_results: %w", err)
	}
	if resultText != "" {
		req.Result = []byte(resultText)
	}
	return req, nil
}
