package ruleengine

import (
	"context"
	"encoding/json"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

// Match - возвращает уникальный список тегов по активным правилам, отсортированным по priority ASC
// TODO: мэтчить нужно по astro_condition, а не по name правила
func (r *PostgresRepository) Match(ctx context.Context, triggers []string) ([]string, error) {
	if len(triggers) == 0 {
		return []string{}, nil
	}

	// TODO:  проверить поиск - заменила name на astro_condition
	query := `
	SELECT product_tags, priority
	FROM astro_rules 
	WHERE is_active AND astro_condition = any($1)
	ORDER BY priority ASC
	`

	uniqueTags := make([]string, 0)
	tags := make(map[string]int)

	rows, err := r.db.QueryContext(ctx, query, pq.Array(triggers))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = rows.Close(); err != nil {
			r.logger.Error("failed to close match rows", zap.Error(err))
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
