package repositories

import (
	"astroapi/internal/models"
	"context"
)

type StreamRepository interface {
	CreateOrUpdateStream(ctx context.Context, stream *models.StreamCfg) error
	GetStream(ctx context.Context, name string) (*models.StreamCfg, error)
	CreateOrUpdateConsumer(ctx context.Context, consumer *models.Consumer) error
	GetConsumer(ctx context.Context, streamName, name string) (*models.Consumer, error)
}
