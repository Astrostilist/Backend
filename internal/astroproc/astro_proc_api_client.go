package astroprocapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	fakemetrics "astroapi/internal/metrics"
	"astroapi/internal/resilience"

	"go.uber.org/zap"
)

const (
	defaultAstroProfileCacheBucket = "astro_profiles"
	defaultAstroProfileTimeout     = 10 * time.Second
)

type AstroProcClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	breaker    *resilience.CircuitBreaker
	cache      *sql.DB
}

type AstroProfile struct {
	ID        string      `json:"id"`
	UserID    string      `json:"user_id"`
	Profile   ProfileData `json:"profile"`
	CreatedAt time.Time   `json:"created_at"`
}

type ProfileData struct {
	ZodiacSign      string `json:"zodiac_sign"`
	VenusPosition   string `json:"venus_position"`
	MarsPosition    string `json:"mars_position"`
	DominantElement string `json:"dominant_element"`
}

func NewAstroAPIClient(baseURL string, db *sql.DB, logger *zap.Logger) *AstroProcClient {

	httpClient := &http.Client{Timeout: defaultAstroProfileTimeout}
	breaker := resilience.NewCircuitBreaker(
		"astro_api",
		5,
		30*time.Second,
		logger, fakemetrics.CircuitBreakerReporter{})

	return &AstroProcClient{baseURL: baseURL,
		logger:     logger,
		httpClient: httpClient,
		breaker:    breaker,
		cache:      db}

}

func (c *AstroProcClient) GetAstroProfile(ctx context.Context, userID string) (AstroProfile, error) {
	var profile AstroProfile

	if strings.TrimSpace(c.baseURL) == "" {
		return profile, errors.New("ASTRO_API_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultAstroProfileTimeout)
	defer cancel()

	query := `
		SELECT id, user_id, profile, created_at 
		FROM astro_profiles_temp 
		WHERE user_id = $1
	`

	var rawProfile []byte
	row := c.cache.QueryRowContext(ctx, query, userID)
	err := row.Scan(&profile.ID, &profile.UserID, &rawProfile, &profile.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return profile, fmt.Errorf("astro profile not found for user %s", userID)
		}
		return profile, fmt.Errorf("failed to get astro profile: %w", err)
	}

	if err := json.Unmarshal(rawProfile, &profile.Profile); err != nil {
		return profile, fmt.Errorf("failed to unmarshal profile: %w", err)
	}

	return profile, nil
}
