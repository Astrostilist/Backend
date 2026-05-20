package middleware

import (
	astrologger "astroapi/internal/logger"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// RequestLogger логирует каждое HTTP-запрос после обработки. Если handler не
// вызвал WriteHeader, логируем HTTP 200 (то, что реально увидит клиент).
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}

		if r.URL.Path == "/metrics" {
			return
		}

		astrologger.Info(r.Context(), "HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", status),
			zap.Duration("duration", time.Since(start)),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
		)
	})
}
