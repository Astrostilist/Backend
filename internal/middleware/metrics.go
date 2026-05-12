package middleware

import (
	"astroapi/internal/metrics"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func RequestMetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			duration := time.Since(start).Seconds()

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			metrics.ObserveMessageProcessingDuration(r.Method, r.URL.Path, fmt.Sprintf("%d", status), duration)
		})
	}
}
