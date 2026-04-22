package ruleengine

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AstroCondition struct {
	Planet string `json:"planet"`
	Sign   string `json:"sign"`
}

type Rule struct {
	ID             uuid.UUID        `json:"id"`
	Name           string           `json:"name"`
	AstroCondition []AstroCondition `json:"astro_condition"`
	ProductTags    []string         `json:"product_tags"`
	Priority       int              `json:"priority"`
	IsActive       bool             `json:"is_active"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type RuleInput struct {
	Name           string
	AstroCondition []AstroCondition
	ProductTags    []string
	Priority       int
	IsActive       bool
}

type ListOptions struct {
	IsActive *bool
	Limit    int // page
	Offset   int // pageSize
}

type ListResult struct {
	Items      []Rule
	TotalCount int
}

type Repository interface {
	Create(ctx context.Context, input *RuleInput) (*Rule, error)
	Get(id string) (*Rule, error)
	Update(ctx context.Context, id string, input *RuleInput) (*Rule, error)
	Delete(id string) error
	List(ctx context.Context, options ListOptions) ([]Rule, Metadata, error)
	Deactivate(ctx context.Context, id string) (*Rule, error)
	Match(ctx context.Context, triggers []string) ([]string, error)
}

type Metadata struct {
	CurrentPage  int `json:"current_page,omitzero"`
	PageSize     int `json:"page_size,omitzero"`
	FirstPage    int `json:"first_page,omitzero"`
	LastPage     int `json:"last_page,omitzero"`
	TotalRecords int `json:"total_records,omitzero"`
}

// calculateMetadata - формирует данные для структруры Metadata
func calculateMetadata(totalRecords, page, pageSize int) Metadata {
	if totalRecords == 0 {
		return Metadata{}
	}

	return Metadata{
		CurrentPage:  page,
		PageSize:     pageSize,
		FirstPage:    1,
		LastPage:     (totalRecords + pageSize - 1) / pageSize,
		TotalRecords: totalRecords,
	}
}

func (f ListOptions) limit() int {
	return f.Limit
}

func (f ListOptions) offset() int {
	return (f.Offset - 1) * f.Limit
}
