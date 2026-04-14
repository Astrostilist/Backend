package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"astroapi/internal/models"

	"github.com/nats-io/nats.go/jetstream"
)

type JetStreamRepository struct {
	js jetstream.JetStream
}

func NewJetStreamRepository(js jetstream.JetStream) *JetStreamRepository {
	return &JetStreamRepository{js: js}
}

func (r *JetStreamRepository) CreateOrUpdateStream(ctx context.Context, stream *models.StreamCfg) error {
	jStreamCfg := r.domainToJetStreamConfig(stream)

	_, err := r.js.Stream(ctx, stream.Name)
	if err != nil {
		if !errors.Is(err, jetstream.ErrStreamNotFound) {
			return fmt.Errorf("failed to get stream: %w", err)
		}

		_, err = r.js.CreateStream(ctx, jStreamCfg)
		if err != nil {
			return fmt.Errorf("failed to create stream: %w", err)
		}
		return nil
	}

	_, err = r.js.UpdateStream(ctx, jStreamCfg)
	if err != nil {
		return fmt.Errorf("failed to update stream: %w", err)
	}

	return nil
}

func (r *JetStreamRepository) GetStream(ctx context.Context, name string) (*models.StreamCfg, error) {
	stream, err := r.js.Stream(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}
	return r.streamToDomainStream(stream), nil
}

func (r *JetStreamRepository) CreateOrUpdateConsumer(ctx context.Context, consumer *models.Consumer) error {
	stream, err := r.js.Stream(ctx, consumer.StreamName)
	if err != nil {
		return fmt.Errorf("failed to get stream %s: %w", consumer.StreamName, err)
	}

	consumerAdapter := r.domainToJetStreamConsumer(consumer)

	_, err = stream.Consumer(ctx, consumer.Name)
	if err != nil {
		if !errors.Is(err, jetstream.ErrConsumerNotFound) {
			return fmt.Errorf("failed to get consumer: %w", err)
		}

		_, err = stream.CreateOrUpdateConsumer(ctx, consumerAdapter)
		if err != nil {
			return fmt.Errorf("failed to create consumer: %w", err)
		}
		return nil
	}

	_, err = stream.CreateOrUpdateConsumer(ctx, consumerAdapter)
	if err != nil {
		return fmt.Errorf("failed to update consumer: %w", err)
	}

	return nil
}

func (r *JetStreamRepository) GetConsumer(ctx context.Context, streamName, name string) (*models.Consumer, error) {
	stream, err := r.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	consumer, err := stream.Consumer(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	info := consumer.CachedInfo()
	return &models.Consumer{
		Name:       info.Name,
		StreamName: streamName,
		AckPolicy:  r.jetStreamToDomainAckPolicy(info.Config.AckPolicy),
		MaxDeliver: info.Config.MaxDeliver,
		Durable:    info.Config.Durable != "",
	}, nil
}

func (r *JetStreamRepository) domainToJetStreamConfig(domain *models.StreamCfg) jetstream.StreamConfig {
	return jetstream.StreamConfig{
		Name:         domain.Name,
		Retention:    jetstream.RetentionPolicy(domain.Retention),
		Subjects:     domain.Subjects,
		MaxConsumers: domain.MaxConsumers,
		MaxMsgs:      domain.MaxMessages,
		MaxBytes:     domain.MaxBytes,
		Duplicates:   time.Duration(domain.Duplicates),
		Storage:      jetstream.StorageType(domain.Storage),
		Replicas:     domain.Replicas,
	}
}

func (r *JetStreamRepository) streamToDomainStream(stream jetstream.Stream) *models.StreamCfg {
	info := stream.CachedInfo()
	return &models.StreamCfg{
		Name:         info.Config.Name,
		Retention:    r.jetStreamToDomainRetention(info.Config.Retention),
		Subjects:     info.Config.Subjects,
		MaxConsumers: int(info.Config.MaxConsumers),
		MaxMessages:  info.Config.MaxMsgs,
		MaxBytes:     info.Config.MaxBytes,
		Storage:      r.jetStreamToDomainStorageType(info.Config.Storage),
	}
}

func (r *JetStreamRepository) domainToJetStreamConsumer(domain *models.Consumer) jetstream.ConsumerConfig {
	return jetstream.ConsumerConfig{
		Name: domain.Name,
		//Durable:       domain.Durable, TODO
		AckPolicy:     jetstream.AckPolicy(domain.AckPolicy),
		MaxDeliver:    domain.MaxDeliver,
		ReplayPolicy:  jetstream.ReplayPolicy(domain.ReplayPolicy),
		AckWait:       time.Duration(domain.AckWait),
		MaxAckPending: domain.MaxAckPending,
		BackOff:       []time.Duration{},
	}
}

func (r *JetStreamRepository) jetStreamToDomainRetention(policy jetstream.RetentionPolicy) models.RetentionPolicy {
	return models.RetentionPolicy(policy)
}

func (r *JetStreamRepository) jetStreamToDomainAckPolicy(policy jetstream.AckPolicy) models.AckPolicy {
	return models.AckPolicy(policy)
}

func (r *JetStreamRepository) jetStreamToDomainStorageType(storage jetstream.StorageType) models.StorageType {
	return models.StorageType(storage)
}
