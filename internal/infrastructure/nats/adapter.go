package nats

import (
	"astroapi/config"
	"astroapi/internal/models"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type NATSConn struct {
	*nats.Conn
	logger *zap.Logger
}

var (
	backOff = [4]time.Duration{
		time.Duration(5 * time.Second),
		time.Duration(30 * time.Second),
		time.Duration(5 * time.Minute),
		time.Duration(1 * time.Hour),
	}
)

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
			Duplicates:   time.Duration(2 * time.Minute),
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
			Duplicates:   time.Duration(30 * time.Second),
			Storage:      0,
			Replicas:     1,
		},
	}

	for _, streamCfg := range streamsCfg {
		if _, err := r.CreateOrUpdateStream(ctx, streamCfg); err != nil {
			return fmt.Errorf("failed to create/update stream %s: %w", streamCfg.Name, err)
		}
	}

	consumersCfg := []jetstream.ConsumerConfig{
		{
			Name:           models.MsgProfileWrk,
			FilterSubjects: []string{fmt.Sprint(models.MsgProfileSubj, ".>")},
			AckPolicy:      jetstream.AckExplicitPolicy,
			MaxDeliver:     models.MsgSMaxRetries,
			ReplayPolicy:   jetstream.ReplayInstantPolicy,
			AckWait:        time.Duration(30 * time.Second),
			MaxAckPending:  1000,
			BackOff:        backOff[:],
		},
		{
			Name:           models.MsgRecommendWrk,
			FilterSubjects: []string{fmt.Sprint(models.MsgRecommendSubj, ".>")},
			AckPolicy:      jetstream.AckExplicitPolicy,
			MaxDeliver:     models.MsgSMaxRetries,
			ReplayPolicy:   jetstream.ReplayInstantPolicy,
			AckWait:        time.Duration(30 * time.Second),
			MaxAckPending:  1000,
			BackOff:        backOff[:],
		},
	}

	for _, consumerCfg := range consumersCfg {
		if _, err := r.CreateOrUpdateConsumer(ctx, "astro_events", consumerCfg); err != nil {
			return fmt.Errorf("failed to create/update consumer %s: %w", consumerCfg.Name, err)
		}
	}

	return nil
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

	subject := originalMsg.Subject()
	msg := nats.Msg{
		Data:    originalMsg.Data(),
		Subject: subject,
		Header: nats.Header{
			"original_subject": {subject},
			"original_msg_id":  {strconv.FormatUint(id, 10)},
			"Failure-Reason":   {reason},
			"Timestamp":        {time.Now().UTC().Format(time.RFC3339)},
		},
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ack, err := r.PublishMsg(pubCtx, &msg, jetstream.WithExpectStream(models.MsgStreamDLQ))
	if err != nil {
		r.logger.Error("Failed to publish to DLQ",
			zap.String("subject", subject),
			zap.String("error", err.Error()))
		return fmt.Errorf("DLQ publish failed: %w", err)
	}

	r.logger.Warn("Message sent to DLQ",
		zap.String("original_subject", subject),
		zap.String("dlq_subject", subject),
		zap.String("reason", reason),
		zap.Uint64("message_id", ack.Sequence))

	return nil
}
