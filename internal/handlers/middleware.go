package handlers

import (
	"net/http"
	"strings"
)

func AdminAuthMiddleware(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		expectedHeader := "Bearer " + adminToken

		if adminToken == "" || authHeader == "" || authHeader != expectedHeader {
			writeError(w, http.StatusUnauthorized, "admin authorization token is missing or invalid")
			return
		}

		next.ServeHTTP(w, r)
	})
}
