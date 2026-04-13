package ruleengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/lib/pq"
)

// RulesModel - инкапсулирует пул соединений БД.
type RulesModel struct {
	DB *sql.DB
}

// Match - возвращает уникальный список тегов по активным правилам, отсортированным по priority ASC
func (r RulesModel) Match(triggers []string) ([]string, error) {
	if len(triggers) == 0 {
		return []string{}, nil
	}

	is_active := true

	query := `
	SELECT product_tags, priority
	FROM astro_rules 
	WHERE is_active=$1 AND name = any($2)
	ORDER BY priority ASC
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	uniqueTags := make([]string, 0)
	tags := make(map[string]int)

	rows, err := r.DB.QueryContext(ctx, query, is_active, pq.Array(triggers))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	for rows.Next() {
		var tmp []byte
		var tmpJson []string
		var p int
		err = rows.Scan(&tmp, &p)
		if err != nil {
			return nil, err
		}

		err := json.Unmarshal(tmp, &tmpJson)
		if err != nil {
			return nil, err
		}

		for _, val := range tmpJson {

			if _, ok := tags[val]; !ok {

				uniqueTags = append(uniqueTags, val)
				tags[val] += 1
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return uniqueTags, nil
}
