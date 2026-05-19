package middleware

import (
	"context"
	"net/http"

	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TraceMiddleware — оборачивает handler в span
func TraceMiddleware(serviceName string) func(http.Handler) http.Handler {
	return otelchi.Middleware(serviceName)
}

// ZapWithTrace — добавляет trace_id в logger из контекста
func ZapWithTrace(logger *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			traceID := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
			reqLogger := logger.With(zap.String("trace_id", traceID))
			ctx = context.WithValue(ctx, "logger", reqLogger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
