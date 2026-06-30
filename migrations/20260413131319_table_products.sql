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

CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS $$ BEGIN NEW.updated_at = CURRENT_TIMESTAMP; RETURN NEW; END; $$ LANGUAGE plpgsql;

-- Ограничения
ALTER TABLE products ADD CONSTRAINT chk_ext_product_id CHECK (ext_product_id <> '');
ALTER TABLE products ADD CONSTRAINT chk_title CHECK (title <> '');
ALTER TABLE products ADD CONSTRAINT chk_price CHECK (price > 0.0);
ALTER TABLE products ADD CONSTRAINT chk_url_is_http_s CHECK (url = '' OR url ~* '^https?://');

-- Триггер для обновления updated_at
CREATE TRIGGER update_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER IF EXISTS update_products_updated_at ON products;

ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_ext_product_id;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_title;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_price;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_url_is_http_s;
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP INDEX IF EXISTS  idx_products_tags;
DROP INDEX IF EXISTS  idx_products_category;

DROP TABLE IF EXISTS products;

