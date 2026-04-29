package alisa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type fakeAstroCacheManager struct {
	mu      sync.Mutex
	buckets map[string]*fakeAstroCacheBucket
}

type fakeAstroCacheBucket struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]fakeAstroCacheRecord
}

type fakeAstroCacheRecord struct {
	value     []byte
	expiresAt time.Time
}

type fakeAstroCacheEntry struct {
	value []byte
}

func (e fakeAstroCacheEntry) Bucket() string                  { return "" }
func (e fakeAstroCacheEntry) Key() string                     { return "" }
func (e fakeAstroCacheEntry) Value() []byte                   { return e.value }
func (e fakeAstroCacheEntry) Revision() uint64                { return 1 }
func (e fakeAstroCacheEntry) Created() time.Time              { return time.Now() }
func (e fakeAstroCacheEntry) Delta() uint64                   { return 0 }
func (e fakeAstroCacheEntry) Operation() jetstream.KeyValueOp { return 0 }

func newFakeAstroCacheManager() *fakeAstroCacheManager {
	return &fakeAstroCacheManager{buckets: make(map[string]*fakeAstroCacheBucket)}
}

func (m *fakeAstroCacheManager) GetOrCreateBucket(_ context.Context, bucket string, ttl time.Duration) (astroCacheBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.buckets[bucket]; ok {
		return existing, nil
	}

	created := &fakeAstroCacheBucket{
		ttl:     ttl,
		entries: make(map[string]fakeAstroCacheRecord),
	}
	m.buckets[bucket] = created
	return created, nil
}

func (b *fakeAstroCacheBucket) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	record, ok := b.entries[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}

	if !record.expiresAt.IsZero() && time.Now().After(record.expiresAt) {
		delete(b.entries, key)
		return nil, jetstream.ErrKeyNotFound
	}

	return fakeAstroCacheEntry{value: record.value}, nil
}

func (b *fakeAstroCacheBucket) Put(_ context.Context, key string, value []byte) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	record := fakeAstroCacheRecord{value: append([]byte(nil), value...)}
	if b.ttl > 0 {
		record.expiresAt = time.Now().Add(b.ttl)
	}
	b.entries[key] = record
	return 1, nil
}

func TestGetAstroProfileUsesCacheOnSecondCall(t *testing.T) {
	var httpRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequests.Add(1)
		_, _ = w.Write([]byte(`{"birth_date":"1992-06-26","birth_place":"Moscow","birth_time":"08:30"}`))
	}))
	defer server.Close()

	cache := newFakeAstroCacheManager()
	client := NewAstroAPIClient(server.URL, nil, zap.NewNop(), AstroAPIClientOptions{
		HTTPClient:  server.Client(),
		CacheTTL:    time.Hour,
		CacheBucket: "test_profiles",
	})
	client.cache = cache

	first, err := client.GetAstroProfile("1992-06-26", "Moscow")
	require.NoError(t, err)
	require.Equal(t, "08:30", first.BirthTime)

	second, err := client.GetAstroProfile("1992-06-26", "Moscow")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, int32(1), httpRequests.Load())
}

func TestGetAstroProfileRefetchesAfterTTLExpiration(t *testing.T) {
	var httpRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := httpRequests.Add(1)
		if requestNumber == 1 {
			_, _ = w.Write([]byte(`{"birth_date":"1992-06-26","birth_place":"Moscow","birth_time":"08:30"}`))
			return
		}
		_, _ = w.Write([]byte(`{"birth_date":"1992-06-26","birth_place":"Moscow","birth_time":"09:45"}`))
	}))
	defer server.Close()

	cache := newFakeAstroCacheManager()
	client := NewAstroAPIClient(server.URL, nil, zap.NewNop(), AstroAPIClientOptions{
		HTTPClient:  server.Client(),
		CacheTTL:    50 * time.Millisecond,
		CacheBucket: "test_profiles",
	})
	client.cache = cache

	first, err := client.GetAstroProfile("1992-06-26", "Moscow")
	require.NoError(t, err)
	require.Equal(t, "08:30", first.BirthTime)

	time.Sleep(70 * time.Millisecond)

	second, err := client.GetAstroProfile("1992-06-26", "Moscow")
	require.NoError(t, err)
	require.Equal(t, "09:45", second.BirthTime)
	require.Equal(t, int32(2), httpRequests.Load())
}

