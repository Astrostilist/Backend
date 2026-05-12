package handlers

import (
	"net/http"
	"strings"
	"time"
)

func AdminAuthMiddleware(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			staticToken := strings.TrimSpace(adminToken)

			isStaticToken := staticToken != "" && authHeader == "Bearer "+staticToken
			isLoginToken := token != "" && VerifyAdminAccessToken(token, staticToken, time.Now())
			if !isStaticToken && !isLoginToken {
				writeError(w, http.StatusUnauthorized, "admin authorization token is missing or invalid")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
