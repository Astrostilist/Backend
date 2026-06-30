// Package products описывает товары и операции с ними.
package products

import (
	"astroapi/internal/models"
	"context"
	"errors"
)

var ErrProductNotFound = errors.New("product not found")

// type Product struct {
// 	SKU       string    `json:"sku"`
// 	Name      string    `json:"name"`
// 	Price     float64   `json:"price"`
// 	Tags      []string  `json:"tags"`
// 	Category  string    `json:"category"`
// 	CreatedAt time.Time `json:"created_at"`
// 	UpdatedAt time.Time `json:"updated_at"`
// }

type ListOptions struct {
	Category string
	Tags     []string
	Limit    int
	Offset   int
}

type ListResult struct {
	Items      []models.CatalogProduct
	TotalCount int
}

type PatchInput struct {
	Price *float64
	Tags  *[]string
}

type CacheInvalidator interface {
	InvalidateProduct(ctx context.Context, sku string) error
}

//go:generate mockgen -source=products.go -destination=mocks/mock_products.go -package=mocks
type Repository interface {
	List(ctx context.Context, options ListOptions) (ListResult, error)
	GetBySKU(ctx context.Context, sku string) (models.CatalogProduct, error)
	Patch(ctx context.Context, sku string, input PatchInput) (models.CatalogProduct, error)
	Recommend(ctx context.Context, tags []string, gender, scenario string, preferences map[string]any) ([]models.CatalogProduct, error)
}
