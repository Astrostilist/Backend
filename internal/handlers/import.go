package handlers

import (
	"astroapi/internal/database"
	"astroapi/internal/importer"
	"encoding/json"
	"net/http"
)

func NewImportHandler(db *database.PostgresDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing file field", http.StatusBadRequest)
			return
		}
		defer func() {
			if cerr := file.Close(); cerr != nil {
				http.Error(w, "Failed to close file", http.StatusInternalServerError)
			}
		}()

		result, err := importer.RunImport(r.Context(), db.DB, file)
		if err != nil {
			http.Error(w, "Import failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}
