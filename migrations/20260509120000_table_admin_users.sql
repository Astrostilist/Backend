-- +goose Up
CREATE TABLE IF NOT EXISTS admin_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL,
    role VARCHAR(50) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_email_lower
    ON admin_users (LOWER(email));

CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_single_super_admin
    ON admin_users (role)
    WHERE role = 'super_admin';

-- +goose Down
DROP INDEX IF EXISTS idx_admin_users_single_super_admin;
DROP INDEX IF EXISTS idx_admin_users_email_lower;
DROP TABLE IF EXISTS admin_users;
