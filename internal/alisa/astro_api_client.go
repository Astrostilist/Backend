package alisa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"astroapi/config"
	"astroapi/internal/resilience"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const (
	defaultAstroProfileCacheBucket = "astro_profiles"
	defaultAstroProfileCacheTTL    = 24 * time.Hour
	defaultAstroProfileTimeout     = 10 * time.Second
)

type astroCacheBucket interface {
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Put(ctx context.Context, key string, value []byte) (uint64, error)
}

type astroCacheManager interface {
	GetOrCreateBucket(ctx context.Context, bucket string, ttl time.Duration) (astroCacheBucket, error)
}

type jetStreamAstroCacheManager struct {
	manager jetstream.KeyValueManager
}

func (m jetStreamAstroCacheManager) GetOrCreateBucket(ctx context.Context, bucket string, ttl time.Duration) (astroCacheBucket, error) {
	kv, err := m.manager.KeyValue(ctx, bucket)
	if err == nil {
		return kv, nil
	}
	if !errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil, err
	}

	kv, err = m.manager.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  bucket,
		History: 1,
		TTL:     ttl,
	})
	if err != nil {
		return nil, err
	}

	return kv, nil
}

type AstroAPIClient struct {
	baseURL     string
	httpClient  *http.Client
	cache       astroCacheManager
	logger      *zap.Logger
	cacheBucket string
	cacheTTL    time.Duration
	breaker     *resilience.CircuitBreaker
}

type AstroAPIClientOptions struct {
	HTTPClient  *http.Client
	CacheBucket string
	CacheTTL    time.Duration
	Metrics     *resilience.Registry
	Breaker     *resilience.CircuitBreaker
}

type astroProfileEnvelope struct {
	Profile AstroProfile `json:"profile"`
	Data    AstroProfile `json:"data"`
}

func NewAstroAPIClient(baseURL string, js jetstream.KeyValueManager, logger *zap.Logger, opts AstroAPIClientOptions) *AstroAPIClient {
	cacheBucket := opts.CacheBucket
	if cacheBucket == "" {
		cacheBucket = defaultAstroProfileCacheBucket
	}

	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultAstroProfileCacheTTL
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultAstroProfileTimeout}
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	var cache astroCacheManager
	if js != nil {
		cache = jetStreamAstroCacheManager{manager: js}
	}

	breaker := opts.Breaker
	if breaker == nil {
		breaker = resilience.NewCircuitBreaker("astro_api", 5, 30*time.Second, logger, opts.Metrics)
	}

	return &AstroAPIClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  httpClient,
		cache:       cache,
		logger:      logger,
		cacheBucket: cacheBucket,
		cacheTTL:    cacheTTL,
		breaker:     breaker,
	}
}

func NewAstroAPIClientFromConfig(cfg *config.Config, js jetstream.KeyValueManager, logger *zap.Logger) *AstroAPIClient {
	baseURL := ""
	if cfg != nil {
		baseURL = cfg.AstroAPIURL
	}
	return NewAstroAPIClient(baseURL, js, logger, AstroAPIClientOptions{})
}

func NewAstroAPIClientFromEnv(js jetstream.KeyValueManager, logger *zap.Logger) *AstroAPIClient {
	return NewAstroAPIClient(os.Getenv("ASTRO_API_URL"), js, logger, AstroAPIClientOptions{})
}

