package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"astroapi/internal/admin"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startMemcached(t *testing.T) *memcache.Client {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "memcached:1.6-alpine",
			ExposedPorts: []string{"11211/tcp"},
			WaitingFor:   wait.ForListeningPort("11211/tcp"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start memcached container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "11211/tcp")
	if err != nil {
		t.Fatalf("get container port: %v", err)
	}

	return memcache.New(fmt.Sprintf("%s:%s", host, port.Port()))
}

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
	cache := startMemcached(t)

	repository := &stubAdminRepository{user: admin.User{
		ID:       "admin-id",
		Email:    "admin@example.com",
		Role:     admin.RoleSuperAdmin,
		IsActive: true,
	}}
	handler := NewAuthHandler(repository, "test-secret", cache)
	handler.now = func() time.Time { return time.Now() }

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
	cache := startMemcached(t)

	repository := &stubAdminRepository{err: admin.ErrInvalidCredential}
	handler := NewAuthHandler(repository, "test-secret", cache)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"bad"}`))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestLoginRevokesOldToken(t *testing.T) {
	cache := startMemcached(t)

	repository := &stubAdminRepository{user: admin.User{
		ID:       "admin-id",
		Email:    "admin@example.com",
		Role:     admin.RoleSuperAdmin,
		IsActive: true,
	}}
	handler := NewAuthHandler(repository, "test-secret", cache)
	handler.now = func() time.Time { return time.Now() }

	loginBody := `{"email":"admin@example.com","password":"password123"}`

	rec1 := httptest.NewRecorder()
	handler.Login(rec1, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody)))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first login: expected status %d, got %d: %s", http.StatusOK, rec1.Code, rec1.Body.String())
	}

	var resp1 struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec1.Body).Decode(&resp1); err != nil {
		t.Fatalf("decode first login response: %v", err)
	}
	oldToken := resp1.Data.AccessToken

	rec2 := httptest.NewRecorder()
	handler.Login(rec2, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second login: expected status %d, got %d: %s", http.StatusOK, rec2.Code, rec2.Body.String())
	}

	protected := AdminAuthMiddleware(cache, "test-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules", nil)
	req.Header.Set("Authorization", "Bearer "+oldToken)
	rec3 := httptest.NewRecorder()
	protected.ServeHTTP(rec3, req)

	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("expected old token to be rejected with %d, got %d", http.StatusUnauthorized, rec3.Code)
	}
}

func TestAdminAuthMiddlewareAcceptsLoginToken(t *testing.T) {
	cache := startMemcached(t)

	jti := uuid.New().String()
	token, err := GenerateAdminAccessToken("admin-id", "admin@example.com", admin.RoleSuperAdmin, "test-secret", jti, time.Now())
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	if err := cache.Set(&memcache.Item{
		Key:        "auth:jti:" + jti,
		Value:      []byte("admin-id"),
		Expiration: int32(adminTokenTTL.Seconds()),
	}); err != nil {
		t.Fatalf("seed jti in cache: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	protected := AdminAuthMiddleware(cache, "test-secret")(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rules", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
}
