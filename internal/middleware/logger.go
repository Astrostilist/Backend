package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/middleware"
	"go.uber.org/zap"
)

func RequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			duration := time.Since(start)

			logger.Info("HTTP request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Duration("duration", duration),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
			)
		}
		return http.HandlerFunc(fn)
	}
}
