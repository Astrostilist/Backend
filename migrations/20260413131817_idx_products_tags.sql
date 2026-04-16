-- +goose Up
CREATE INDEX IF NOT EXISTS idx_products_tags
    ON products
    USING GIN (tags jsonb_path_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_products_tags;
