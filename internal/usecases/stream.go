package usecases

import (
	"astroapi/internal/models"
	"astroapi/internal/repositories"
	"context"
	"fmt"
	"time"
)

type StreamUseCase struct {
	repo repositories.StreamRepository
}

func NewStreamUseCase(repo repositories.StreamRepository) *StreamUseCase {
	return &StreamUseCase{repo: repo}
}

func (s *StreamUseCase) Initialize(ctx context.Context) error {
	streams := []*models.StreamCfg{
		{
			Name:         "astro_events",
			Retention:    models.WorkQueuePolicy,
			Subjects:     []string{"astro.events.>"},
			MaxConsumers: -1,
			MaxMessages:  -1,
			MaxBytes:     -1,
			Duplicates:   models.Duration(2 * time.Minute),
			Storage:      0,
			Replicas:     1,
		},
		{
			Name:         "astro_dlq",
			Retention:    models.LimitsPolicy,
			Subjects:     []string{"astro.dlq.>"},
			MaxConsumers: -1,
			MaxMessages:  100000,
			MaxBytes:     100 << 20,
			Duplicates:   models.Duration(30 * time.Second),
			Storage:      0,
			Replicas:     1,
		},
	}

	for _, stream := range streams {
		if err := s.repo.CreateOrUpdateStream(ctx, stream); err != nil {
			return fmt.Errorf("failed to create/update stream %s: %w", stream.Name, err)
		}
	}

	consumers := []*models.Consumer{
		{
			Name:          "astro-profile-worker",
			StreamName:    "astro_events",
			AckPolicy:     models.AckExplicit,
			MaxDeliver:    5,
			Durable:       true,
			ReplayPolicy:  models.ReplayInstant,
			AckWait:       models.Duration(30 * time.Second),
			MaxAckPending: 1000,
			BackOff:       []models.Duration{models.Duration(100 * time.Millisecond), models.Duration(500 * time.Millisecond), models.Duration(1 * time.Second)},
		},
		{
			Name:          "astro-recommend-worker",
			StreamName:    "astro_events",
			AckPolicy:     models.AckExplicit,
			MaxDeliver:    5,
			Durable:       true,
			ReplayPolicy:  models.ReplayInstant,
			AckWait:       models.Duration(30 * time.Second),
			MaxAckPending: 1000,
			BackOff:       []models.Duration{models.Duration(100 * time.Millisecond), models.Duration(500 * time.Millisecond), models.Duration(1 * time.Second)},
		},
	}

	for _, consumer := range consumers {
		if err := s.repo.CreateOrUpdateConsumer(ctx, consumer); err != nil {
			return fmt.Errorf("failed to create/update consumer %s: %w", consumer.Name, err)
		}
	}

	return nil
}
