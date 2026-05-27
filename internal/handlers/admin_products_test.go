package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"astroapi/internal/products"

	"github.com/go-chi/chi/v5"
)

type fakeProductsRepository struct {
	items   map[string]products.Product
	listErr error
}

func newFakeProductsRepository(items []products.Product) *fakeProductsRepository {
	repository := &fakeProductsRepository{items: make(map[string]products.Product, len(items))}
	for _, item := range items {
		repository.items[item.SKU] = item
	}
	return repository
}

func (r *fakeProductsRepository) List(_ context.Context, options products.ListOptions) (products.ListResult, error) {
	if r.listErr != nil {
		return products.ListResult{}, r.listErr
	}

	filtered := make([]products.Product, 0, len(r.items))
	for _, item := range r.items {
		if options.Category != "" && item.Category != options.Category {
			continue
		}
		if len(options.Tags) > 0 && !containsAllTags(item.Tags, options.Tags) {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].SKU < filtered[j].SKU
	})

	start := min(options.Offset, len(filtered))
	end := min(start+options.Limit, len(filtered))

	return products.ListResult{
		Items:      filtered[start:end],
		TotalCount: len(filtered),
	}, nil
}

func (r *fakeProductsRepository) GetBySKU(_ context.Context, sku string) (products.Product, error) {
	productItem, ok := r.items[sku]
	if !ok {
		return products.Product{}, products.ErrProductNotFound
	}
	return productItem, nil
}

func (r *fakeProductsRepository) Patch(_ context.Context, sku string, input products.PatchInput) (products.Product, error) {
	productItem, ok := r.items[sku]
	if !ok {
		return products.Product{}, products.ErrProductNotFound
	}

	if input.Price != nil {
		productItem.Price = *input.Price
	}
	if input.Tags != nil {
		productItem.Tags = *input.Tags
	}
	productItem.UpdatedAt = time.Now().UTC()
	r.items[sku] = productItem

	return productItem, nil
}

type fakeProductCacheInvalidator struct {
	invalidatedSKU string
}

func (i *fakeProductCacheInvalidator) InvalidateProduct(_ context.Context, sku string) error {
	i.invalidatedSKU = sku
	return nil
}

func newAdminProductsTestMux(t *testing.T, repository products.Repository, invalidator products.CacheInvalidator) chi.Router {
	t.Helper()
	router := chi.NewRouter()
	handler := NewAdminProductsHandler(repository, invalidator, nil)
	router.Get("/api/v1/admin/products", handler.ListProducts)
	router.Patch("/api/v1/admin/products/{sku}", handler.PatchProduct)
	return router
}

func containsAllTags(productTags []string, filterTags []string) bool {
	available := make(map[string]struct{}, len(productTags))
	for _, tag := range productTags {
		available[tag] = struct{}{}
	}

	for _, tag := range filterTags {
		if _, ok := available[tag]; !ok {
			return false
		}
	}
	return true
}

func TestListProductsFiltersByCategory(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := newFakeProductsRepository([]products.Product{
		{
			SKU:       "sku-1",
			Name:      "Silk scarf",
			Price:     1200,
			Tags:      []string{"silk"},
			Category:  "scarves",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			SKU:       "sku-2",
			Name:      "Silver ring",
			Price:     2500,
			Tags:      []string{"silver"},
			Category:  "rings",
			CreatedAt: now,
			UpdatedAt: now,
		},
	})

	mux := newAdminProductsTestMux(t, repository, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?category=rings&page=1&page_size=10", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var envelope Response
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	payload, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatal("expected response data to be a map")
	}

	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatal("expected response data.items to be a list")
	}
	if len(items) != 1 {
		t.Fatalf("expected one product, got %d", len(items))
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatal("expected first product item to be an object")
	}
	if item["category"] != "rings" {
		t.Fatalf("expected rings category, got %v", item["category"])
	}
}

func TestListProductsUnknownTagReturnsEmptyList(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := newFakeProductsRepository([]products.Product{
		{
			SKU:       "sku-1",
			Name:      "Silk scarf",
			Price:     1200,
			Tags:      []string{"silk"},
			Category:  "scarves",
			CreatedAt: now,
			UpdatedAt: now,
		},
	})

	mux := newAdminProductsTestMux(t, repository, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?tags=unknown", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var envelope Response
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	payload, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatal("expected response data to be a map")
	}

	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatal("expected response data.items to be a list")
	}
	if len(items) != 0 {
		t.Fatalf("expected empty products list, got %d", len(items))
	}

	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected response data.metadata to be a map")
	}
	if metadata["total_records"] != float64(0) {
		t.Fatalf("expected total_records=0, got %v", metadata["total_records"])
	}
}

func TestListProductsDBErrorReturnsUnavailable(t *testing.T) {
	t.Parallel()

	repository := newFakeProductsRepository(nil)
	repository.listErr = errors.New("connection refused")

	mux := newAdminProductsTestMux(t, repository, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?tags=silk", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, response.Code)
	}

	var envelope Response
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != "db_unavailable" {
		t.Fatalf("expected db_unavailable error, got %q", envelope.Error)
	}
}

func TestPatchProductUnknownSKUReturnsNotFound(t *testing.T) {
	t.Parallel()

	mux := newAdminProductsTestMux(t, newFakeProductsRepository(nil), nil)

	payload := []byte(`{"tags":["new"]}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/products/missing-sku", bytes.NewReader(payload))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}

func TestPatchProductTagsInvalidatesCache(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := newFakeProductsRepository([]products.Product{
		{
			SKU:       "sku-1",
			Name:      "Silk scarf",
			Price:     1200,
			Tags:      []string{"old"},
			Category:  "scarves",
			CreatedAt: now,
			UpdatedAt: now,
		},
	})
	invalidator := &fakeProductCacheInvalidator{}
	mux := newAdminProductsTestMux(t, repository, invalidator)

	payload := []byte(`{"tags":["new","summer"]}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/products/sku-1", bytes.NewReader(payload))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, response.Code, response.Body.String())
	}
	if invalidator.invalidatedSKU != "sku-1" {
		t.Fatalf("expected cache invalidation for sku-1, got %q", invalidator.invalidatedSKU)
	}
}
