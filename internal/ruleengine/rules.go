package ruleengine

import (
	"context"
	"time"
)

type Rule struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	AstroCondition map[string]any `json:"astro_condition"`
	ProductTags    []string       `json:"product_tags"`
	Priority       int            `json:"priority"`
	IsActive       bool           `json:"is_active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type RuleInput struct {
	Name           string
	AstroCondition map[string]any
	ProductTags    []string
	Priority       int
	IsActive       bool
}

type ListOptions struct {
	IsActive *bool
	Limit    int
	Offset   int
}

type ListResult struct {
	Items      []Rule
	TotalCount int
}

type Repository interface {
	List(ctx context.Context, options ListOptions) (ListResult, error)
	Create(ctx context.Context, input RuleInput) (Rule, error)
	Update(ctx context.Context, id string, input RuleInput) (Rule, error)
	Deactivate(ctx context.Context, id string) (Rule, error)
	Match(ctx context.Context, triggers []string) ([]string, error)
}
