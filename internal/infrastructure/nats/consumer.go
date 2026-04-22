package nats

import (
	"astroapi/internal/models"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type MessageConsumer struct {
	sm *JetStreamAdapter
}

func NewMessageConsumer(js *JetStreamAdapter, logger *zap.Logger) *MessageConsumer {
	return &MessageConsumer{sm: js}
}

func (c *MessageConsumer) ConsumeWithHandler(ctx context.Context, streamName, consumerName string,
	handler func(context.Context, jetstream.Msg) error) error {
	stream, err := c.sm.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream: %w", err)
	}

	consumer, err := stream.Consumer(ctx, consumerName)
	if err != nil {
		return fmt.Errorf("failed to get consumer %s: %w", consumerName, err)
	}

	consumerCtx, err := consumer.Consume(
		func(msg jetstream.Msg) {
			msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			if err := handler(msgCtx, msg); err != nil {
				c.sm.logger.Error("Message handler failed",
					zap.String("consumer", consumerName),
					zap.String("subject", msg.Subject()),
					zap.String("error", err.Error()))
				attempt := uint64(1)
				id := uint64(0)
				if meta, metaErr := msg.Metadata(); metaErr == nil {
					attempt = meta.NumDelivered
					id = meta.Sequence.Stream
				}
				if isPermanentError(err) || attempt >= models.MsgSMaxRetries {
					if dlqErr := c.sm.publishToDLQ(ctx, msg, err.Error(), id); dlqErr != nil {
						c.sm.logger.Error("Failed to send to DLQ",
							zap.String("error", dlqErr.Error()))
					} else {
						if err = msg.Ack(); err != nil {
							c.sm.logger.Error("Failed to ack original message sent to DLQ",
								zap.String("error", err.Error()))
						}
					}
				} else {
					c.sm.logger.Info("Temporary error, allowing redelivery", zap.String("consumer", consumerName))
					// длительность задержки игнорируется, т к приоритет у настроек стрима,
					// но на всякий случай продублируем здесь
					delayID := min(attempt, 3)
					if nackErr := msg.NakWithDelay(backOff[delayID]); nackErr != nil {
						c.sm.logger.Error("Failed to negative acknowledge message",
							zap.String("error", nackErr.Error()))
					} else {
						c.sm.logger.Error("Message negative acknowledged",
							zap.String("consumer", consumerName),
							zap.String("subject", msg.Subject()),
							zap.Uint64("msg_id", id),
							zap.Uint64("attempt", attempt),
							zap.Any("next_delay", backOff[delayID]),
							zap.Error(err))
					}

				}
			} else {
				if ackErr := msg.Ack(); ackErr != nil {
					c.sm.logger.Error("Failed to acknowledge message",
						zap.String("error", ackErr.Error()))
				} else {
					c.sm.logger.Debug("Message processed and acknowledged",
						zap.String("consumer", consumerName),
						zap.String("subject", msg.Subject()))
				}
			}
		},
		jetstream.PullExpiry(30*time.Second),
		jetstream.PullHeartbeat(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	go func() {
		<-ctx.Done()
		consumerCtx.Stop()
	}()

	return nil
}

var permanentErrorMarkers = []string{"validation", "malformed", "invalid_format"}

func isPermanentError(err error) bool {
	errStr := err.Error()
	for _, marker := range permanentErrorMarkers {
		if strings.Contains(errStr, marker) {
			return true
		}
	}
	return false
}
