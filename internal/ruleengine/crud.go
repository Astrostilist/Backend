package ruleengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRuleNotFound = errors.New("record not found")
)

var (
	uuidRegex = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
)

type PostgresRepository struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Create - создание правила в БД.
func (r *PostgresRepository) Create(ctx context.Context, input *RuleInput) (uuid.UUID, error) {
	conditionJSON, tagsJSON, err := marshalRulePayload(*input)
	if err != nil {
		return uuid.Nil, err
	}
	quary := `
			INSERT INTO astro_rules (name, astro_condition, product_tags, priority, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`

	var id string
	err = r.db.QueryRowContext(
		ctx,
		quary,
		input.Name,
		conditionJSON,
		tagsJSON,
		input.Priority,
		input.IsActive,
	).Scan(&id)

	// ruleItem, scanErr := scanRule(row)
	// if scanErr != nil {
	// 	return nil, scanErr
	// }

	if err != nil {
		return uuid.Nil, err
	}

	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, err
	}

	return parsedUUID, nil
}

// Get - получить конкретную запись из БД.
func (r *PostgresRepository) Get(id string) (*Rule, error) {
	if !Matches(id, uuidRegex) {
		return nil, ErrRuleNotFound
	}

	query := `
		SELECT id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
		FROM astro_rules
		WHERE id = $1`

	var rule Rule
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	var tmpCndtn, tmpTags []byte

	err = r.db.QueryRowContext(ctx, query, ID).Scan(
		&rule.ID,
		&rule.Name,
		&tmpCndtn,
		&tmpTags,
		&rule.Priority,
		&rule.IsActive,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRuleNotFound
		default:
			return nil, err
		}
	}

	err = json.Unmarshal(tmpCndtn, &rule.AstroCondition)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(tmpTags, &rule.ProductTags)
	if err != nil {
		return nil, err
	}

	return &rule, nil
}

// Update - метод обновляет конкретную запись в БД.
func (r *PostgresRepository) Update(ctx context.Context, id string, input *RuleInput) (*Rule, error) {
	if !Matches(id, uuidRegex) {
		return nil, ErrRuleNotFound
	}

	conditionJSON, tagsJSON, err := marshalRulePayload(*input)
	if err != nil {
		return nil, err
	}
	quary := `
			UPDATE astro_rules
			SET name = $2,
			    astro_condition = $3::jsonb,
			    product_tags = $4::jsonb,
			    priority = $5,
			    is_active = $6,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
			RETURNING id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
		`
	ID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(
		ctx,
		quary,
		ID,
		input.Name,
		conditionJSON,
		tagsJSON,
		input.Priority,
		input.IsActive,
	)

	ruleItem, scanErr := scanRule(row)
	if scanErr != nil {
		return nil, scanErr
	}

	return &ruleItem, nil
}

// Delete - метод удаляет определнную запись в БД.
func (r *PostgresRepository) Delete(id string) error {
	if !Matches(id, uuidRegex) {
		return ErrRuleNotFound
	}

	ID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	if ID == uuid.Nil {
		return ErrRuleNotFound
	}

	query := `
		DELETE FROM astro_rules
		WHERE id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := r.db.ExecContext(ctx, query, ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	// если кол-во возвращенных строк 0, значит ничего не удалили
	if rowsAffected == 0 {
		return ErrRuleNotFound
	}

	return nil
}

// List -  получить список правил. GET /api/v1/admin/rules?page=1&page_size=5
func (r *PostgresRepository) List(ctx context.Context, options ListOptions) ([]Rule, Metadata, error) {
	query := fmt.Sprintf(`
	SELECT COUNT(*) OVER(), id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
	FROM  astro_rules
	WHERE is_active = %v
	ORDER BY name ASC
	LIMIT $1 OFFSET $2`, *options.IsActive)

	args := []any{options.limit(), options.offset()}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	totalRecords := 0

	rules := []Rule{}

	for rows.Next() {
		var rule Rule

		var tmpCndtn, tmpTags []byte
		err := rows.Scan(
			&totalRecords,
			&rule.ID,
			&rule.Name,
			&tmpCndtn,
			&tmpTags,
			&rule.Priority,
			&rule.IsActive,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		err = json.Unmarshal(tmpCndtn, &rule.AstroCondition)
		if err != nil {
			return nil, Metadata{}, err
		}
		err = json.Unmarshal(tmpTags, &rule.ProductTags)
		if err != nil {
			return nil, Metadata{}, err
		}

		rules = append(rules, rule)
	}

	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, options.Limit, options.Offset)
	return rules, metadata, nil
}

// Deactivate - отключить правило.
func (r *PostgresRepository) Deactivate(ctx context.Context, id string) (*Rule, error) {
	if !Matches(id, uuidRegex) {
		return nil, ErrRuleNotFound
	}
	ID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(
		ctx,
		`
			UPDATE astro_rules
			SET is_active = FALSE,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
			RETURNING id, name, astro_condition, product_tags, priority, is_active, created_at, updated_at
		`,
		ID,
	)

	ruleItem, scanErr := scanRule(row)
	if scanErr != nil {
		return nil, scanErr
	}

	return &ruleItem, nil
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

	// ?
	if len(ruleItem.AstroCondition) == 0 {
		ruleItem.AstroCondition = []AstroCondition{}
	}

	if err := json.Unmarshal(tagsJSON, &ruleItem.ProductTags); err != nil {
		return Rule{}, fmt.Errorf("decode product tags: %w", err)
	}

	if ruleItem.ProductTags == nil {
		ruleItem.ProductTags = []string{}
	}

	return ruleItem, nil
}

// Matches - возвращает true, если строка соотв-ет определенному шаблону.
func Matches(value string, rx *regexp.Regexp) bool {
	return rx.MatchString(value)
}
