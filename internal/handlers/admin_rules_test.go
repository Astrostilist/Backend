package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	rules "astroapi/internal/ruleengine"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeRulesRepository struct {
	items map[string]rules.Rule
}

func newFakeRulesRepository(items []rules.Rule) *fakeRulesRepository {
	repository := &fakeRulesRepository{items: make(map[string]rules.Rule, len(items))}
	for _, item := range items {
		repository.items[item.ID.String()] = item
	}
	return repository
}

func (r *fakeRulesRepository) List(_ context.Context, options rules.ListOptions) ([]*rules.Rule, rules.Metadata, error) {
	filtered := make([]rules.Rule, 0)
	for _, item := range r.items {
		if options.IsActive != nil && item.IsActive != *options.IsActive {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Priority == filtered[j].Priority {
			return filtered[i].ID.String() < filtered[j].ID.String()
		}
		return filtered[i].Priority < filtered[j].Priority
	})

	totalRecords := len(filtered)
	offset := (options.Page - 1) * options.PageSize
	if offset < 0 {
		offset = 0
	}

	start := offset
	if start > totalRecords {
		start = totalRecords
	}

	end := start + options.PageSize
	if end > totalRecords {
		end = totalRecords
	}

	totalPages := 0
	if options.PageSize > 0 {
		totalPages = (totalRecords + options.PageSize - 1) / options.PageSize
	}
	metadata := rules.Metadata{
		CurrentPage:  options.Page,
		PageSize:     options.PageSize,
		FirstPage:    1,
		LastPage:     totalPages,
		TotalRecords: totalRecords,
	}

	subFiltered := make([]*rules.Rule, 0)
	for i := start; i < end; i++ {
		subFiltered = append(subFiltered, &filtered[i])
	}

	return subFiltered, metadata, nil
}

func (r *fakeRulesRepository) Create(_ context.Context, input *rules.RuleInput) (uuid.UUID, error) {
	now := time.Now().UTC()
	smplID, err := uuid.Parse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	if err != nil {
		return uuid.Nil, err
	}
	createdRule := rules.Rule{
		ID:             smplID,
		Name:           input.Name,
		AstroCondition: input.AstroCondition,
		ProductTags:    input.ProductTags,
		Priority:       input.Priority,
		IsActive:       input.IsActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	r.items[createdRule.ID.String()] = createdRule
	return smplID, nil
}

func (r *fakeRulesRepository) Update(_ context.Context, id string, input *rules.RuleInput) (uuid.UUID, error) {
	currentRule, ok := r.items[id]
	if !ok {
		return uuid.Nil, rules.ErrRuleNotFound
	}

	currentRule.Name = input.Name
	currentRule.AstroCondition = input.AstroCondition
	currentRule.ProductTags = input.ProductTags
	currentRule.Priority = input.Priority
	currentRule.IsActive = input.IsActive
	currentRule.UpdatedAt = time.Now().UTC()
	r.items[id] = currentRule

	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, err
	}
	return parsedUUID, nil
}

func (r *fakeRulesRepository) Patch(_ context.Context, id string, input *rules.RuleInput) (uuid.UUID, error) {
	currentRule, ok := r.items[id]
	if !ok {
		return uuid.Nil, rules.ErrRuleNotFound
	}

	currentRule.Name = input.Name
	currentRule.AstroCondition = input.AstroCondition
	currentRule.ProductTags = input.ProductTags
	currentRule.Priority = input.Priority
	currentRule.IsActive = input.IsActive
	currentRule.UpdatedAt = time.Now().UTC()
	r.items[id] = currentRule

	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, err
	}
	return parsedUUID, nil
}

func (r *fakeRulesRepository) Get(_ context.Context, id string) (*rules.Rule, error) {
	currentRule, ok := r.items[id]
	if !ok {
		return nil, rules.ErrRuleNotFound
	}
	return &currentRule, nil
}

func (r *fakeRulesRepository) Delete(_ context.Context, id string) error {
	return nil
}

func (r *fakeRulesRepository) Deactivate(_ context.Context, id string) (*rules.Rule, error) {
	currentRule, ok := r.items[id]
	if !ok {
		return nil, rules.ErrRuleNotFound
	}

	currentRule.IsActive = false
	currentRule.UpdatedAt = time.Now().UTC()
	r.items[id] = currentRule

	return &currentRule, nil
}

func (r *fakeRulesRepository) Match(_ context.Context, tags []string) ([]string, error) {
	return []string{}, nil
}

func newAdminRulesTestMux(t *testing.T, repository rules.Repository) chi.Router {
	t.Helper()
	router := chi.NewRouter()
	handler := NewAdminRulesHandler(repository)
	router.Get("/api/v1/admin/rules", handler.ListRules)
	router.Post("/api/v1/admin/rules", handler.CreateRule)
	router.Get("/api/v1/admin/rules/{id}", handler.GetRule)
	router.Put("/api/v1/admin/rules/{id}", handler.UpdateRule)
	router.Patch("/api/v1/admin/rules/{id}", handler.PatchRule)
	router.Delete("/api/v1/admin/rules/{id}", handler.DeleteRule)
	return router
}

