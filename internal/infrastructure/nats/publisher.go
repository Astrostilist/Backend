package nats

import (
	astrologger "astroapi/internal/logger"
	"astroapi/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type MessagePublisher struct {
	sm *JetStreamAdapter
}

func NewMessagePublisher(js *JetStreamAdapter) *MessagePublisher {
	return &MessagePublisher{sm: js}
}

func (p *MessagePublisher) PublishMessage(ctx context.Context, streamName, subject string, payload any) error {

	if streamName == models.MsgStreamDLQ {
		return fmt.Errorf("publish failed: wrong stream name '%s'", streamName)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	span := trace.SpanFromContext(ctx)
	spanCtx := span.SpanContext()

	astrologger.Debug(ctx, "PublishMessage debug",
		zap.Bool("span_valid", spanCtx.IsValid()),
		zap.String("trace_id", spanCtx.TraceID().String()),
		zap.String("span_id", spanCtx.SpanID().String()))

	msg := models.MessageWithTrace{}
	if spanCtx.IsValid() {
		propagator := otel.GetTextMapPropagator()
		carrier := propagation.MapCarrier{}

		// Инжектим в carrier
		propagator.Inject(ctx, carrier)

		astrologger.Debug(ctx, "Trace context carrier",
			zap.Any("carrier_keys", carrier.Keys()),
			zap.Any("carrier_content", carrier))

		msg.TraceContext = carrier
	} else {
		msg.TraceContext = map[string]string{}
	}
	msg.Payload = payloadBytes

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return p.sm.publishMsg(pubCtx, subject, streamName, data)

}
