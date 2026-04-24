package repositories

import (
	"astroapi/internal/usecases/repositories/domain"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
)

type CacheRepo struct {
	client *memcache.Client
	ttl    int32
}

func NewCacheRepo(ttl time.Duration, servers []string) *CacheRepo {
	return &CacheRepo{
		client: memcache.New(servers...),
		ttl:    int32(ttl.Seconds()),
	}
}

func (r *CacheRepo) Save(ctx context.Context, data domain.PersonalData) error {
	if data.UserID == "" {
		return errors.New("UserID cannot be empty")
	}
	key := fmt.Sprintf("personal_data:%s", data.UserID)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("personal data serialization error: %w", err)
	}
	// Создаём запись в Memcached
	item := &memcache.Item{
		Key:        key,
		Value:      jsonData,
		Expiration: r.ttl,
	}

	if err := r.client.Set(item); err != nil {
		return fmt.Errorf("memcached save error: %w", err)
	}

	return nil
}
