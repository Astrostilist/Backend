package products

import (
	"astroapi/internal/models"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

type PostgresRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(ctx context.Context, options ListOptions) (ListResult, error) {
	totalCount := 0
	query := `
	SELECT COUNT(*) OVER(), 
               sku, ext_product_id, title, price, article, 
               tags, category, url, images, rating, created_at, updated_at
        FROM products
        WHERE ($1 = '' OR category ILIKE '%' || $1 || '%')
          AND ($2::jsonb IS NULL OR tags @> $2::jsonb)
        ORDER BY created_at DESC
        LIMIT $3 OFFSET $4
	`

	tagsFilter, err := buildTagsFilterArg(options.Tags)
	if err != nil {
		return ListResult{}, err
	}
	args := []any{options.Category, tagsFilter, options.Limit, options.Offset}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list products: %w", err)
	}
	defer func() {
		err = rows.Close()
		if err != nil {
			r.logger.Error("failed to rows close", zap.Error(err))
		}
	}()

	items := make([]models.CatalogProduct, 0)
	for rows.Next() {
		var item models.CatalogProduct
		var scanErr error
		item, totalCount, scanErr = scanProduct(rows, true)
		if scanErr != nil {
			return ListResult{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate products: %w", err)
	}

	return ListResult{Items: items, TotalCount: totalCount}, nil
}

func (r *PostgresRepository) GetBySKU(ctx context.Context, sku string) (models.CatalogProduct, error) {
	query := `
		SELECT sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at
		FROM products
		WHERE sku = $1
	`

	row := r.db.QueryRowContext(ctx, query, sku)

	productItem, _, err := scanProduct(row, false)
	if err != nil {
		return models.CatalogProduct{}, err
	}

	return productItem, nil
}

func (r *PostgresRepository) Patch(ctx context.Context, sku string, input PatchInput) (models.CatalogProduct, error) {
	if input.Price == nil && input.Tags == nil {
		return models.CatalogProduct{}, errors.New("no product fields to update")
	}

	var (
		row rowScanner
		err error
	)

	switch {
	case input.Price != nil && input.Tags != nil:
		var tagsJSON string
		tagsJSON, err = marshalProductTags(*input.Tags)
		if err == nil {
			query := `
				UPDATE products
				SET price = $2,
				    tags = $3::jsonb,
				    updated_at = CURRENT_TIMESTAMP
				WHERE sku = $1
				RETURNING sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at
			`
			row = r.db.QueryRowContext(ctx, query, sku, *input.Price, tagsJSON)
		}
	case input.Price != nil:
		query := `
			UPDATE products
			SET price = $2,
			    updated_at = CURRENT_TIMESTAMP
			WHERE sku = $1
			RETURNING sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at
		`
		row = r.db.QueryRowContext(ctx, query, sku, *input.Price)
	case input.Tags != nil:
		var tagsJSON string
		tagsJSON, err = marshalProductTags(*input.Tags)
		if err == nil {
			query := `
				UPDATE products
				SET tags = $2::jsonb,
				    updated_at = CURRENT_TIMESTAMP
				WHERE sku = $1
				RETURNING sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at
			`
			row = r.db.QueryRowContext(ctx, query, sku, tagsJSON)
		}
	}
	if err != nil {
		return models.CatalogProduct{}, err
	}

	productItem, _, err := scanProduct(row, false)
	if err != nil {
		return models.CatalogProduct{}, err
	}

	return productItem, nil
}

func buildTagsFilterArg(tags []string) (any, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	return marshalProductTags(tags)
}

func marshalProductTags(tags []string) (string, error) {
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("marshal product tags: %w", err)
	}

	return string(tagsJSON), nil
}

func scanProduct(scanner rowScanner, withCount bool) (models.CatalogProduct, int, error) {
	productItem := models.CatalogProduct{}
	var (
		tagsJSON   []byte
		urlImgs    pq.StringArray
		totalCount int
		err        error
	)
	// sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at

	if withCount {
		err = scanner.Scan(
			&totalCount,
			&productItem.SKU,
			&productItem.ExtProductID,
			&productItem.Name,
			&productItem.Price,
			&productItem.Article,
			&tagsJSON,
			&productItem.Category,
			&productItem.URL,
			&urlImgs,
			&productItem.Rating,
			&productItem.CreatedAt,
			&productItem.UpdatedAt,
		)
	} else {
		err = scanner.Scan(
			&productItem.SKU,
			&productItem.ExtProductID,
			&productItem.Name,
			&productItem.Price,
			&productItem.Article,
			&tagsJSON,
			&productItem.Category,
			&productItem.URL,
			&urlImgs,
			&productItem.Rating,
			&productItem.CreatedAt,
			&productItem.UpdatedAt,
		)
		if err != nil {
			totalCount = 1
		}
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.CatalogProduct{}, 0, ErrProductNotFound
		}
		return models.CatalogProduct{}, 0, fmt.Errorf("scan product: %w", err)
	}

	if err := json.Unmarshal(tagsJSON, &productItem.Tags); err != nil {
		return models.CatalogProduct{}, 0, fmt.Errorf("decode product tags: %w", err)
	}
	if productItem.Tags == nil {
		productItem.Tags = []string{}
	}

	productItem.Images = []string(urlImgs)

	return productItem, totalCount, nil
}

func (r *PostgresRepository) Recommend(ctx context.Context, tags []string, gender, scenario string, preferences map[string]any) ([]models.CatalogProduct, error) {

	// TODO: добавить учет доп параметров в поиске (gen, scen, pref)
	limit := 100
	offset := 0

	var builder strings.Builder
	builder.WriteString("{")
	for i, t := range tags {
		if i > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(strconv.Quote(t))
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

	rows, err := r.db.QueryContext(ctx, query, pgArray, limit, offset)
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
			&p.ExtProductID,
			&p.Name,
			&p.Price,
			&p.Article,
			&tagsJSON,
			&p.Category,
			&p.URL,
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
