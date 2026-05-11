package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"astroapi/internal/admin"
)

type AuthHandler struct {
	repository  admin.Repository
	tokenSecret string
	now         func() time.Time
}

type adminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type adminLoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func NewAuthHandler(repository admin.Repository, tokenSecret string) *AuthHandler {
	return &AuthHandler{
		repository:  repository,
		tokenSecret: strings.TrimSpace(tokenSecret),
		now:         time.Now,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.repository == nil {
		writeError(w, http.StatusInternalServerError, "auth repository is not configured")
		return
	}
	if h.tokenSecret == "" {
		writeError(w, http.StatusInternalServerError, "admin token secret is not configured")
		return
	}

	var payload adminLoginRequest

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid login request")
		return
	}

	user, err := h.repository.VerifyCredentials(r.Context(), payload.Email, payload.Password)
	if err != nil {
		if errors.Is(err, admin.ErrInvalidCredential) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to verify admin credentials")
		return
	}

	accessToken, err := GenerateAdminAccessToken(user.ID, user.Email, user.Role, h.tokenSecret, h.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate admin access token")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Admin authenticated successfully",
		Data: adminLoginResponse{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			ExpiresIn:   int64(adminTokenTTL.Seconds()),
		},
	})
}
