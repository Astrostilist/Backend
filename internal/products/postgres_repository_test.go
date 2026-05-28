package products_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"astroapi/internal/products"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func newProductsRepo(t *testing.T) (*products.PostgresRepository, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	return products.NewPostgresRepository(db), db, mock
}

func TestPostgresRepositoryListFiltersByCategory(t *testing.T) {
	t.Parallel()

	repository, db, mock := newProductsRepo(t)
	defer db.Close()

	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"count", "sku", "ext_product_id", "title", "price", "article",
		"tags", "category", "url", "images", "rating", "created_at", "updated_at",
	}).AddRow(
		1,
		"sku-1", "1", "Silver ring", 2500.0, "art-1",
		[]byte(`["silver"]`), "rings", "https://example.com",
		pq.Array([]string{"https://example.com/img.jpg"}), 4.5, now, now)

	mock.ExpectQuery(`SELECT COUNT\(\*\) OVER\(\), sku, ext_product_id, title, price, article, tags, category, url, images, rating, created_at, updated_at FROM products WHERE \(\$1 = '' OR category ILIKE '%' || $1 || '%'\) AND \(\$2::jsonb IS NULL OR tags @> $2::jsonb\) ORDER BY created_at DESC LIMIT $3 OFFSET $4`).
		WithArgs("rings", nil, 10, 0).
		WillReturnRows(rows)

	result, err := repository.List(context.Background(), products.ListOptions{
		Category: "rings",
		Limit:    10,
		Offset:   0,
	})

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "rings", result.Items[0].Category)
	require.Equal(t, 1, result.TotalCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresRepositoryPatchUnknownSKU(t *testing.T) {
	t.Parallel()

	repository, db, mock := newProductsRepo(t)
	defer db.Close()

	tags := []string{"new"}
	mock.ExpectQuery(`UPDATE products\s+SET tags = \$2::jsonb`).
		WithArgs("missing-sku", `["new"]`).
		WillReturnError(sql.ErrNoRows)

	_, err := repository.Patch(context.Background(), "missing-sku", products.PatchInput{Tags: &tags})

	require.ErrorIs(t, err, products.ErrProductNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresRepositoryPatchEmptyInput(t *testing.T) {
	t.Parallel()

	repository, db, _ := newProductsRepo(t)
	defer db.Close()

	_, err := repository.Patch(context.Background(), "sku-1", products.PatchInput{})

	require.Error(t, err)
	require.False(t, errors.Is(err, products.ErrProductNotFound))
}
