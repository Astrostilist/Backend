-- -- +goose Up
-- ALTER TABLE products
--     ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT '';

-- -- +goose Down
-- ALTER TABLE products
--     DROP COLUMN IF EXISTS category;
