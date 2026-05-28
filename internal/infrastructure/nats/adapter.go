package nats

import (
	"astroapi/config"
	"astroapi/internal/models"
	"context"
	"errors"
	"fmt"

	"strconv"
	"time"

	"astroapi/internal/metrics"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type NATSConn struct {
	*nats.Conn
	logger *zap.Logger
}

var defaultBackOff = [4]time.Duration{
	5 * time.Second,
	30 * time.Second,
	5 * time.Minute,
	1 * time.Hour,
}

func InitNATS(ctx context.Context, logger *zap.Logger, cfg *config.Config) (*NATSConn, error) {
	opts := []nats.Option{
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Info("Disconnected from NATS", zap.Error(err))
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("Reconnected to NATS", zap.String("url", nc.ConnectedUrl()))
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Info("NATS connection closed")
		}),
	}
	natsurl := fmt.Sprintf("nats://%s:%s", cfg.NATSHost, cfg.NATSPort)
	conn, err := nats.Connect(natsurl, opts...)
	return &NATSConn{Conn: conn, logger: logger}, err
}

func (nc *NATSConn) DrainNATS() {
	err := nc.Drain()
	if err != nil {
		nc.logger.Error("Failed to drain NATS connection", zap.Error(err))
	}
	nc.Close()
}

func (nc *NATSConn) Ping(ctx context.Context) error {
	if nc.IsConnected() {
		return nil
	}
	return fmt.Errorf("nats is disconnected")
}

type JetStreamAdapter struct {
	jetstream.JetStream
	logger *zap.Logger
}

func NewJetStreamRepository(js jetstream.JetStream, logger *zap.Logger) *JetStreamAdapter {
	return &JetStreamAdapter{js, logger}
}

func (r *JetStreamAdapter) InitializeStreams(ctx context.Context) error {
	r.logger.Info("Initializing streams and consumers...")

	if err := r.initStreams(ctx); err != nil {
		r.logger.Error("Streams and consumers initialization failed", zap.Error(err))
		return err
	}

	r.logger.Info("Streams and consumers initialized successfully")
	return nil
}

func (r *JetStreamAdapter) initStreams(ctx context.Context) error {
	streamsCfg := []jetstream.StreamConfig{
		{
			Name:         models.MsgStreamEvents,
			Retention:    jetstream.WorkQueuePolicy,
			Subjects:     []string{"astro.events.>"},
			MaxConsumers: -1,
			MaxMsgs:      -1,
			MaxBytes:     -1,
			Duplicates:   2 * time.Minute,
			Storage:      0,
			Replicas:     1,
		},
		{
			Name:         models.MsgStreamDLQ,
			Retention:    jetstream.LimitsPolicy,
			Subjects:     []string{"astro.dlq.>"},
			MaxConsumers: -1,
			MaxMsgs:      100000,
			MaxBytes:     100 << 20,
			Duplicates:   30 * time.Second,
			Storage:      0,
			Replicas:     1,
		},
	}

	for _, streamCfg := range streamsCfg {
		// First check if the stream exists
		stream, err := r.Stream(ctx, streamCfg.Name)
		if err != nil {
			// If stream doesn't exist, create it
			if errors.Is(err, jetstream.ErrStreamNotFound) {
				if _, err := r.CreateStream(ctx, streamCfg); err != nil {
					return fmt.Errorf("failed to create stream %s: %w", streamCfg.Name, err)
				}
			} else {
				return fmt.Errorf("failed to check stream %s: %w", streamCfg.Name, err)
			}
		} else {
			// Stream exists - we should not try to change retention policies
			// Get current stream configuration
			info, err := stream.Info(ctx)
			if err != nil {
				return fmt.Errorf("failed to get stream %s info: %w", streamCfg.Name, err)
			}

			// Check if retention policy is different (this is what causes the error)
			if info.Config.Retention != streamCfg.Retention {
				r.logger.Warn("Stream retention policy differs from configuration - skipping update",
					zap.String("stream", streamCfg.Name),
					zap.String("current_policy", info.Config.Retention.String()),
					zap.String("configured_policy", streamCfg.Retention.String()))
				// Continue with existing stream - don't try to change retention policy
				continue
			}

			// For other configurations, we could potentially update if needed
			// But to be safe and avoid other potential issues, we'll just log that
			// the stream exists with the expected configuration
			r.logger.Info("Stream already exists with matching configuration",
				zap.String("stream", streamCfg.Name))
		}
	}

	consumersCfg := []jetstream.ConsumerConfig{
		{
			Name: models.MsgProfileWrk,
			FilterSubjects:    []string{models.MsgProfileSubj, fmt.Sprint(models.MsgProfileSubj, ".>")},
			AckPolicy:         jetstream.AckExplicitPolicy,
			ReplayPolicy:      jetstream.ReplayInstantPolicy,
			AckWait:           30 * time.Second,
			MaxAckPending:     1000,
			MaxDeliver:        models.MsgSMaxRetries,
			InactiveThreshold: 24 * time.Hour,
		},
		{
			Name:              models.MsgRecommendWrk,
			FilterSubjects:    []string{models.MsgRecommendSubj, fmt.Sprint(models.MsgRecommendSubj, ".>")},
			AckPolicy:         jetstream.AckExplicitPolicy,
			ReplayPolicy:      jetstream.ReplayInstantPolicy,
			AckWait:           30 * time.Second,
			MaxAckPending:     1000,
			MaxDeliver:        models.MsgSMaxRetries,
			InactiveThreshold: 24 * time.Hour,
		},
	}

	for _, consumerCfg := range consumersCfg {
		// Check if consumer exists before trying to create/update
		stream, err := r.Stream(ctx, "astro_events")
		if err != nil {
			return fmt.Errorf("failed to get stream astro_events for consumer %s: %w", consumerCfg.Name, err)
		}

		_, err = stream.Consumer(ctx, consumerCfg.Name)
		if err != nil {
			// If consumer doesn't exist, create it
			if errors.Is(err, jetstream.ErrConsumerNotFound) {
				if _, err := r.CreateOrUpdateConsumer(ctx, "astro_events", consumerCfg); err != nil {
					return fmt.Errorf("failed to create consumer %s: %w", consumerCfg.Name, err)
				}
			} else {
				return fmt.Errorf("failed to check consumer %s: %w", consumerCfg.Name, err)
			}
		} else {
			// Consumer exists, log that we're using existing one
			r.logger.Info("Consumer already exists",
				zap.String("consumer", consumerCfg.Name),
				zap.String("stream", "astro_events"))
		}
	}

	_, err := r.initDLQConsumer(ctx)
	return err

}

