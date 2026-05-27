package astro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"astroapi/config"
	"astroapi/internal/resilience"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const (
	defaultNatalChartCacheBucket = "natal_charts"
	defaultNatalChartCacheTTL    = 24 * time.Hour
)

type cacheBucket interface {
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Put(ctx context.Context, key string, value []byte) (uint64, error)
}

type cacheManager interface {
	GetOrCreateBucket(ctx context.Context, bucket string, ttl time.Duration) (cacheBucket, error)
}

type jetStreamCacheManager struct {
	manager jetstream.KeyValueManager
}

func (m jetStreamCacheManager) GetOrCreateBucket(ctx context.Context, bucket string, ttl time.Duration) (cacheBucket, error) {
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

// Client is the application-facing AstroProvider implementation.
// It wraps a concrete provider with cache, circuit breaker and clear errors.
type Client struct {
	provider    AstroProvider
	cache       cacheManager
	cacheBucket string
	cacheTTL    time.Duration
	breaker     *resilience.CircuitBreaker
	logger      *zap.Logger
}

type ClientOptions struct {
	Provider     AstroProvider
	ProviderName string
	CacheBucket  string
	CacheTTL     time.Duration
	Metrics      resilience.StateReporter
	Breaker      *resilience.CircuitBreaker
}

func NewClient(provider AstroProvider, js jetstream.KeyValueManager, logger *zap.Logger, opts ClientOptions) *Client {
	cacheBucket := opts.CacheBucket
	if cacheBucket == "" {
		cacheBucket = defaultNatalChartCacheBucket
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultNatalChartCacheTTL
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if opts.Provider != nil {
		provider = opts.Provider
	}
	var cache cacheManager
	if js != nil {
		cache = jetStreamCacheManager{manager: js}
	}
	breaker := opts.Breaker
	if breaker == nil {
		providerName := NormalizeProviderName(opts.ProviderName)
		breaker = resilience.NewCircuitBreaker(providerName, 5, 30*time.Second, logger, opts.Metrics)
	}
	return &Client{
		provider:    provider,
		cache:       cache,
		cacheBucket: cacheBucket,
		cacheTTL:    cacheTTL,
		breaker:     breaker,
		logger:      logger,
	}
}

func NewClientFromConfig(
	cfg *config.Config,
	js jetstream.KeyValueManager,
	logger *zap.Logger,
	metricsReporter resilience.StateReporter,
) (*Client, error) {
	provider, providerName, err := NewProviderFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	client := NewClient(provider, js, logger, ClientOptions{
		Metrics:      metricsReporter,
		ProviderName: providerName,
	})
	return client, nil
}

func (c *Client) GetNatalChart(ctx context.Context, dob DateOfBirth, lat float64, lon float64) (NatalData, error) {
	var data NatalData
	if c == nil || c.provider == nil {
		return data, errors.New("astro provider is not configured")
	}
	cacheKey := BuildNatalChartCacheKey(dob, lat, lon)
	if c.cache != nil {
		cached, found, err := c.getCachedNatalChart(ctx, cacheKey)
		if err != nil {
			return data, err
		}
		if found {
			return cached, nil
		}
	}
	err := c.breaker.Execute(func() error {
		var providerErr error
		data, providerErr = c.provider.GetNatalChart(ctx, dob, lat, lon)
		return providerErr
	})
	if err != nil {
		return data, fmt.Errorf("get natal chart from astro provider: %w", err)
	}
	if c.cache != nil {
		if err := c.cacheNatalChart(ctx, cacheKey, data); err != nil {
			return data, err
		}
	}
	return data, nil
}

func (c *Client) getCachedNatalChart(ctx context.Context, cacheKey string) (NatalData, bool, error) {
	var data NatalData
	bucket, err := c.cache.GetOrCreateBucket(ctx, c.cacheBucket, c.cacheTTL)
	if err != nil {
		return data, false, fmt.Errorf("open natal chart KV bucket: %w", err)
	}
	entry, err := bucket.Get(ctx, cacheKey)
	if err == nil {
		if err = json.Unmarshal(entry.Value(), &data); err != nil {
			return data, false, fmt.Errorf("decode cached natal chart: %w", err)
		}
		c.logger.Debug("natal chart cache hit", zap.String("key", cacheKey))
		return data, true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		c.logger.Debug("natal chart cache miss", zap.String("key", cacheKey))
		return data, false, nil
	}
	return data, false, fmt.Errorf("read natal chart from KV: %w", err)
}

func (c *Client) cacheNatalChart(ctx context.Context, cacheKey string, data NatalData) error {
	bucket, err := c.cache.GetOrCreateBucket(ctx, c.cacheBucket, c.cacheTTL)
	if err != nil {
		return fmt.Errorf("open natal chart KV bucket: %w", err)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal natal chart for cache: %w", err)
	}
	_, err = bucket.Put(ctx, cacheKey, payload)
	if err != nil {
		return fmt.Errorf("cache natal chart in KV: %w", err)
	}
	return nil
}
