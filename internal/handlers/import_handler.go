package handlers

import (
	"astroapi/internal/importer"
	"net/http"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type AdminImportHandler struct {
	repository importer.Repository
	logger     *zap.Logger
}

func NewAdminImportHandler(repository importer.Repository) *AdminImportHandler {
	return &AdminImportHandler{repository: repository}
}

func RegisterAdminImportRoutes(router chi.Router, secretTokenAdmin string, cache *memcache.Client, handler *AdminImportHandler) {
	router.Route("/api/v1/admin/catalog/import", func(router chi.Router) {
		router.Use(AdminAuthMiddleware(cache, secretTokenAdmin))
		router.Post("/", handler.StartImport)
	})
}

func (h *AdminImportHandler) StartImport(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field: "+err.Error())
		//http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}

	defer func() {
		if cerr := file.Close(); cerr != nil {
			writeError(w, http.StatusInternalServerError, "failed to close file: "+err.Error())
			// http.Error(w, "failed to close file", http.StatusInternalServerError)
		}
	}()

	result, err := h.repository.RunImportCSV(r.Context(), file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "import failed:: "+err.Error())
		// http.Error(w, "import failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "catalog parsed successfully",
		Data:    map[string]any{"result": result},
	})

}

// func NewImportHandler(db *database.PostgresDB) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		if r.Method != http.MethodPost {
// 			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
// 			return
// 		}

// 		err := r.ParseMultipartForm(32 << 20)
// 		if err != nil {
// 			http.Error(w, "Failed to parse form", http.StatusBadRequest)
// 			return
// 		}

// 		file, _, err := r.FormFile("file")
// 		if err != nil {
// 			http.Error(w, "Missing file field", http.StatusBadRequest)
// 			return
// 		}
// 		defer func() {
// 			if cerr := file.Close(); cerr != nil {
// 				http.Error(w, "Failed to close file", http.StatusInternalServerError)
// 			}
// 		}()

// 		result, err := importer.RunImport(r.Context(), db.DB, file)
// 		if err != nil {
// 			http.Error(w, "Import failed: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		w.Header().Set("Content-Type", "application/json")
// 		if err := json.NewEncoder(w).Encode(result); err != nil {
// 			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
// 			return
// 		}
// 	}
// }
