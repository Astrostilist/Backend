-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_products_tags ON products USING GIN (tags);
-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_products_tags;