package handlers

import (
	"net/http"
	"strings"
	"time"
)

func AdminAuthMiddleware(tokenSecret string) func(http.Handler) http.Handler {
	secret := strings.TrimSpace(tokenSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			parts := strings.Fields(authHeader)
			isBearerToken := len(parts) == 2 && strings.EqualFold(parts[0], "Bearer")

			if !isBearerToken || !VerifyAdminAccessToken(parts[1], secret, time.Now()) {
				writeError(w, http.StatusUnauthorized, "admin authorization token is missing or invalid")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
