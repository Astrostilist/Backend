-- +goose Up
CREATE TABLE astro_profiles_temp (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE,
    profile JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO astro_profiles_temp (id, user_id, profile) VALUES
(
    '123e4567-e89b-12d3-a456-426614174001',
    '123e4567-e89b-12d3-a456-426614174000',
    '{"zodiac_sign": "Taurus", "venus_position": "Aries", "mars_position": "Gemini", "dominant_element": "Earth"}'
),
(
    '123e4567-e89b-12d3-a456-426614174002',
    '123e4567-e89b-12d3-a456-426614174001',
    '{"zodiac_sign": "Scorpio", "venus_position": "Libra", "mars_position": "Leo", "dominant_element": "Water"}'
),
(
    '123e4567-e89b-12d3-a456-426614174003',
    '123e4567-e89b-12d3-a456-426614174002',
    '{"zodiac_sign": "Leo", "venus_position": "Taurus", "mars_position": "Sagittarius", "dominant_element": "Fire"}'
),
(
    '123e4567-e89b-12d3-a456-426614174004',
    '123e4567-e89b-12d3-a456-426614174003',
    '{"zodiac_sign": "Virgo", "venus_position": "Cancer", "mars_position": "Pisces", "dominant_element": "Earth"}'
),
(
    '123e4567-e89b-12d3-a456-426614174005',
    '123e4567-e89b-12d3-a456-426614174004',
    '{"zodiac_sign": "Aquarius", "venus_position": "Capricorn", "mars_position": "Aries", "dominant_element": "Air"}'
);

-- +goose Down
DROP TABLE astro_profiles_temp;