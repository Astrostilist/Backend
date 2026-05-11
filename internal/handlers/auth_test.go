package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"astroapi/internal/admin"
)

type stubAdminRepository struct {
	user admin.User
	err  error
}

func (r *stubAdminRepository) HasSuperAdmin(ctx context.Context) (bool, error) {
	return false, nil
}

func (r *stubAdminRepository) CreateSuperAdmin(ctx context.Context, input admin.CreateSuperAdminInput) (admin.User, error) {
	return admin.User{}, nil
}

func (r *stubAdminRepository) VerifyCredentials(ctx context.Context, email, password string) (admin.User, error) {
	if r.err != nil {
		return admin.User{}, r.err
	}
	return r.user, nil
}

func TestAuthHandlerLoginReturnsBearerToken(t *testing.T) {
	repository := &stubAdminRepository{user: admin.User{
		ID:       "admin-id",
		Email:    "admin@example.com",
		Role:     admin.RoleSuperAdmin,
		IsActive: true,
	}}
	handler := NewAuthHandler(repository, "test-secret")
	handler.now = func() time.Time { return time.Unix(100, 0) }

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"password123"}`))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "access_token") {
		t.Fatalf("expected access_token in response: %s", rec.Body.String())
	}
}

func TestAuthHandlerLoginRejectsInvalidCredentials(t *testing.T) {
	repository := &stubAdminRepository{err: admin.ErrInvalidCredential}
	handler := NewAuthHandler(repository, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"bad"}`))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAdminAuthMiddlewareAcceptsLoginToken(t *testing.T) {
	token, err := GenerateAdminAccessToken("admin-id", "admin@example.com", admin.RoleSuperAdmin, "test-secret", time.Now())
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	protected := AdminAuthMiddleware("test-secret")(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
}
