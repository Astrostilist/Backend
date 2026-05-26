package products

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"astroapi/internal/models"

	"github.com/lib/pq"
)

// FindByTags ищет товары по тегам. limit не может быть больше 100.
func FindByTags(ctx context.Context, db *sql.DB, tags []string, limit, offset int) ([]models.CatalogProduct, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var builder strings.Builder
	builder.WriteString("{")
	for i, t := range tags {
		if i > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(fmt.Sprintf("%q", t))
	}
	builder.WriteString("}")
	pgArray := builder.String()

	query := `
        SELECT sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at,
               (SELECT COUNT(*) FROM jsonb_array_elements_text(tags) AS elem WHERE elem = ANY($1::text[])) AS match_count
        FROM products
        WHERE tags ?| $1::text[]
        ORDER BY match_count DESC, rating DESC
        LIMIT $2 OFFSET $3
    `

	rows, err := db.QueryContext(ctx, query, pgArray, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var products []models.CatalogProduct
	for rows.Next() {
		var p models.CatalogProduct
		var (
			tagsJSON []byte
			urlImgs  pq.StringArray
		)
		var rating sql.NullFloat64
		var matchCount int
		//        sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at
		if err := rows.Scan(
			&p.SKU,
			&p.Ext_product_id,
			&p.Name,
			&p.Price,
			&p.Article,
			&tagsJSON,
			&p.Category,
			&p.Url,
			&urlImgs,
			&rating,
			&p.CreatedAt,
			&p.UpdatedAt,
			&matchCount); err != nil {
			return nil, err
		}
		if rating.Valid {
			p.Rating = rating.Float64
		}
		if err := json.Unmarshal(tagsJSON, &p.Tags); err != nil {
			return nil, err
		}
		p.Images = []string(urlImgs)
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}
