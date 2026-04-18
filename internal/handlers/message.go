package handlers

import (
	"astroapi/internal/models"
	"context"
	"fmt"

	"go.uber.org/zap"
)

type EventHandler interface {
	Handle(ctx context.Context, payload []byte) error
}

type MsgRouter struct {
	handlers map[string]EventHandler
	logger   *zap.Logger
}

// MsgPublisher - интерфейс для отправки сообщений
type MsgPublisher interface {
	PublishMessage(ctx context.Context, streamName, subject string, payload any) error
}

type MsgDLQReader interface {
	GetMessages(ctx context.Context) ([]models.Message, error)
}

type HandlerFunc func(ctx context.Context, payload []byte) error

func (f HandlerFunc) Handle(ctx context.Context, payload []byte) error {
	return f(ctx, payload)
}

func NewMsgRouter(logger *zap.Logger) *MsgRouter {
	return &MsgRouter{
		handlers: make(map[string]EventHandler),
		logger:   logger,
	}
}

func (r *MsgRouter) Register(subject string, handler EventHandler) {
	r.handlers[subject] = handler
	r.logger.Info("Handler registered", zap.String("event_type", subject))
}

// Dispatch принимает сырые данные и вызывает нужный хендлер для обработки
func (r *MsgRouter) Dispatch(ctx context.Context, subject string, data []byte) error {

	handler, ok := r.handlers[subject]
	if !ok {
		return fmt.Errorf("no handler found for subject: %s", subject)
	}

	r.logger.Debug("Dispatching message", zap.String("subject", subject))
	return handler.Handle(ctx, data)
}

func HandleProfile(ctx context.Context, payload []byte) error {
	return nil
}

func HandleRecommend(ctx context.Context, payload []byte) error {
	return nil
}
