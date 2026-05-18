package ruleengine

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Rule struct {
	ID             uuid.UUID         `json:"id,omitzero"`
	Name           string            `json:"name,omitzero"`
	AstroCondition map[string]string `json:"astro_condition,omitzero"`
	ProductTags    []string          `json:"product_tags,omitzero"`
	Priority       int               `json:"priority,omitzero"`
	IsActive       bool              `json:"is_active,omitzero"`
	CreatedAt      time.Time         `json:"created_at,omitzero"`
	UpdatedAt      time.Time         `json:"updated_at,omitzero"`
}

type RuleInput struct {
	Name           string
	AstroCondition map[string]string
	ProductTags    []string
	Priority       int
	IsActive       bool
}

type ListOptions struct {
	IsActive *bool
	PageSize int // pageSize
	Page     int // page
}

type ListResult struct {
	Items      []Rule
	TotalCount int
}
type Repository interface {
	Create(ctx context.Context, input *RuleInput) (uuid.UUID, error)
	Get(ctx context.Context, id string) (*Rule, error)
	Update(ctx context.Context, id string, input *RuleInput) (uuid.UUID, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, options ListOptions) ([]*Rule, Metadata, error)
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
	return f.PageSize
}

func (f ListOptions) offset() int {
	return (f.Page - 1) * f.PageSize
}
