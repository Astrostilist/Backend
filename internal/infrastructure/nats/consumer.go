package nats

import (
	astrologger "astroapi/internal/infrastructure/logger"
	"astroapi/internal/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type MessageConsumer struct {
	sm      *JetStreamAdapter
	backOff [4]time.Duration
}

type DLQReader struct {
	sm *JetStreamAdapter
}

func NewMessageConsumer(js *JetStreamAdapter) *MessageConsumer {
	return &MessageConsumer{sm: js, backOff: defaultBackOff}
}

func (c *MessageConsumer) SetBackOffForTest(delays [4]time.Duration) {
	c.backOff = delays
}

func NewDLQReader(js *JetStreamAdapter, logger *zap.Logger) *DLQReader {
	return &DLQReader{sm: js}
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
			var wrappedMsg models.MessageWithTrace
			if err := json.Unmarshal(msg.Data(), &wrappedMsg); err != nil {
				// Если ошибка парсинга - используем глобальный логгер
				astrologger.Error(ctx, "Failed to unmarshal message with trace",
					zap.ByteString("data", msg.Data()),
					zap.String("error", err.Error()))

				return
			}

			// Восстанавливаем trace контекст
			carrier := propagation.MapCarrier(wrappedMsg.TraceContext)
			tracectx := otel.GetTextMapPropagator().Extract(ctx, carrier)

			parentSpan := trace.SpanFromContext(tracectx)
			parentSc := parentSpan.SpanContext()
			astrologger.Debug(tracectx, "ConsumeWithHandler debug: Extracted context",
				zap.Bool("valid", parentSc.IsValid()),
				zap.String("trace_id", parentSc.TraceID().String()),
				zap.String("span_id", parentSc.SpanID().String()))

			// Убедимся, что у нас есть span
			tracer := otel.Tracer("nats-consumer")
			spanctx, span := tracer.Start(tracectx, "consumer.process-message")
			defer span.End()

			// Создаем контекст с таймаутом, сохраняя trace информацию
			msgCtx, cancel := context.WithTimeout(spanctx, 30*time.Second)
			defer cancel()

			meta, _ := msg.Metadata()
			astrologger.Debug(msgCtx, "ConsumeWithHandler debug: Processing message",
				zap.Any("payload", wrappedMsg.Payload),
				zap.Uint64("stream_seq", meta.Sequence.Stream),
				zap.Uint64("delivered", meta.NumDelivered))

			if err := handler(msgCtx, msg); err != nil {
				astrologger.Error(msgCtx, "Message handler failed",
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
						astrologger.Error(msgCtx, "Failed to send to DLQ",
							zap.String("error", dlqErr.Error()))
					} else {
						if err = msg.Ack(); err != nil {
							astrologger.Error(msgCtx, "Failed to ack original message sent to DLQ",
								zap.String("error", err.Error()))
						}
					}
				} else {
					astrologger.Info(msgCtx, "Temporary error, allowing redelivery", zap.String("consumer", consumerName))
					// длительность задержки игнорируется, т к приоритет у настроек стрима,
					// но на всякий случай продублируем здесь
					delayID := min(attempt-1, 3)
					if nackErr := msg.NakWithDelay(c.backOff[delayID]); nackErr != nil {
						//if nackErr := msg.Nak(); nackErr != nil {
						c.sm.logger.Error("Failed to negative acknowledge message",
							zap.String("error", nackErr.Error()))
					} else {
						astrologger.Warn(msgCtx, "Message negative acknowledged",
							zap.String("consumer", consumerName),
							zap.String("subject", msg.Subject()),
							zap.Uint64("msg_id", id),
							zap.Uint64("attempt", attempt),
							zap.Any("next_delay", c.backOff[delayID]),
							zap.Error(err))
					}

				}
			} else {
				if ackErr := msg.Ack(); ackErr != nil {
					astrologger.Error(msgCtx, "Failed to acknowledge message",
						zap.String("error", ackErr.Error()))
				} else {
					astrologger.Info(msgCtx, "Message processed and acknowledged",
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

func (d *DLQReader) GetMessages(ctx context.Context) ([]models.Message, error) {

	var consumer jetstream.Consumer
	var err error

	stream, err := d.sm.Stream(ctx, models.MsgStreamDLQ)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream '%s': %w", models.MsgStreamDLQ, err)
	}

	consumer, err = stream.Consumer(ctx, models.MsgDLQViewer)
	if err != nil {
		if errors.Is(err, jetstream.ErrConsumerNotFound) {
			consumer, err = d.sm.initDLQConsumer(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to create consumer %s: %w", models.MsgDLQViewer, err)
			}
		} else {
			return nil, fmt.Errorf("failed to get consumer %s: %w", models.MsgDLQViewer, err)
		}
	}

	messages, err := consumer.Fetch(50, jetstream.FetchMaxWait(2*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	if messages.Error() != nil {
		return nil, fmt.Errorf("failed to get messages: %w", messages.Error())
	}

	res := []models.Message{}
	msgChan := messages.Messages()
	for msg := range msgChan {
		r := models.Message{
			Subject: msg.Subject(),
			Headers: msg.Headers(),
			Data:    string(msg.Data()),
		}
		res = append(res, r)
	}

	return res, nil
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
