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
)

const testAdminToken = "test-admin-token"

type fakeRulesRepository struct {
	items map[string]rules.Rule
}

func newFakeRulesRepository(items []rules.Rule) *fakeRulesRepository {
	repository := &fakeRulesRepository{items: make(map[string]rules.Rule, len(items))}
	for _, item := range items {
		repository.items[item.ID] = item
	}
	return repository
}

func (r *fakeRulesRepository) List(_ context.Context, options rules.ListOptions) (rules.ListResult, error) {
	filtered := make([]rules.Rule, 0, len(r.items))
	for _, item := range r.items {
		if options.IsActive != nil && item.IsActive != *options.IsActive {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Priority == filtered[j].Priority {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Priority < filtered[j].Priority
	})

	start := min(options.Offset, len(filtered))

	end := min(start+options.Limit, len(filtered))

	return rules.ListResult{
		Items:      filtered[start:end],
		TotalCount: len(filtered),
	}, nil
}

func (r *fakeRulesRepository) Create(_ context.Context, input rules.RuleInput) (rules.Rule, error) {
	now := time.Now().UTC()
	createdRule := rules.Rule{
		ID:             "created-rule-id",
		Name:           input.Name,
		AstroCondition: input.AstroCondition,
		ProductTags:    input.ProductTags,
		Priority:       input.Priority,
		IsActive:       input.IsActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	r.items[createdRule.ID] = createdRule
	return createdRule, nil
}

func (r *fakeRulesRepository) Update(_ context.Context, id string, input rules.RuleInput) (rules.Rule, error) {
	currentRule, ok := r.items[id]
	if !ok {
		return rules.Rule{}, rules.ErrRuleNotFound
	}

	currentRule.Name = input.Name
	currentRule.AstroCondition = input.AstroCondition
	currentRule.ProductTags = input.ProductTags
	currentRule.Priority = input.Priority
	currentRule.IsActive = input.IsActive
	currentRule.UpdatedAt = time.Now().UTC()
	r.items[id] = currentRule

	return currentRule, nil
}

func (r *fakeRulesRepository) Deactivate(_ context.Context, id string) (rules.Rule, error) {
	currentRule, ok := r.items[id]
	if !ok {
		return rules.Rule{}, rules.ErrRuleNotFound
	}

	currentRule.IsActive = false
	currentRule.UpdatedAt = time.Now().UTC()
	r.items[id] = currentRule

	return currentRule, nil
}
func (r *fakeRulesRepository) Match(_ context.Context, tags []string) ([]string, error) {

	return []string{}, nil
}
func newAdminRulesTestMux(repository rules.Repository) chi.Router {
	router := chi.NewRouter()
	RegisterAdminRulesRoutes(router, testAdminToken, NewAdminRulesHandler(repository))
	return router
}

func TestAdminRulesRequireToken(t *testing.T) {
	t.Parallel()

	mux := newAdminRulesTestMux(newFakeRulesRepository(nil))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestCreateRuleRejectsNegativePriority(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"name":"Retrograde rule","astro_condition":{"planet":"mars"},"product_tags":["energy"],"priority":-1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rules", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()

	mux := newAdminRulesTestMux(newFakeRulesRepository(nil))
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, response.Code)
	}
}

func TestDeleteRuleSoftDeletesRecord(t *testing.T) {
	t.Parallel()

	ruleID := "11111111-1111-1111-1111-111111111111"
	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:             ruleID,
			Name:           "Seasonal rule",
			AstroCondition: map[string]any{"sign": "aries"},
			ProductTags:    []string{"spring"},
			Priority:       10,
			IsActive:       true,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		},
	})

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/rules/"+ruleID, nil)
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()

	mux := newAdminRulesTestMux(repository)
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	storedRule, exists := repository.items[ruleID]
	if !exists {
		t.Fatal("expected soft-deleted rule to stay in repository")
	}

	if storedRule.IsActive {
		t.Fatal("expected soft-deleted rule to be deactivated")
	}
}

func TestListRulesFiltersByActiveFlag(t *testing.T) {
	t.Parallel()

	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:             "11111111-1111-1111-1111-111111111111",
			Name:           "Active rule",
			AstroCondition: map[string]any{"sign": "aries"},
			ProductTags:    []string{"spring"},
			Priority:       1,
			IsActive:       true,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		},
		{
			ID:             "22222222-2222-2222-2222-222222222222",
			Name:           "Inactive rule",
			AstroCondition: map[string]any{"sign": "taurus"},
			ProductTags:    []string{"earth"},
			Priority:       2,
			IsActive:       false,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules?is_active=true&limit=50&offset=0", nil)
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()

	mux := newAdminRulesTestMux(repository)
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
