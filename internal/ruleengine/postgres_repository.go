package ruleengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

var (
	ErrRuleNotFound = errors.New("astro rule not found")
)

type PostgresRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewPostgresRepository(db *sql.DB, logger *zap.Logger) *PostgresRepository {
	return &PostgresRepository{db: db, logger: logger}
}

func (r *PostgresRepository) List(ctx context.Context, options ListOptions) (ListResult, error) {
	conditions := make([]string, 0, 1)
	queryArgs := make([]any, 0, 3)

	if options.IsActive != nil {
		queryArgs = append(queryArgs, *options.IsActive)
		conditions = append(conditions, "is_active = $1")
	}

	whereClause := normalizeQuery(conditions)

	var totalCount int
	countQuery := "SELECT COUNT(*) FROM astro_rules" + whereClause
	if err := r.db.QueryRowContext(ctx, countQuery, queryArgs...).Scan(&totalCount); err != nil {
		return ListResult{}, fmt.Errorf("count astro rules: %w", err)
	}

	limitArgIndex := len(queryArgs) + 1
	offsetArgIndex := len(queryArgs) + 2
	queryArgs = append(queryArgs, options.Limit, options.Offset)

	selectQuery := buildListQuery(whereClause, limitArgIndex, offsetArgIndex)

	rows, err := r.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list astro rules: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.Warn("failed to close astro rules rows", zap.Error(closeErr))
		}
	}()

	ruleItems := make([]Rule, 0)
	for rows.Next() {
		ruleItem, scanErr := scanRule(rows)
		if scanErr != nil {
			return ListResult{}, scanErr
		}
		ruleItems = append(ruleItems, ruleItem)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate astro rules: %w", err)
	}

	return ListResult{
		Items:      ruleItems,
		TotalCount: totalCount,
	}, nil
}

func (r *PostgresRepository) Create(ctx context.Context, input RuleInput) (Rule, error) {
	conditionJSON, tagsJSON, err := marshalRulePayload(input)
	if err != nil {
		return Rule{}, err
	}

	row := r.db.QueryRowContext(
		ctx,
		`
			INSERT INTO astro_rules (name, astro_condition, product_tags, priority, is_active)
			VALUES ($1, $2::jsonb, $3::jsonb, $4, $5)
			RETURNING id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
		`,
		input.Name,
		conditionJSON,
		tagsJSON,
		input.Priority,
		input.IsActive,
	)

	ruleItem, scanErr := scanRule(row)
	if scanErr != nil {
		return Rule{}, scanErr
	}

	return ruleItem, nil
}

func (r *PostgresRepository) Update(ctx context.Context, id string, input RuleInput) (Rule, error) {
	conditionJSON, tagsJSON, err := marshalRulePayload(input)
	if err != nil {
		return Rule{}, err
	}

	row := r.db.QueryRowContext(
		ctx,
		`
			UPDATE astro_rules
			SET name = $2,
			    astro_condition = $3::jsonb,
			    product_tags = $4::jsonb,
			    priority = $5,
			    is_active = $6,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
			RETURNING id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
		`,
		id,
		input.Name,
		conditionJSON,
		tagsJSON,
		input.Priority,
		input.IsActive,
	)

	ruleItem, scanErr := scanRule(row)
	if scanErr != nil {
		return Rule{}, scanErr
	}

	return ruleItem, nil
}

func (r *PostgresRepository) Deactivate(ctx context.Context, id string) (Rule, error) {
	row := r.db.QueryRowContext(
		ctx,
		`
			UPDATE astro_rules
			SET is_active = FALSE,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
			RETURNING id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
		`,
		id,
	)

	ruleItem, scanErr := scanRule(row)
	if scanErr != nil {
		return Rule{}, scanErr
	}

	return ruleItem, nil
}

func marshalRulePayload(input RuleInput) ([]byte, []byte, error) {
	conditionJSON, err := json.Marshal(input.AstroCondition)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal astro condition: %w", err)
	}

	tagsJSON, err := json.Marshal(input.ProductTags)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal product tags: %w", err)
	}

	return conditionJSON, tagsJSON, nil
}

func scanRule(scanner rowScanner) (Rule, error) {
	ruleItem := Rule{}
	var conditionJSON []byte
	var tagsJSON []byte

	if err := scanner.Scan(
		&ruleItem.ID,
		&ruleItem.Name,
		&conditionJSON,
		&tagsJSON,
		&ruleItem.Priority,
		&ruleItem.IsActive,
		&ruleItem.CreatedAt,
		&ruleItem.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, fmt.Errorf("scan astro rule: %w", err)
	}

	if err := json.Unmarshal(conditionJSON, &ruleItem.AstroCondition); err != nil {
		return Rule{}, fmt.Errorf("decode astro condition: %w", err)
	}

	if len(ruleItem.AstroCondition) == 0 {
		ruleItem.AstroCondition = map[string]any{}
	}

	if err := json.Unmarshal(tagsJSON, &ruleItem.ProductTags); err != nil {
		return Rule{}, fmt.Errorf("decode product tags: %w", err)
	}

	if ruleItem.ProductTags == nil {
		ruleItem.ProductTags = []string{}
	}

	return ruleItem, nil
}

func normalizeQuery(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conditions, " AND ")
}

func buildListQuery(whereClause string, limitArgIndex int, offsetArgIndex int) string {
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
		SELECT id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
		FROM astro_rules`)
	queryBuilder.WriteString(whereClause)
	queryBuilder.WriteString(`
		ORDER BY priority ASC, created_at DESC
		LIMIT $`)
	fmt.Fprintf(&queryBuilder, "%d", limitArgIndex)
	queryBuilder.WriteString(` OFFSET $`)
	fmt.Fprintf(&queryBuilder, "%d", offsetArgIndex)

	return queryBuilder.String()
}
