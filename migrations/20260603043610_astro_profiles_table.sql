-- +goose Up
CREATE TABLE astro_profiles (
  id UUID PRIMARY KEY,
  user_id VARCHAR(255) NOT NULL, -- внешний ID из бота + у нас он же в табл users
  profile_hash VARCHAR(64) NOT NULL, -- SHA256(dob+user_id) для дедупликации (либо другой способ)
  dob_encrypted BYTEA, -- шифруется только если consent=true
  consent_given BOOLEAN DEFAULT false,
  profile_data JSONB NOT NULL, -- {sun_sign, venus_sign, ...}
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(user_id, profile_hash)
);

CREATE INDEX idx_astro_profiles_hash ON astro_profiles(user_id, profile_hash);;

-- +goose Down
DROP TABLE IF EXISTS astro_profiles;
