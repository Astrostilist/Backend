-- +goose NO TRANSACTION
-- +goose Up
-- ALTER TABLE products ADD COLUMN IF NOT EXISTS rating DECIMAL(3,2) DEFAULT 0;
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_products_tags ON products USING GIN (tags);

-- -- +goose Down
-- DROP INDEX CONCURRENTLY IF EXISTS idx_products_tags;
-- ALTER TABLE products DROP COLUMN IF EXISTS rating;
