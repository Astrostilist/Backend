package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"astroapi/internal/ruleengine"
	rules "astroapi/internal/ruleengine"

	"github.com/go-chi/chi/v5"
)

const (
	defaultRulesLimit   = 50
	maxRulesLimit       = 200
	defaultRulePriority = 99
)

type AdminRulesHandler struct {
	repository rules.Repository
}

type adminRuleRequest struct {
	Name           string            `json:"name"`
	AstroCondition map[string]string `json:"astro_condition"`
	ProductTags    []string          `json:"product_tags"`
	Priority       *int              `json:"priority"`
	IsActive       *bool             `json:"is_active"`
}

func NewAdminRulesHandler(repository rules.Repository) *AdminRulesHandler {
	return &AdminRulesHandler{repository: repository}
}

func RegisterAdminRulesRoutes(router chi.Router, adminToken string, handler *AdminRulesHandler) {
	router.Route("/api/v1/admin/rules", func(router chi.Router) {
		router.Use(AdminAuthMiddleware(adminToken))
		router.Get("/", handler.ListRules)
		router.Post("/", handler.CreateRule)
		router.Get("/{id}", handler.GetRule)
		router.Put("/{id}", handler.UpdateRule)
		router.Patch("/{id}", handler.PatchRule)
		router.Delete("/{id}", handler.DeleteRule)
	})
}

// ListRules - обрабатывает  get, выдает список правил.
func (h *AdminRulesHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	listOptions, err := parseRulesListOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, metadata, err := h.repository.List(r.Context(), listOptions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch astro rules")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Astro rules fetched successfully",
		Data:    map[string]any{"rules": result, "metadata": metadata},
	})
}

// CreateRule - обрабатывает POST, создает правило в БД.
func (h *AdminRulesHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeAdminRuleRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	input, err := payload.toRuleInput()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	idCreatedRule, err := h.repository.Create(r.Context(), &input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create astro rule")
		return
	}
	result := map[string]any{"id": idCreatedRule.String()}

	writeJSON(w, http.StatusCreated, Response{
		Message: "Astro rule created successfully",
		Data:    result,
	})
}

func (h *AdminRulesHandler) GetRule(w http.ResponseWriter, r *http.Request) {
	ruleID := strings.TrimSpace(chi.URLParam(r, "id"))
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "rule id is required")
		return
	}

	rule, err := h.repository.Get(r.Context(), ruleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch astro rules")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Astro rules fetched successfully",
		Data:    map[string]any{"rule": rule},
	})
}

func (h *AdminRulesHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	ruleID := strings.TrimSpace(chi.URLParam(r, "id"))
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "rule id is required")
		return
	}

	payload, err := decodeAdminRuleRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	input, err := payload.toRuleInput()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	updatedRule, err := h.repository.Update(r.Context(), ruleID, &input)
	if err != nil {
		if errors.Is(err, rules.ErrRuleNotFound) {
			writeError(w, http.StatusNotFound, "astro rule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update astro rule")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Astro rule updated successfully",
		Data:    map[string]any{"id": updatedRule.String()},
	})
}

