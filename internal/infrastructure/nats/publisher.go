package nats

import (
	"astroapi/internal/models"
	"context"
	"encoding/json"
	"fmt"

	"time"

	"go.uber.org/zap"
)

type MessagePublisher struct {
	sm *JetStreamAdapter
}

func NewMessagePublisher(js *JetStreamAdapter, logger *zap.Logger) *MessagePublisher {
	return &MessagePublisher{sm: js}
}

func (p *MessagePublisher) PublishMessage(ctx context.Context, streamName, subject string, payload any) error {

	if streamName == models.MsgStreamDLQ {
		return fmt.Errorf("publish failed: wrong stream name '%s'", streamName)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return p.sm.publishMsg(pubCtx, subject, streamName, data)

}
