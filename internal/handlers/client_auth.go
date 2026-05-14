package handlers

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

func ClientAuthMiddleware(botAPIKey string) func(http.Handler) http.Handler {
	expectedToken := strings.TrimSpace(botAPIKey)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearerToken(r.Header.Get("Authorization"))
			isValid := ok && expectedToken != "" && constantTimeEqual(token, expectedToken)
			if !isValid {
				writeError(w, http.StatusUnauthorized, "bot api key is missing or invalid")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractBearerToken(authHeader string) (string, bool) {
	authHeader = strings.TrimSpace(authHeader)
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerPrefix))
	return token, token != ""
}

func constantTimeEqual(actual, expected string) bool {
	if len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
