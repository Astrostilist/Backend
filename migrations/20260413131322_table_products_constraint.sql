-- +goose Up
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
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_images_urls;
