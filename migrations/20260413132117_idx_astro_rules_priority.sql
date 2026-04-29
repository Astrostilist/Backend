-- +goose Up
CREATE INDEX IF NOT EXISTS idx_astro_rules_priority
    ON astro_rules (is_active, priority);

-- +goose Down
DROP INDEX IF EXISTS idx_astro_rules_priority;
