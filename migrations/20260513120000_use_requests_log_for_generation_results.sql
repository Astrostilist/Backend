-- +goose Up
ALTER TABLE requests_log
    DROP CONSTRAINT IF EXISTS chk_requests_log_status;

ALTER TABLE requests_log
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

DO $$
BEGIN
    IF to_regclass('public.generation_results') IS NOT NULL THEN
        UPDATE requests_log AS rl
        SET status = gr.status,
            result_payload = COALESCE(gr.result_payload, rl.result_payload),
            error_reason = COALESCE(gr.error_reason, rl.error_reason),
            updated_at = GREATEST(rl.updated_at, gr.updated_at),
            completed_at = CASE
                WHEN gr.status IN ('completed', 'failed')
                    THEN COALESCE(rl.completed_at, gr.updated_at, CURRENT_TIMESTAMP)
                ELSE rl.completed_at
            END
        FROM generation_results AS gr
        WHERE rl.request_id = gr.request_id;
    END IF;
END $$;

ALTER TABLE requests_log
    ADD CONSTRAINT chk_requests_log_status
    CHECK (status IN ('pending', 'processing', 'completed', 'failed'));

DROP TABLE IF EXISTS generation_results;

-- +goose Down
CREATE TABLE IF NOT EXISTS generation_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL UNIQUE REFERENCES requests_log(request_id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    result_payload JSONB,
    error_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_generation_results_status
    ON generation_results (status, created_at DESC);

INSERT INTO generation_results (request_id, status, result_payload, error_reason, created_at, updated_at)
SELECT request_id, status, result_payload, error_reason, created_at, updated_at
FROM requests_log
ON CONFLICT (request_id) DO UPDATE SET
    status = EXCLUDED.status,
    result_payload = EXCLUDED.result_payload,
    error_reason = EXCLUDED.error_reason,
    updated_at = EXCLUDED.updated_at;

ALTER TABLE requests_log
    DROP CONSTRAINT IF EXISTS chk_requests_log_status;

ALTER TABLE requests_log
    ADD CONSTRAINT chk_requests_log_status
    CHECK (status IN ('pending', 'processing', 'completed', 'failed'));

ALTER TABLE requests_log
    DROP COLUMN IF EXISTS completed_at;
