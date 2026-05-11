-- +goose Up

-- Переводим все TIMESTAMP → TIMESTAMPTZ для корректной работы с таймзонами
ALTER TABLE products
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE astro_rules
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE requests_log
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

-- CHECK constraint: защищает от попадания мусора в status
ALTER TABLE requests_log
    ADD CONSTRAINT chk_requests_log_status
    CHECK (status IN ('accepted', 'processing', 'completed', 'failed'));

-- CHECK constraint: scenario
ALTER TABLE requests_log
    ADD CONSTRAINT chk_requests_log_scenario
    CHECK (scenario IN ('personal_style', 'perfect_gift', 'profile'));

-- +goose Down
ALTER TABLE requests_log
    DROP CONSTRAINT IF EXISTS chk_requests_log_status,
    DROP CONSTRAINT IF EXISTS chk_requests_log_scenario;

ALTER TABLE products
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE astro_rules
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE requests_log
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';
