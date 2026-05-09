-- +goose Up
CREATE TABLE IF NOT EXISTS generation_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL UNIQUE REFERENCES requests_log(request_id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    result_payload JSONB,
    error_reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_generation_results_status
    ON generation_results (status, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_generation_results_status;
DROP TABLE IF EXISTS generation_results;
