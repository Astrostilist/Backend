package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		configuredKey  string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "valid bot api key passes",
			configuredKey:  "test-bot-key",
			authHeader:     "Bearer test-bot-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing authorization header is rejected",
			configuredKey:  "test-bot-key",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong bot api key is rejected",
			configuredKey:  "test-bot-key",
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing configured key rejects all requests",
			configuredKey:  "",
			authHeader:     "Bearer test-bot-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "non bearer authorization scheme is rejected",
			configuredKey:  "test-bot-key",
			authHeader:     "Token test-bot-key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler := ClientAuthMiddleware(tt.configuredKey)(next)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/profile", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Fatalf("unexpected status: got %d, want %d", rr.Code, tt.expectedStatus)
			}
		})
	}
}
