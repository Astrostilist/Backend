package handlers

import (
	"astroapi/internal/models"
	"context"
	"fmt"

	"go.uber.org/zap"
)

//go:generate mockgen -source=message.go -destination=mocks/mock_message.go -package=mocks

// EventHandler обрабатывает одно сообщение из JetStream.
type EventHandler interface {
	Handle(ctx context.Context, payload []byte) error
}

// MsgPublisher отправляет сообщения в JetStream.
type MsgPublisher interface {
	PublishMessage(ctx context.Context, streamName, subject string, payload any) error
}

type MsgDLQReader interface {
	GetMessages(ctx context.Context) ([]models.Message, error)
}

// HandlerFunc — функциональный адаптер под EventHandler.
type HandlerFunc func(ctx context.Context, payload []byte) error

func (f HandlerFunc) Handle(ctx context.Context, payload []byte) error {
	return f(ctx, payload)
}

// MsgRouter направляет входящие сообщения по subject.
type MsgRouter struct {
	handlers map[string]EventHandler
	logger   *zap.Logger
}

func NewMsgRouter(logger *zap.Logger) *MsgRouter {
	return &MsgRouter{
		handlers: make(map[string]EventHandler),
		logger:   logger,
	}
}

func (r *MsgRouter) Register(subject string, handler EventHandler) {
	r.handlers[subject] = handler
	r.logger.Info("handler registered", zap.String("subject", subject))
}

// Dispatch вызывает зарегистрированный хендлер для subject.
func (r *MsgRouter) Dispatch(ctx context.Context, subject string, data []byte) error {
	handler, ok := r.handlers[subject]
	if !ok {
		return fmt.Errorf("no handler found for subject: %s", subject)
	}
	r.logger.Debug("dispatching message", zap.String("subject", subject))
	return handler.Handle(ctx, data)
}
