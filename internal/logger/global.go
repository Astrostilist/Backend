package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var globalLogger *zap.Logger

func SetGlobalLogger(logger *zap.Logger) {
	globalLogger = logger
}

func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	logger := getLoggerWithTrace(ctx)
	logger.Debug(msg, fields...)
}

func Info(ctx context.Context, msg string, fields ...zap.Field) {
	logger := getLoggerWithTrace(ctx)
	logger.Info(msg, fields...)
}

func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	logger := getLoggerWithTrace(ctx)
	logger.Warn(msg, fields...)
}

func Error(ctx context.Context, msg string, fields ...zap.Field) {
	logger := getLoggerWithTrace(ctx)
	logger.Error(msg, fields...)
}

func GetGlobal() *zap.Logger {
	return globalLogger
}

func getLoggerWithTrace(ctx context.Context) *zap.Logger {

	if globalLogger == nil {
		panic("loger is not initialized")
	}

	span := trace.SpanFromContext(ctx)
	spanCtx := span.SpanContext()
	if spanCtx.HasTraceID() {
		return GetGlobal().With(
			zap.String("trace_id", spanCtx.TraceID().String()),
			zap.String("span_id", spanCtx.SpanID().String()),
		)
	}

	return GetGlobal()
}
