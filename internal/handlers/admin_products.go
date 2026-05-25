package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"astroapi/internal/products"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/go-chi/chi/v5"
)

const (
	defaultProductsPage     = 1
	defaultProductsPageSize = 50
	maxProductsPageSize     = 200
)

type AdminProductsHandler struct {
	repository       products.Repository
	cacheInvalidator products.CacheInvalidator
}

type adminProductsListResponse struct {
	Items    []products.Product `json:"items"`
	Metadata paginationMetadata `json:"metadata"`
}

type paginationMetadata struct {
	CurrentPage  int `json:"current_page"`
	PageSize     int `json:"page_size"`
	FirstPage    int `json:"first_page"`
	LastPage     int `json:"last_page"`
	TotalRecords int `json:"total_records"`
}

type adminProductPatchRequest struct {
	Price *float64  `json:"price"`
	Tags  *[]string `json:"tags"`
}

func NewAdminProductsHandler(repository products.Repository, cacheInvalidator products.CacheInvalidator) *AdminProductsHandler {
	return &AdminProductsHandler{repository: repository, cacheInvalidator: cacheInvalidator}
}

func RegisterAdminProductsRoutes(router chi.Router, secretTokenAdmin string, cache *memcache.Client, handler *AdminProductsHandler) {
	router.Route("/api/v1/admin/products", func(router chi.Router) {
		router.Use(AdminAuthMiddleware(cache, secretTokenAdmin))
		router.Get("/", handler.ListProducts)
		router.Get("/{sku}", handler.GetProduct)
		router.Patch("/{sku}", handler.PatchProduct)
	})
}

func (h *AdminProductsHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	options, page, pageSize, err := parseProductsListOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.repository.List(r.Context(), options)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch products")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Products fetched successfully",
		Data: adminProductsListResponse{
			Items:    result.Items,
			Metadata: buildPaginationMetadata(page, pageSize, result.TotalCount),
		},
	})
}

func (h *AdminProductsHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	sku := strings.TrimSpace(chi.URLParam(r, "sku"))
	if sku == "" {
		writeError(w, http.StatusBadRequest, "sku is required")
		return
	}

	productItem, err := h.repository.GetBySKU(r.Context(), sku)
	if err != nil {
		if errors.Is(err, products.ErrProductNotFound) {
			writeError(w, http.StatusNotFound, "product not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch product")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Product fetched successfully",
		Data:    productItem,
	})
}

func (h *AdminProductsHandler) PatchProduct(w http.ResponseWriter, r *http.Request) {
	sku := strings.TrimSpace(chi.URLParam(r, "sku"))
	if sku == "" {
		writeError(w, http.StatusBadRequest, "sku is required")
		return
	}

	payload, err := decodeProductPatchRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	input, tagsChanged, err := payload.toPatchInput()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	updatedProduct, err := h.repository.Patch(r.Context(), sku, input)
	if err != nil {
		if errors.Is(err, products.ErrProductNotFound) {
			writeError(w, http.StatusNotFound, "product not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update product")
		return
	}

	if tagsChanged && h.cacheInvalidator != nil {
		if err := h.cacheInvalidator.InvalidateProduct(r.Context(), sku); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to invalidate product cache")
			return
		}
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Product updated successfully",
		Data:    updatedProduct,
	})
}

func decodeProductPatchRequest(r *http.Request) (adminProductPatchRequest, error) {
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var payload adminProductPatchRequest
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return adminProductPatchRequest{}, errors.New("request body is required")
		}
		return adminProductPatchRequest{}, err
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return adminProductPatchRequest{}, errors.New("request body must contain a single JSON object")
	}

	return payload, nil
}

func (r adminProductPatchRequest) toPatchInput() (products.PatchInput, bool, error) {
	if r.Price == nil && r.Tags == nil {
		return products.PatchInput{}, false, errors.New("at least one field must be provided")
	}

	if r.Price != nil && *r.Price < 0 {
		return products.PatchInput{}, false, errors.New("price must be greater than or equal to 0")
	}

	input := products.PatchInput{Price: r.Price}
	tagsChanged := r.Tags != nil
	if r.Tags != nil {
		normalizedTags, err := normalizeProductTags(*r.Tags)
		if err != nil {
			return products.PatchInput{}, false, err
		}
		input.Tags = &normalizedTags
	}

	return input, tagsChanged, nil
}

func parseProductsListOptions(r *http.Request) (products.ListOptions, int, int, error) {
	query := r.URL.Query()

	page, err := parseBoundedPositiveInt(query.Get("page"), defaultProductsPage, 1, 0, "page")
	if err != nil {
		return products.ListOptions{}, 0, 0, err
	}

	pageSize, err := parseBoundedPositiveInt(query.Get("page_size"), defaultProductsPageSize, 1, maxProductsPageSize, "page_size")
	if err != nil {
		return products.ListOptions{}, 0, 0, err
	}

	tags := parseProductTagsFilter(query["tags"])

	return products.ListOptions{
		Category: strings.TrimSpace(query.Get("category")),
		Tags:     tags,
		Limit:    pageSize,
		Offset:   (page - 1) * pageSize,
	}, page, pageSize, nil
}

func parseBoundedPositiveInt(raw string, defaultValue, minValue, maxValue int, fieldName string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultValue, nil
	}

	parsedValue, err := strconv.Atoi(raw)
	if err != nil || parsedValue < minValue || (maxValue > 0 && parsedValue > maxValue) {
		if maxValue > 0 {
			return 0, errors.New(fieldName + " must be an integer between 1 and " + strconv.Itoa(maxValue))
		}
		return 0, errors.New(fieldName + " must be an integer greater than or equal to 1")
	}

	return parsedValue, nil
}

func parseProductTagsFilter(rawValues []string) []string {
	tags := make([]string, 0)
	for _, rawValue := range rawValues {
		for _, part := range strings.Split(rawValue, ",") {
			tag := strings.TrimSpace(part)
			if tag == "" {
				continue
			}
			tags = append(tags, tag)
		}
	}

	return tags
}

func normalizeProductTags(rawTags []string) ([]string, error) {
	tags := make([]string, 0, len(rawTags))
	for _, rawTag := range rawTags {
		tag := strings.TrimSpace(rawTag)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}

	if len(rawTags) > 0 && len(tags) == 0 {
		return nil, errors.New("tags must contain non-empty values")
	}

	return tags, nil
}

func buildPaginationMetadata(page, pageSize, totalRecords int) paginationMetadata {
	lastPage := 1
	if totalRecords > 0 {
		lastPage = (totalRecords + pageSize - 1) / pageSize
	}

	return paginationMetadata{
		CurrentPage:  page,
		PageSize:     pageSize,
		FirstPage:    1,
		LastPage:     lastPage,
		TotalRecords: totalRecords,
	}
}
