package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// ConsentGuard проверяет consent_given в JSON-теле запроса.
func ConsentGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ConsentGiven bool `json:"consent_given"`
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		/*if !req.ConsentGiven {
		    http.Error(w, "consent required", http.StatusForbidden)
		    return
		}*/

		// Восстанавливаем тело для следующих обработчиков
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		next.ServeHTTP(w, r)
	})
}
