package logger

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(servicename, logLevel string) (*zap.Logger, error) {
	level := zap.NewAtomicLevelAt(parseLogLevel(logLevel))

	encoderConfig := zapcore.EncoderConfig{
		LevelKey:      "level",
		TimeKey:       "timestamp",
		MessageKey:    "message",
		NameKey:       "service",
		StacktraceKey: "stacktrace",
		EncodeTime:    zapcore.ISO8601TimeEncoder,
		EncodeLevel:   zapcore.LowercaseLevelEncoder,
	}

	config := zap.NewProductionConfig()
	config.EncoderConfig = encoderConfig
	config.Level = level
	config.DisableStacktrace = false

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	logger = logger.With(zap.String("service", servicename))

	return logger, nil
}

func NewWithTraceMetadata(ctx context.Context, logger *zap.Logger) *zap.Logger {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return logger
	}
	return logger.With(
		zap.String("trace_id", spanContext.TraceID().String()),
		zap.String("span_id", spanContext.SpanID().String()),
	)
}

func parseLogLevel(loglevel string) zapcore.Level {
	var level zapcore.Level
	switch strings.ToLower(loglevel) {
	case "debug":
		level = zap.DebugLevel
	case "info":
		level = zap.InfoLevel
	case "warn", "warning":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	default:
		level = zap.InfoLevel
	}
	return level
}