func (c *AstroAPIClient) GetAstroProfile(birthDate, birthPlace string) (AstroProfile, error) {
	var profile AstroProfile

	if strings.TrimSpace(c.baseURL) == "" {
		return profile, errors.New("ASTRO_API_URL is not configured")
	}

	timeout := c.httpClient.Timeout
	if timeout <= 0 {
		timeout = defaultAstroProfileTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cacheKey := buildAstroProfileCacheKey(birthDate, birthPlace)

	if c.cache != nil {
		cachedProfile, found, err := c.getCachedProfile(ctx, cacheKey)
		if err != nil {
			return profile, err
		}
		if found {
			return cachedProfile, nil
		}
	}

	err := c.breaker.Execute(func() error {
		var fetchErr error
		profile, fetchErr = c.fetchProfile(ctx, birthDate, birthPlace)
		return fetchErr
	})
	if err != nil {
		return profile, err
	}

	if c.cache != nil {
		if err := c.cacheProfile(ctx, cacheKey, profile); err != nil {
			return profile, err
		}
	}

	return profile, nil
}

func (c *AstroAPIClient) getCachedProfile(ctx context.Context, cacheKey string) (AstroProfile, bool, error) {
	var profile AstroProfile

	bucket, err := c.cache.GetOrCreateBucket(ctx, c.cacheBucket, c.cacheTTL)
	if err != nil {
		return profile, false, fmt.Errorf("open astro profile KV bucket: %w", err)
	}

	entry, err := bucket.Get(ctx, cacheKey)
	if err == nil {
		if err = json.Unmarshal(entry.Value(), &profile); err != nil {
			return profile, false, fmt.Errorf("decode cached astro profile: %w", err)
		}
		c.logger.Debug("astro profile cache hit", zap.String("key", cacheKey))
		return profile, true, nil
	}

	if errors.Is(err, jetstream.ErrKeyNotFound) {
		c.logger.Debug("astro profile cache miss", zap.String("key", cacheKey))
		return profile, false, nil
	}

	return profile, false, fmt.Errorf("read astro profile from KV: %w", err)
}

func (c *AstroAPIClient) cacheProfile(ctx context.Context, cacheKey string, profile AstroProfile) error {
	bucket, err := c.cache.GetOrCreateBucket(ctx, c.cacheBucket, c.cacheTTL)
	if err != nil {
		return fmt.Errorf("open astro profile KV bucket: %w", err)
	}

	payload, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal astro profile for cache: %w", err)
	}

	_, err = bucket.Put(ctx, cacheKey, payload)
	if err != nil {
		return fmt.Errorf("cache astro profile in KV: %w", err)
	}

	return nil
}

func (c *AstroAPIClient) fetchProfile(ctx context.Context, birthDate, birthPlace string) (AstroProfile, error) {
	var profile AstroProfile

	requestURL, err := buildAstroProfileURL(c.baseURL, birthDate, birthPlace)
	if err != nil {
		return profile, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return profile, fmt.Errorf("create astro API request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return profile, fmt.Errorf("send astro API request: %w", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			c.logger.Error("failed to close response body", zap.Error(err))
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return profile, fmt.Errorf("read astro API response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return profile, fmt.Errorf("astro API request failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	profile, err = decodeAstroProfile(body)
	if err != nil {
		return profile, err
	}

	if profile.BirthDate == "" {
		profile.BirthDate = birthDate
	}
	if profile.BirthPlace == "" {
		profile.BirthPlace = birthPlace
	}

	return profile, nil
}

func buildAstroProfileURL(baseURL, birthDate, birthPlace string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse ASTRO_API_URL: %w", err)
	}

	query := parsedURL.Query()
	query.Set("birth_date", birthDate)
	query.Set("birth_place", birthPlace)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}

func decodeAstroProfile(payload []byte) (AstroProfile, error) {
	var directProfile AstroProfile
	if err := json.Unmarshal(payload, &directProfile); err == nil && !directProfile.IsZero() {
		return directProfile, nil
	}

	var envelope astroProfileEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return AstroProfile{}, fmt.Errorf("decode astro profile response: %w", err)
	}

	if !envelope.Profile.IsZero() {
		return envelope.Profile, nil
	}
	if !envelope.Data.IsZero() {
		return envelope.Data, nil
	}

	return AstroProfile{}, errors.New("astro profile response is empty")
}

func buildAstroProfileCacheKey(birthDate, birthPlace string) string {
	return fmt.Sprintf("astro_profile:%s:%s", birthDate, birthPlace)
}
