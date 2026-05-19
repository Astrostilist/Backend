-- +goose Up
-- Функция для авто-обновления updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Функция для проверки всех URL в массиве
CREATE OR REPLACE FUNCTION check_images_urls(images_arr TEXT[])
RETURNS BOOLEAN AS $$
BEGIN
    IF array_length(images_arr, 1) IS NULL THEN
        RETURN TRUE;
    END IF;
    
    RETURN (
        SELECT bool_and(
            img = '' OR
            img ~* '^https?://(localhost|([0-9]{1,3}\.){3}[0-9]{1,3}|[a-zA-Z0-9_-]+(\.[a-zA-Z0-9_-]+)*\.[a-zA-Z]{2,})(:[0-9]+)?(/.*)?$'
        )
        FROM unnest(images_arr) AS img
        WHERE img != ''
    );
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ext_product_id VARCHAR(255) NOT NULL UNIQUE,
    title VARCHAR(500) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    price NUMERIC(10,2) NOT NULL DEFAULT 0.0,
    url TEXT NOT NULL DEFAULT '',
    images TEXT[] NOT NULL DEFAULT '{}',
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    category TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Ограничения
    CONSTRAINT chk_ext_product_id CHECK (ext_product_id <> ''),
    CONSTRAINT chk_title CHECK (title <> ''),
    CONSTRAINT chk_price CHECK (price >= 0.0),
    CONSTRAINT chk_url_is_http_s CHECK (url = '' OR url ~* '^https?://'),
    CONSTRAINT chk_images_urls CHECK (check_images_urls(images))
);

-- Триггер для обновления updated_at
CREATE TRIGGER update_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down

DROP TABLE IF EXISTS products;

DROP FUNCTION IF EXISTS update_updated_at_column();
DROP FUNCTION IF EXISTS check_images_urls(TEXT[]);


-- CREATE TABLE IF NOT EXISTS products (
-- -    sku TEXT PRIMARY KEY,   =>  ext_product_id
-- -    name TEXT NOT NULL,     =>  title
-- V    description TEXT,
-- +    price DECIMAL(10,2) NOT NULL,
-- +    tags JSONB,
--     category TEXT
-- );