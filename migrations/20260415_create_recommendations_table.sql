-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS recommendations (
                                               id UUID PRIMARY KEY,
                                               user_id UUID NOT NULL,
                                               scenario VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'processing',
    result_text TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
                             );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS recommendations;
-- +goose StatementEnd