package ruleengine

import (
	"context"
	"encoding/json"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

// Match - возвращает уникальный список тегов по активным правилам, отсортированным по priority ASC
func (r *PostgresRepository) Match(ctx context.Context, triggers []string) ([]string, error) {
	if len(triggers) == 0 {
		return []string{}, nil
	}

	isActive := true

	query := `
	SELECT product_tags, priority
	FROM astro_rules 
	WHERE is_active=$1 AND name = any($2)
	ORDER BY priority ASC
	`

	uniqueTags := make([]string, 0)
	tags := make(map[string]int)

	rows, err := r.db.QueryContext(ctx, query, isActive, pq.Array(triggers))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = rows.Close(); err != nil {
			r.logger.Warn("failed to close match rows", zap.Error(err))
		}
	}()

	for rows.Next() {
		var tmp []byte
		var tmpJSON []string
		var p int
		if err := rows.Scan(&tmp, &p); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tmp, &tmpJSON); err != nil {
			return nil, err
		}
		for _, val := range tmpJSON {
			if _, ok := tags[val]; !ok {
				uniqueTags = append(uniqueTags, val)
				tags[val]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return uniqueTags, nil
}