func (r *JetStreamAdapter) initDLQConsumer(ctx context.Context) (jetstream.Consumer, error) {

	var consumer jetstream.Consumer
	var err error

	consumerCfg := jetstream.ConsumerConfig{
		Name:              models.MsgDLQViewer,
		FilterSubjects:    []string{fmt.Sprint(models.MsgDLQSubj, ">")},
		DeliverPolicy:     jetstream.DeliverLastPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		ReplayPolicy:      jetstream.ReplayInstantPolicy,
		InactiveThreshold: 24 * time.Hour, // Авто-удаление если не используется
	}

	// Check if consumer exists first
	stream, err := r.Stream(ctx, models.MsgStreamDLQ)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream %s for DLQ consumer: %w", models.MsgStreamDLQ, err)
	}

	_, err = stream.Consumer(ctx, consumerCfg.Name)
	if err != nil {
		// If consumer doesn't exist, create it
		if errors.Is(err, jetstream.ErrConsumerNotFound) {
			if consumer, err = r.CreateOrUpdateConsumer(ctx, models.MsgStreamDLQ, consumerCfg); err != nil {
				return nil, fmt.Errorf("failed to create consumer %s: %w", consumerCfg.Name, err)
			}
		} else {
			return nil, fmt.Errorf("failed to check DLQ consumer %s: %w", consumerCfg.Name, err)
		}
	} else {
		// Consumer exists, log that we're using existing one
		r.logger.Info("DLQ consumer already exists",
			zap.String("consumer", consumerCfg.Name),
			zap.String("stream", models.MsgStreamDLQ))
	}

	return consumer, nil
}

func (r *JetStreamAdapter) publishMsg(ctx context.Context, subject, streamName string, payload []byte) error {

	ack, err := r.Publish(ctx, subject, payload, jetstream.WithExpectStream(streamName))
	if err != nil {
		r.logger.Error("Failed to publish event",
			zap.String("subject", subject),
			zap.String("error", err.Error()))
		return fmt.Errorf("publish failed: %w", err)
	}

	r.logger.Debug("Event published successfully",
		zap.String("subject", subject),
		zap.String("stream", streamName),
		zap.Uint64("message_id", ack.Sequence))

	return nil
}

func (r *JetStreamAdapter) publishToDLQ(ctx context.Context, originalMsg jetstream.Msg, reason string, id uint64) error {

	origsubject := originalMsg.Subject()
	subject, exists := models.DLQSubjectMap[origsubject]
	if !exists {
		subject = models.MsgDLQSubj
	}
	msg := nats.Msg{
		Data:    originalMsg.Data(),
		Subject: subject,
		Header: nats.Header{
			"original_subject": {origsubject},
			"original_msg_id":  {strconv.FormatUint(id, 10)},
			"failure_reason":   {reason},
			"timestamp":        {time.Now().UTC().Format(time.RFC3339)},
		},
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ack, err := r.PublishMsg(pubCtx, &msg, jetstream.WithExpectStream(models.MsgStreamDLQ))
	if err != nil {
		r.logger.Error("Failed to publish to DLQ",
			zap.String("stream", models.MsgStreamDLQ),
			zap.String("subject", subject),
			zap.String("error", err.Error()))
		return fmt.Errorf("DLQ publish failed: %w", err)
	}

	metrics.IncdlqMessagesTotal()

	r.logger.Warn("Message sent to DLQ",
		zap.String("original_subject", origsubject),
		zap.String("dlq_subject", subject),
		zap.String("reason", reason),
		zap.Uint64("message_id", ack.Sequence))

	return nil
}