func TestGetAstroProfileLogsCacheHitAndMiss(t *testing.T) {
	var httpRequests atomic.Int32

	core, observedLogs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequests.Add(1)
		_, _ = w.Write([]byte(`{"birth_date":"1992-06-26","birth_place":"Moscow","birth_time":"08:30"}`))
	}))
	defer server.Close()

	cache := newFakeAstroCacheManager()
	client := NewAstroAPIClient(server.URL, nil, logger, AstroAPIClientOptions{
		HTTPClient:  server.Client(),
		CacheTTL:    time.Hour,
		CacheBucket: "test_profiles",
	})
	client.cache = cache

	_, err := client.GetAstroProfile("1992-06-26", "Moscow")
	require.NoError(t, err)
	_, err = client.GetAstroProfile("1992-06-26", "Moscow")
	require.NoError(t, err)

	require.Equal(t, int32(1), httpRequests.Load())
	require.Equal(t, 2, observedLogs.Len())
	require.Equal(t, "astro profile cache miss", observedLogs.All()[0].Message)
	require.Equal(t, "astro profile cache hit", observedLogs.All()[1].Message)
}

func TestBuildAstroProfileURLAddsQueryParams(t *testing.T) {
	requestURL, err := buildAstroProfileURL("https://astro.example/api/profile", "1992-06-26", "Santa Cruz")
	require.NoError(t, err)
	require.Contains(t, requestURL, "birth_date=1992-06-26")
	require.Contains(t, requestURL, "birth_place=Santa+Cruz")
}

func TestBuildAstroProfileCacheKeyUsesTaskFormat(t *testing.T) {
	key := buildAstroProfileCacheKey("1992-06-26", "Moscow")
	require.Equal(t, "astro_profile:1992-06-26:Moscow", key)
}

func TestDecodeAstroProfileSupportsEnvelope(t *testing.T) {
	profile, err := decodeAstroProfile([]byte(`{"profile":{"birth_date":"1992-06-26","birth_place":"Moscow","birth_time":"08:30"}}`))
	require.NoError(t, err)
	require.Equal(t, "08:30", profile.BirthTime)
}

func TestGetAstroProfileFailsWhenURLIsMissing(t *testing.T) {
	client := NewAstroAPIClient("", nil, zap.NewNop(), AstroAPIClientOptions{})
	_, err := client.GetAstroProfile("1992-06-26", "Moscow")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ASTRO_API_URL")
}

func TestGetCachedProfileReturnsDecodeErrorForCorruptedEntry(t *testing.T) {
	cache := newFakeAstroCacheManager()
	bucket, err := cache.GetOrCreateBucket(context.Background(), "test_profiles", time.Hour)
	require.NoError(t, err)
	_, err = bucket.Put(context.Background(), buildAstroProfileCacheKey("1992-06-26", "Moscow"), []byte("not-json"))
	require.NoError(t, err)

	client := NewAstroAPIClient("https://astro.example/profile", nil, zap.NewNop(), AstroAPIClientOptions{
		CacheTTL:    time.Hour,
		CacheBucket: "test_profiles",
	})
	client.cache = cache

	_, _, err = client.getCachedProfile(context.Background(), buildAstroProfileCacheKey("1992-06-26", "Moscow"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode cached astro profile")
}

func TestDecodeAstroProfileFailsOnEmptyResponse(t *testing.T) {
	_, err := decodeAstroProfile([]byte(`{"profile":{}}`))
	require.EqualError(t, err, "astro profile response is empty")
}
