-- +goose Up
CREATE TABLE IF NOT EXISTS products (
    sku VARCHAR(255) PRIMARY KEY,
    ext_product_id VARCHAR(255) NOT NULL UNIQUE,
    title VARCHAR(500) NOT NULL DEFAULT '',
    article TEXT  NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    price NUMERIC(10,2) NOT NULL,
    url TEXT NOT NULL DEFAULT '',
    images TEXT[] NOT NULL DEFAULT '{}',
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    rating DECIMAL(3,2) NOT NULL DEFAULT 0,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
    
-- Создание индексов (без CONCURRENTLY для миграций)
CREATE INDEX IF NOT EXISTS idx_products_tags ON products USING GIN (tags);
CREATE INDEX IF NOT EXISTS idx_products_category ON products(category);


-- +goose Down
DROP INDEX IF EXISTS  idx_products_tags;
DROP INDEX IF EXISTS  idx_products_category;

DROP TABLE IF EXISTS products;



-- CREATE TABLE IF NOT EXISTS products (
-- -    sku TEXT PRIMARY KEY,   =>  ext_product_id
-- -    name TEXT NOT NULL,     =>  title
-- V    description TEXT,
-- +    price DECIMAL(10,2) NOT NULL,
-- +    tags JSONB,
--     category TEXT
-- );