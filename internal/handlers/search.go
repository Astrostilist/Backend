package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"astroapi/internal/database"
	"astroapi/internal/products"
)

func SearchProductsHandler(db *database.PostgresDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tagsParam := r.URL.Query().Get("tags")
		if tagsParam == "" {
			http.Error(w, "Missing tags parameter", http.StatusBadRequest)
			return
		}
		tags := strings.Split(tagsParam, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}

		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil || limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}

		result, err := products.FindByTags(r.Context(), db.DB, tags, limit, offset)
		if err != nil {
			http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}