func TestRuleCreateRejectsNegativePriority(t *testing.T) {
	t.Parallel()

	mux := newAdminRulesTestMux(t, newFakeRulesRepository(nil))
	payload := []byte(`{"name":"Retrograde rule","astro_condition":{"planet":"mars"},"product_tags":["energy"],"priority":-1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rules", bytes.NewReader(payload))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, response.Code)
	}
}

func TestRuleDeleteSoftDeletesRecord(t *testing.T) {
	t.Parallel()

	smplID, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:   smplID,
			Name: "Seasonal rule",
			AstroCondition: map[string]string{
				"sign": "aries",
			},
			ProductTags: []string{"spring"},
			Priority:    10,
			IsActive:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})

	mux := newAdminRulesTestMux(t, repository)
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/rules/"+smplID.String(), nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	storedRule, exists := repository.items[smplID.String()]
	if !exists {
		t.Fatal("expected soft-deleted rule to stay in repository")
	}
	if storedRule.IsActive {
		t.Fatal("expected soft-deleted rule to be deactivated")
	}
}

func TestRuleCreateSucceeds(t *testing.T) {
	t.Parallel()

	repository := newFakeRulesRepository(nil)
	mux := newAdminRulesTestMux(t, repository)
	payload := []byte(`{"name":"Lunar","astro_condition":{"moon":"full"},"product_tags":["mystic"],"priority":10,"is_active":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rules", bytes.NewReader(payload))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", response.Code, response.Body.String())
	}

	var resp Response
	if err := json.NewDecoder(response.Body).Decode(&resp); err != nil {
		t.Fatalf("cannot decode body: %v", err)
	}

	b, _ := json.Marshal(resp.Data)
	output := make(map[string]string)
	if err := json.Unmarshal(b, &output); err != nil {
		t.Fatalf("cannot decode body Data: %v", err)
	}

	idStr, ok := output["id"]
	if !ok {
		t.Fatal("missing field 'id'")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		t.Fatalf("cannot parse uuid: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("uuid is Nil")
	}
}

func TestRuleUpdateModifiesFields(t *testing.T) {
	t.Parallel()

	ruleID, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:   ruleID,
			Name: "Old",
			AstroCondition: map[string]string{
				"sign": "aries",
			},
			ProductTags: []string{"fire"},
			Priority:    5,
			IsActive:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})

	mux := newAdminRulesTestMux(t, repository)
	payload := []byte(`{"name":"New","astro_condition":{"sign":"taurus"},"product_tags":["earth"],"priority":7,"is_active":true}`)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/rules/"+ruleID.String(), bytes.NewReader(payload))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", response.Code, response.Body.String())
	}

	updated := repository.items[ruleID.String()]
	if updated.Name != "New" || updated.Priority != 7 {
		t.Fatalf("rule not updated: %+v", updated)
	}
}

func TestRuleUpdateNotFound(t *testing.T) {
	t.Parallel()

	mux := newAdminRulesTestMux(t, newFakeRulesRepository(nil))
	payload := []byte(`{"name":"X","astro_condition":{"a":"b"},"product_tags":["t"],"priority":1,"is_active":true}`)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/rules/missing", bytes.NewReader(payload))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestRuleListFiltersByActiveFlag(t *testing.T) {
	t.Parallel()

	smplID1, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	smplID2, err := uuid.Parse("22222222-2222-2222-2222-222222222222")
	require.NoError(t, err)

	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:   smplID1,
			Name: "Active rule",
			AstroCondition: map[string]string{
				"sign": "aries",
			},
			ProductTags: []string{"spring"},
			Priority:    1,
			IsActive:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		{
			ID:   smplID2,
			Name: "Inactive rule",
			AstroCondition: map[string]string{
				"sign": "taurus",
			},
			ProductTags: []string{"earth"},
			Priority:    2,
			IsActive:    false,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})

	mux := newAdminRulesTestMux(t, repository)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules?is_active=true&page_size=50&page=1", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var envelope Response
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatal("expected response data to be a map")
	}

	items, ok := data["rules"].([]any)
	if !ok {
		t.Fatal("expected response data.rules to be a list")
	}

	if len(items) != 1 {
		t.Fatalf("expected one active rule, got %d", len(items))
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatal("expected first rule item to be an object")
	}

	if item["is_active"] != true {
		t.Fatal("expected filtered item to be active")
	}
}

func TestRuleGetReturnsStoredRule(t *testing.T) {
	t.Parallel()

	smplID, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:   smplID,
			Name: "Active rule",
			AstroCondition: map[string]string{
				"sign": "aries",
			},
			ProductTags: []string{"spring"},
			Priority:    1,
			IsActive:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})

	mux := newAdminRulesTestMux(t, repository)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules/"+smplID.String(), nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var envelope Response
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatal("expected response data to be a map")
	}

	item, ok := data["rule"].(map[string]any)
	if !ok {
		t.Fatal("expected response data.rule to be an object")
	}

	if item["is_active"] != true {
		t.Fatal("expected rule to be active")
	}
	if item["name"] != "Active rule" {
		t.Fatalf("expected name 'Active rule', got %v", item["name"])
	}
}
