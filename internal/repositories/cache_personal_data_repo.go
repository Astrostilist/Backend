package repositories

import (
	"astroapi/internal/repositories/domain"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"go.opentelemetry.io/otel"
)

type CacheRepo struct {
	client *memcache.Client
	ttl    int32
}

func NewCacheRepo(servers []string, ttl time.Duration) *CacheRepo {
	return &CacheRepo{
		client: memcache.New(servers...),
		ttl:    int32(ttl.Seconds()),
	}
}

func (r *CacheRepo) Save(ctx context.Context, data domain.PersonalData) error {
	tracer := otel.Tracer("cache-repo")
	repoctx, repoSpan := tracer.Start(ctx, "user-profile.Save")
	_ = repoctx
	defer repoSpan.End()

	if data.UserID == "" {
		err := errors.New("UserID cannot be empty")
		repoSpan.RecordError(err)
		return err
	}
	key := fmt.Sprintf("personal_data:%s", data.UserID)
	jsonData, err := json.Marshal(data)
	if err != nil {
		err := fmt.Errorf("personal data serialization error: %w", err)
		repoSpan.RecordError(err)
		return err
	}
	// Создаём запись в Memcached
	item := &memcache.Item{
		Key:        key,
		Value:      jsonData,
		Expiration: r.ttl,
	}

	if err := r.client.Set(item); err != nil {
		err := fmt.Errorf("memcached save error: %w", err)
		repoSpan.RecordError(err)
		return err
	}

	return nil
}
