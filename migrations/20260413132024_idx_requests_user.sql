-- +goose Up
CREATE INDEX IF NOT EXISTS idx_requests_user
    ON requests_log (user_id, created_at DESC);
-- +goose Down
DROP INDEX IF EXISTS idx_requests_user;
