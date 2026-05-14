package infra

import (
	health "astroapi/internal/infrastructure/health"
	nats "astroapi/internal/infrastructure/nats"
	"astroapi/internal/metrics"
	"astroapi/internal/models"
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	lagTickerDuration    = 15 * time.Second
	natsRequestTimeout   = 5 * time.Second
	healthTickerDuration = 15 * time.Second
	healthRequestTimeout = 5 * time.Second
)

type MonitorService struct {
	js     *nats.JetStreamAdapter
	hth    *health.HealthServiceRepo
	logger *zap.Logger
}

func NewMonitorService(js *nats.JetStreamAdapter, hth *health.HealthServiceRepo, logger *zap.Logger) *MonitorService {
	return &MonitorService{js: js, hth: hth, logger: logger}
}

func (m *MonitorService) StartInfraMonitor(ctx context.Context) {
	lagTicker := time.NewTicker(lagTickerDuration)
	healthTicker := time.NewTicker(healthTickerDuration)
	defer lagTicker.Stop()
	defer healthTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Stopping nats monitor")
			return
		case <-lagTicker.C:
			infoctx, cancel := context.WithTimeout(ctx, natsRequestTimeout)
			m.GetConsumerInfo(infoctx, m.js, models.MsgStreamEvents, models.MsgProfileWrk)
			m.GetConsumerInfo(infoctx, m.js, models.MsgStreamEvents, models.MsgRecommendWrk)
			cancel()
		case <-healthTicker.C:
			infoctx, cancel := context.WithTimeout(ctx, healthRequestTimeout)
			m.hth.PingInfra(infoctx)
			cancel()
		}
	}
}

func (m *MonitorService) GetConsumerInfo(ctx context.Context, js *nats.JetStreamAdapter, stream, consumer string) {
	cs, err := js.Consumer(ctx, stream, consumer)
	if err != nil {
		m.logger.Error("error getting consumer lag",
			zap.String("stream", stream),
			zap.String("consumer", consumer),
			zap.String("error", err.Error()))
		return
	}
	inf, err := cs.Info(ctx)
	if err != nil {
		m.logger.Error("error getting consumer lag",
			zap.String("stream", stream),
			zap.String("consumer", consumer),
			zap.String("error", err.Error()))
		return
	}
	metrics.SetNatsConsumerLag(stream, consumer, float64(inf.NumPending))
	m.logger.Info("got consumer lag",
		zap.String("stream", stream),
		zap.String("consumer", consumer),
		zap.Float64("lag", float64(inf.NumPending)))
}