// PatchRule - частичное обновление.
func (h *AdminRulesHandler) PatchRule(w http.ResponseWriter, r *http.Request) {
	ruleID := strings.TrimSpace(chi.URLParam(r, "id"))
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "rule id is required")
		return
	}
	// получить данные,если запись уже есть в БД
	rule, err := h.repository.Get(r.Context(), ruleID)
	if err != nil {
		switch {
		case errors.Is(err, ruleengine.ErrRuleNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	payload, err := decodeAdminRuleRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// копирование, т.к. rule *rules.Rule
	var copiedRule adminRuleRequest
	copiedRule.Name = rule.Name
	copiedRule.AstroCondition = maps.Clone(rule.AstroCondition)
	copy(copiedRule.ProductTags, rule.ProductTags)
	copiedRule.Priority = &rule.Priority
	copiedRule.IsActive = &rule.IsActive

	// выполним проверки введенных данных
	// если значения равны nil => пользователь не указал данные
	if payload.Name != "" {
		copiedRule.Name = payload.Name
	}
	if payload.AstroCondition != nil {
		copiedRule.AstroCondition = maps.Clone(payload.AstroCondition)
	}
	if len(payload.ProductTags) > 0 {
		copy(copiedRule.ProductTags, payload.ProductTags)
	}
	if payload.Priority != nil {
		copiedRule.Priority = payload.Priority
	}
	if payload.IsActive != nil {
		copiedRule.IsActive = payload.IsActive
	}
	// валидация
	input, err := copiedRule.toRuleInput()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// воспользуемся матодом Update
	updatedRule, err := h.repository.Update(r.Context(), ruleID, &input)

	if err != nil {
		if errors.Is(err, rules.ErrRuleNotFound) {
			writeError(w, http.StatusNotFound, "astro rule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update astro rule")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Astro rule updated successfully",
		Data:    map[string]any{"id": updatedRule.String()},
	})
}

func (h *AdminRulesHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	ruleID := strings.TrimSpace(chi.URLParam(r, "id"))
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "rule id is required")
		return
	}

	deactivatedRule, err := h.repository.Deactivate(r.Context(), ruleID)
	if err != nil {
		if errors.Is(err, rules.ErrRuleNotFound) {
			writeError(w, http.StatusNotFound, "astro rule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to deactivate astro rule")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Astro rule deactivated successfully",
		Data:    deactivatedRule,
	})
}

func decodeAdminRuleRequest(r *http.Request) (adminRuleRequest, error) {
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var payload adminRuleRequest
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return adminRuleRequest{}, errors.New("request body is required")
		}
		return adminRuleRequest{}, err
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return adminRuleRequest{}, errors.New("request body must contain a single JSON object")
	}

	return payload, nil
}

func (r adminRuleRequest) toRuleInput() (rules.RuleInput, error) {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return rules.RuleInput{}, errors.New("name is required")
	}

	if r.AstroCondition == nil {
		return rules.RuleInput{}, errors.New("astro_condition is required")
	}

	priority := defaultRulePriority
	if r.Priority != nil {
		priority = *r.Priority
	}
	if priority < 0 {
		return rules.RuleInput{}, errors.New("priority must be greater than or equal to 0")
	}

	productTags := normalizeTags(r.ProductTags)

	isActive := true
	if r.IsActive != nil {
		isActive = *r.IsActive
	}

	return rules.RuleInput{
		Name:           name,
		AstroCondition: r.AstroCondition,
		ProductTags:    productTags,
		Priority:       priority,
		IsActive:       isActive,
	}, nil
}

// parseRulesListOptions - парсинг параметров запроса.
func parseRulesListOptions(r *http.Request) (rules.ListOptions, error) {
	query := r.URL.Query()

	pageSize := defaultRulesLimit
	if rawLimit := strings.TrimSpace(query.Get("page_size")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > maxRulesLimit {
			return rules.ListOptions{}, errors.New("page_size (limit) must be an integer between 1 and 200")
		}
		pageSize = parsedLimit
	}

	page := 1
	if rawOffset := strings.TrimSpace(query.Get("page")); rawOffset != "" {
		parsedOffset, err := strconv.Atoi(rawOffset)
		if err != nil || parsedOffset < 0 {
			return rules.ListOptions{}, errors.New("page (offset) must be an integer greater than or equal to 0")
		}
		page = parsedOffset
	}

	var isActiveFilter *bool
	if rawActive := strings.TrimSpace(query.Get("is_active")); rawActive != "" {
		parsedActive, err := strconv.ParseBool(rawActive)
		if err != nil {
			return rules.ListOptions{}, errors.New("is_active must be true or false")
		}
		isActiveFilter = &parsedActive
	} else {
		dfltIsActive := true
		isActiveFilter = &dfltIsActive
	}

	return rules.ListOptions{
		IsActive: isActiveFilter,
		PageSize: pageSize,
		Page:     page,
	}, nil
}

// normalizeTags приводит теги к lowercase, убирает пробелы и дубли.
// Гарантирует совпадение тегов из правил с тегами товаров из RetailCRM.
func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimFunc(strings.ToLower(t), unicode.IsSpace)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			result = append(result, t)
		}
	}
	return result
}
