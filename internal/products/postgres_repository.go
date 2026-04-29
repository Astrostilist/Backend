package products

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
)

type PostgresRepository struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(ctx context.Context, options ListOptions) (ListResult, error) {
	tagsFilter, err := buildTagsFilterArg(options.Tags)
	if err != nil {
		return ListResult{}, err
	}

	countQuery := `
		SELECT COUNT(*)
		FROM products
		WHERE ($1 = '' OR category = $1)
		  AND ($2::jsonb IS NULL OR tags @> $2::jsonb)
	`

	var totalCount int
	if err := r.db.QueryRowContext(ctx, countQuery, options.Category, tagsFilter).Scan(&totalCount); err != nil {
		return ListResult{}, fmt.Errorf("count products: %w", err)
	}

	query := `
		SELECT ext_product_id, title, price, tags, category, created_at, updated_at
		FROM products
		WHERE ($1 = '' OR category = $1)
		  AND ($2::jsonb IS NULL OR tags @> $2::jsonb)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.QueryContext(ctx, query, options.Category, tagsFilter, options.Limit, options.Offset)
	if err != nil {
		return ListResult{}, fmt.Errorf("list products: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("failed to close products rows: %v", closeErr)
		}
	}()

	items := make([]Product, 0)
	for rows.Next() {
		item, scanErr := scanProduct(rows)
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

func (r *PostgresRepository) GetBySKU(ctx context.Context, sku string) (Product, error) {
	query := `
		SELECT ext_product_id, title, price, tags, category, created_at, updated_at
		FROM products
		WHERE ext_product_id = $1
	`

	row := r.db.QueryRowContext(ctx, query, sku)

	productItem, err := scanProduct(row)
	if err != nil {
		return Product{}, err
	}

	return productItem, nil
}

func (r *PostgresRepository) Patch(ctx context.Context, sku string, input PatchInput) (Product, error) {
	if input.Price == nil && input.Tags == nil {
		return Product{}, errors.New("no product fields to update")
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
				WHERE ext_product_id = $1
				RETURNING ext_product_id, title, price, tags, category, created_at, updated_at
			`
			row = r.db.QueryRowContext(ctx, query, sku, *input.Price, tagsJSON)
		}
	case input.Price != nil:
		query := `
			UPDATE products
			SET price = $2,
			    updated_at = CURRENT_TIMESTAMP
			WHERE ext_product_id = $1
			RETURNING ext_product_id, title, price, tags, category, created_at, updated_at
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
				WHERE ext_product_id = $1
				RETURNING ext_product_id, title, price, tags, category, created_at, updated_at
			`
			row = r.db.QueryRowContext(ctx, query, sku, tagsJSON)
		}
	}
	if err != nil {
		return Product{}, err
	}

	productItem, err := scanProduct(row)
	if err != nil {
		return Product{}, err
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

func scanProduct(scanner rowScanner) (Product, error) {
	productItem := Product{}
	var tagsJSON []byte

	if err := scanner.Scan(
		&productItem.SKU,
		&productItem.Name,
		&productItem.Price,
		&tagsJSON,
		&productItem.Category,
		&productItem.CreatedAt,
		&productItem.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("scan product: %w", err)
	}

	if err := json.Unmarshal(tagsJSON, &productItem.Tags); err != nil {
		return Product{}, fmt.Errorf("decode product tags: %w", err)
	}
	if productItem.Tags == nil {
		productItem.Tags = []string{}
	}

	return productItem, nil
}
