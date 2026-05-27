-- +goose Up
ALTER TABLE requests_log
    DROP CONSTRAINT IF EXISTS chk_requests_log_status;

ALTER TABLE requests_log
    ADD CONSTRAINT chk_requests_log_status
    CHECK (status IN ('pending', 'processing', 'retry', 'completed', 'failed'));

-- +goose Down
ALTER TABLE requests_log
    DROP CONSTRAINT IF EXISTS chk_requests_log_status;

ALTER TABLE requests_log
    ADD CONSTRAINT chk_requests_log_status
    CHECK (status IN ('pending', 'processing', 'completed', 'failed'));
