-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS feedback (
                                        id UUID PRIMARY KEY,
                                        request_id UUID NOT NULL UNIQUE REFERENCES recommendations(id) ON DELETE CASCADE,
                                        rating INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
                                        comment VARCHAR(500),
                                        created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS feedback;
-- +goose StatementEnd