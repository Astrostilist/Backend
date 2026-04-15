package handlers

import (
    "encoding/json"
    "net/http"
    "astroapi/internal/database"
    "astroapi/internal/importer"
)

func ImportCatalogHandler(w http.ResponseWriter, r *http.Request) {
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
        if err := file.Close(); err != nil {
            http.Error(w, "Failed to close file", http.StatusInternalServerError)
        }
    }()

    result, err := importer.RunImport(r.Context(), database.DB.DB, file)
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