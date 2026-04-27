package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

const testAdminToken = "test-admin-token"

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
	filtered := make([]rules.Rule, 0, len(r.items))
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

	start := min(options.Offset, len(filtered))

	end := min(start+options.Limit, len(filtered))
	mtdata := rules.Metadata{
		CurrentPage:  options.Limit,
		PageSize:     options.Limit,
		FirstPage:    start,
		LastPage:     end,
		TotalRecords: len(filtered),
	}

	var subFiltered []*rules.Rule
	subFiltered = make([]*rules.Rule, 0)
	for i := start; i < end; i++ {
		subFiltered = append(subFiltered, &filtered[i])
	}
	// subFiltered := filtered[start:end]
	// myPointer := &subFiltered

	return subFiltered, mtdata, nil
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

func (r *fakeRulesRepository) Get(_ context.Context, id string) (*rules.Rule, error) {
	currentRule, ok := r.items[id]
	if !ok {
		return nil, rules.ErrRuleNotFound
	}

	currentRule.Name = ""
	currentRule.AstroCondition = map[string]string{}
	currentRule.ProductTags = []string{}
	currentRule.Priority = 1
	currentRule.IsActive = true
	currentRule.UpdatedAt = time.Now().UTC()
	r.items[id] = currentRule

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

	smplId, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:   smplId,
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

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/rules/"+smplId.String(), nil)
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()

	mux := newAdminRulesTestMux(repository)
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	storedRule, exists := repository.items[smplId.String()]
	if !exists {
		t.Fatal("expected soft-deleted rule to stay in repository")
	}

	if storedRule.IsActive {
		t.Fatal("expected soft-deleted rule to be deactivated")
	}
}

func TestCreateRuleSucceeds(t *testing.T) {
	t.Parallel()

	repository := newFakeRulesRepository(nil)
	payload := []byte(`{"name":"Lunar","astro_condition":{"moon":"full"},"product_tags":["mystic"],"priority":10,"is_active":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rules", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()

	mux := newAdminRulesTestMux(repository)
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", response.Code, response.Body.String())
	}
	fmt.Println("epository.items[created-rule-id] = ", repository.items)

	var resp Response
	if err := json.NewDecoder(response.Body).Decode(&resp); err != nil {
		t.Fatalf("cannot be decoded body: %v\n", err)
	}
	fmt.Printf("%v %T\n", resp.Data, resp.Data)

	bytes, _ := json.Marshal(resp.Data)
	output := make(map[string]string)

	err := json.Unmarshal(bytes, &output)
	if err != nil {
		t.Fatalf("cannot be decoded body Data: %v\n", err)
	}

	idStr, ok := output["id"]
	if !ok {
		t.Fatal("couldn't find  tag 'id'")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		t.Fatalf("cannot be get uuid: %v\n", err)
	}
	if id == uuid.Nil {
		t.Fatal("uuid is Nil")
	}

	// if _, ok := repository.items["created-rule-id"]; !ok {
	// 	t.Fatal("rule was not stored")
	// }
}

func TestUpdateRuleModifiesFields(t *testing.T) {
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

	payload := []byte(`{"name":"New","astro_condition":{"sign":"taurus"},"product_tags":["earth"],"priority":7,"is_active":true}`)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/rules/"+ruleID.String(), bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()

	mux := newAdminRulesTestMux(repository)
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", response.Code, response.Body.String())
	}

	updated := repository.items[ruleID.String()]
	if updated.Name != "New" || updated.Priority != 7 {
		t.Fatalf("rule not updated: %+v", updated)
	}
}

func TestUpdateRuleNotFound(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"name":"X","astro_condition":{"a":"b"},"product_tags":["t"],"priority":1,"is_active":true}`)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/rules/missing", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()

	mux := newAdminRulesTestMux(newFakeRulesRepository(nil))
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestListRulesFiltersByActiveFlag(t *testing.T) {
	t.Parallel()
	smplId1, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	smplId2, err := uuid.Parse("22222222-2222-2222-2222-222222222222")
	require.NoError(t, err)

	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:   smplId1,
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
			ID:   smplId2,
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

	items, ok := payload["rules"].([]any)
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

func TestGetRuleFiltersByActiveFlag(t *testing.T) {
	t.Parallel()
	smplId1, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	repository := newFakeRulesRepository([]rules.Rule{
		{
			ID:   smplId1,
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

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules/"+smplId1.String(), nil)
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

	item, ok := payload["rule"].(map[string]any)
	if !ok {
		t.Fatal("expected first rule item to be an object")
	}

	if item["is_active"] != true {
		t.Fatal("expected filtered item to be active")
	}

}
