-- +goose Up
CREATE INDEX IF NOT EXISTS idx_requests_status
    ON requests_log (status, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_requests_status;
