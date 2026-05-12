-- +goose Up
CREATE TABLE IF NOT EXISTS products (
    sku TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    price DECIMAL(10,2) NOT NULL,
    tags JSONB,
    category TEXT
);

-- +goose Down
DROP TABLE IF EXISTS products;
