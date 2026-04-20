package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	rules "astroapi/internal/ruleengine"

	"github.com/go-chi/chi/v5"
)

const (
	defaultRulesLimit   = 50
	maxRulesLimit       = 200
	defaultRulePriority = 100
)

type AdminRulesHandler struct {
	repository rules.Repository
}

type adminRuleRequest struct {
	Name           string         `json:"name"`
	AstroCondition map[string]any `json:"astro_condition"`
	ProductTags    []string       `json:"product_tags"`
	Priority       *int           `json:"priority"`
	IsActive       *bool          `json:"is_active"`
}

type adminRulesListResponse struct {
	Items      []rules.Rule `json:"items"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
	TotalCount int          `json:"total_count"`
}

func NewAdminRulesHandler(repository rules.Repository) *AdminRulesHandler {
	return &AdminRulesHandler{repository: repository}
}

func RegisterAdminRulesRoutes(router chi.Router, adminToken string, handler *AdminRulesHandler) {
	router.Route("/api/v1/admin/rules", func(router chi.Router) {
		router.Use(AdminAuthMiddleware(adminToken))
		router.Get("/", handler.ListRules)
		router.Post("/", handler.CreateRule)
		router.Put("/{id}", handler.UpdateRule)
		router.Delete("/{id}", handler.DeleteRule)
	})
}

func (h *AdminRulesHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	listOptions, err := parseRulesListOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.repository.List(r.Context(), listOptions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch astro rules")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Astro rules fetched successfully",
		Data: adminRulesListResponse{
			Items:      result.Items,
			Limit:      listOptions.Limit,
			Offset:     listOptions.Offset,
			TotalCount: result.TotalCount,
		},
	})
}

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

	createdRule, err := h.repository.Create(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create astro rule")
		return
	}

	writeJSON(w, http.StatusCreated, Response{
		Message: "Astro rule created successfully",
		Data:    createdRule,
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

	updatedRule, err := h.repository.Update(r.Context(), ruleID, input)
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
		Data:    updatedRule,
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

	productTags := r.ProductTags
	if productTags == nil {
		productTags = []string{}
	}

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

func parseRulesListOptions(r *http.Request) (rules.ListOptions, error) {
	query := r.URL.Query()

	limit := defaultRulesLimit
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > maxRulesLimit {
			return rules.ListOptions{}, errors.New("limit must be an integer between 1 and 200")
		}
		limit = parsedLimit
	}

	offset := 0
	if rawOffset := strings.TrimSpace(query.Get("offset")); rawOffset != "" {
		parsedOffset, err := strconv.Atoi(rawOffset)
		if err != nil || parsedOffset < 0 {
			return rules.ListOptions{}, errors.New("offset must be an integer greater than or equal to 0")
		}
		offset = parsedOffset
	}

	var isActiveFilter *bool
	if rawActive := strings.TrimSpace(query.Get("is_active")); rawActive != "" {
		parsedActive, err := strconv.ParseBool(rawActive)
		if err != nil {
			return rules.ListOptions{}, errors.New("is_active must be true or false")
		}
		isActiveFilter = &parsedActive
	}

	return rules.ListOptions{
		IsActive: isActiveFilter,
		Limit:    limit,
		Offset:   offset,
	}, nil
}
