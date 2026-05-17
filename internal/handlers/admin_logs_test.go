package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"astroapi/internal/adminlogs"
	"astroapi/internal/requests"

	"github.com/go-chi/chi/v5"
)

type fakeAdminLogsRepository struct {
	items       []adminlogs.LogEntry
	called      bool
	lastOptions adminlogs.ListOptions
}

// List возвращает тестовые admin logs с учетом статуса и лимита.
// На вход принимает контекст и опции фильтрации, на выход возвращает список и счетчик.
func (r *fakeAdminLogsRepository) List(_ context.Context, options adminlogs.ListOptions) (adminlogs.ListResult, error) {
	r.called = true
	r.lastOptions = options

	filtered := make([]adminlogs.LogEntry, 0, len(r.items))
	for _, item := range r.items {
		if options.Status != "" && item.Status != options.Status {
			continue
		}
		filtered = append(filtered, item)
	}

	totalCount := len(filtered)
	if options.Limit < len(filtered) {
		filtered = filtered[:options.Limit]
	}

	return adminlogs.ListResult{Items: filtered, TotalCount: totalCount}, nil
}

// newAdminLogsTestMux создает chi-роутер для тестов admin logs.
// На вход принимает репозиторий, на выход возвращает настроенный роутер.
func newAdminLogsTestMux(repository adminlogs.Repository) chi.Router {
	router := chi.NewRouter()
	RegisterAdminLogsRoutes(router, testAdminToken, NewAdminLogsHandler(repository))
	return router
}

// decodeAdminLogsData декодирует data из JSON-ответа admin logs.
// На вход принимает тест и recorder, на выход возвращает data-объект ответа.
func decodeAdminLogsData(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var envelope Response
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatal("expected response data to be an object")
	}

	return data
}

// TestListAdminLogsFiltersByFailedStatus проверяет фильтр status=failed.
// На вход принимает тестовый контекст, на выход проверяет HTTP-ответ handler.
func TestListAdminLogsFiltersByFailedStatus(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := &fakeAdminLogsRepository{items: []adminlogs.LogEntry{
		{RequestID: "req-1", UserID: "user-1", Status: requests.StatusFailed, CreatedAt: now},
		{RequestID: "req-2", UserID: "user-2", Status: requests.StatusCompleted, CreatedAt: now},
	}}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?status=failed", nil)
	request.Header.Set("Authorization", "Bearer "+testAdminBearerToken(t))
	response := httptest.NewRecorder()

	newAdminLogsTestMux(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	data := decodeAdminLogsData(t, response)
	items, ok := data["items"].([]any)
	if !ok {
		t.Fatal("expected items to be a list")
	}
	if len(items) != 1 {
		t.Fatalf("expected one failed item, got %d", len(items))
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatal("expected first item to be an object")
	}
	if item["status"] != requests.StatusFailed {
		t.Fatalf("expected failed status, got %v", item["status"])
	}
	if data["total_count"] != float64(1) {
		t.Fatalf("expected total_count=1, got %v", data["total_count"])
	}
}

// TestListAdminLogsRejectsLimitOverMax проверяет ошибку limit=201.
// На вход принимает тестовый контекст, на выход проверяет статус 400 и отсутствие вызова репозитория.
func TestListAdminLogsRejectsLimitOverMax(t *testing.T) {
	t.Parallel()

	repository := &fakeAdminLogsRepository{}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?limit=201", nil)
	request.Header.Set("Authorization", "Bearer "+testAdminBearerToken(t))
	response := httptest.NewRecorder()

	newAdminLogsTestMux(repository).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	if repository.called {
		t.Fatal("repository must not be called for invalid limit")
	}
}

// TestListAdminLogsTruncatesErrorMessage проверяет обрезку error_message в HTTP-ответе.
// На вход принимает тестовый контекст, на выход проверяет длину error_message.
func TestListAdminLogsTruncatesErrorMessage(t *testing.T) {
	t.Parallel()

	repository := &fakeAdminLogsRepository{items: []adminlogs.LogEntry{
		{
			RequestID:    "req-1",
			UserID:       "user-1",
			Status:       requests.StatusFailed,
			ErrorMessage: strings.Repeat("x", 501),
			CreatedAt:    time.Now().UTC(),
		},
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil)
	request.Header.Set("Authorization", "Bearer "+testAdminBearerToken(t))
	response := httptest.NewRecorder()

	newAdminLogsTestMux(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	data := decodeAdminLogsData(t, response)
	items := data["items"].([]any)
	item := items[0].(map[string]any)
	errorMessage, ok := item["error_message"].(string)
	if !ok {
		t.Fatal("expected error_message to be a string")
	}
	if len([]rune(errorMessage)) != 500 {
		t.Fatalf("expected error_message length 500, got %d", len([]rune(errorMessage)))
	}
}
