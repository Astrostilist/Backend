package natsadapter

import (
	"astroapi/internal/usecases"
	"context"

	"go.uber.org/zap"
)

type StreamManager struct {
	streamUseCase *usecases.StreamUseCase
	logger        *zap.Logger
}

func NewStreamManager(streamUseCase *usecases.StreamUseCase, logger *zap.Logger) *StreamManager {
	return &StreamManager{
		streamUseCase: streamUseCase,
		logger:        logger,
	}
}

func (sm *StreamManager) Initialize(ctx context.Context) error {
	sm.logger.Info("Initializing streams and consumers...")

	if err := sm.streamUseCase.Initialize(ctx); err != nil {
		sm.logger.Error("Streams and consumers initialization failed", zap.Error(err))
		return err
	}

	sm.logger.Info("Streams and consumers initialized successfully")
	return nil
}
