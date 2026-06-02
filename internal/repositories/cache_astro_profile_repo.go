package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"

	"astroapi/internal/repositories/domain"

	"github.com/bradfitz/gomemcache/memcache"
)

type cacheAstroProfileRepo struct {
	client *memcache.Client
	ttl    int32
}

func NewCacheAstroProfileRepo(servers []string, ttl time.Duration) *cacheAstroProfileRepo {
	return &cacheAstroProfileRepo{
		client: memcache.New(servers...),
		ttl:    int32(ttl.Seconds()),
	}
}

func (r *cacheAstroProfileRepo) Save(ctx context.Context, profile domain.AstroProfile) error {
	tracer := otel.Tracer("cache-astro-profile-repo")
	repoctx, repoSpan := tracer.Start(ctx, "astro-profile.Save")
	_ = repoctx
	defer repoSpan.End()

	if profile.ProfileHash == "" {
		err := errors.New("profileHash cannot be empty")
		repoSpan.RecordError(err)
		return err
	}
	key := fmt.Sprintf("astro_profile:%s", profile.ProfileHash)
	jsonData, err := json.Marshal(profile)
	if err != nil {
		err = fmt.Errorf("astro_profile serialization error: %w", err)
		repoSpan.RecordError(err)
		return err
	}
	item := &memcache.Item{
		Key:        key,
		Value:      jsonData,
		Expiration: r.ttl,
	}

	if err := r.client.Set(item); err != nil {
		err = fmt.Errorf("memcached save error: %w", err)
		repoSpan.RecordError(err)
		return err
	}
	return nil
}
func (r *cacheAstroProfileRepo) ReceivingByHash(ctx context.Context, hash string) (*domain.AstroProfile, error) {
	tracer := otel.Tracer("cache-astro-profile-repo")
	_, repoSpan := tracer.Start(ctx, "astro-profile.ReceivingByHash")
	defer repoSpan.End()

	if hash == "" {
		err := errors.New("profile hash must not be empty")
		repoSpan.RecordError(err)
		return nil, err
	}
	key := "astro_profile:" + hash

	var p *domain.AstroProfile

	item, err := r.client.Get(key)

	if err == nil {
		if unmarshalErr := json.Unmarshal(item.Value, &p); unmarshalErr != nil {
			unmarshalErr := fmt.Errorf("data deserialization error: %w", unmarshalErr)
			repoSpan.RecordError(unmarshalErr)
			return nil, unmarshalErr
		}
	} else if !errors.Is(err, memcache.ErrCacheMiss) {
		repoSpan.RecordError(err)
		return nil, err
	} else {
		return nil, nil
	}
	return p, nil
}
