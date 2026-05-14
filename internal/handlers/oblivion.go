package handlers

import (
	"astroapi/internal/logger"
	"astroapi/internal/repositories"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Handler struct {
	Repo   repositories.UserRepository
	Logger *zap.Logger
}

func (h *Handler) OblivionHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.NewWithTraceMetadata(r.Context(), h.Logger)

	userID := strings.TrimSpace(chi.URLParam(r, "user_id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	found, err := h.Repo.DeleteUsers(r.Context(), userID)
	if err != nil {
		log.Error("failed to delete user", zap.String("user_id", userID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	// проверка было ли что-то удалено
	if !found {
		log.Warn("user not found", zap.String("user_id", userID))
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	log.Info("user deleted successfully", zap.String("user_id", userID), zap.Time("timestamp", time.Now().UTC()))

	w.WriteHeader(http.StatusNoContent)
}
