package nats

import (
	"astroapi/internal/metrics"
	"astroapi/internal/models"
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	lagTickerDuration  = 15 * time.Second
	natsRequestTimeout = 5 * time.Second
)

func StartLagMonitor(ctx context.Context, js *JetStreamAdapter) {
	ticker := time.NewTicker(lagTickerDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			js.logger.Info("Stopping nats monitor")
			return
		case <-ticker.C:
			infoctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			GetConsumerInfo(infoctx, js, models.MsgStreamEvents, models.MsgProfileWrk)
			GetConsumerInfo(infoctx, js, models.MsgStreamEvents, models.MsgRecommendWrk)
			cancel()
		}
	}
}

func GetConsumerInfo(ctx context.Context, js *JetStreamAdapter, stream, consumer string) {
	cs, err := js.Consumer(ctx, stream, consumer)
	if err != nil {
		js.logger.Error("error getting consumer lag",
			zap.String("stream", stream),
			zap.String("consumer", consumer),
			zap.String("error", err.Error()))
		return
	}
	inf, err := cs.Info(ctx)
	if err != nil {
		js.logger.Error("error getting consumer lag",
			zap.String("stream", stream),
			zap.String("consumer", consumer),
			zap.String("error", err.Error()))
		return
	}
	metrics.SetNatsConsumerLag(stream, consumer, float64(inf.NumPending))
	js.logger.Info("got consumer lag",
		zap.String("stream", stream),
		zap.String("consumer", consumer),
		zap.Float64("lag", float64(inf.NumPending)))
}
