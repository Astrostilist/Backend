package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"astroapi/internal/adminlogs"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/go-chi/chi/v5"
)

const (
	defaultAdminLogsLimit = 50
	maxAdminLogsLimit     = 200
)

// AdminLogsHandler обслуживает административные запросы к журналу генераций.
type AdminLogsHandler struct {
	repository adminlogs.Repository
}

type adminLogsListResponse struct {
	Items      []adminlogs.LogEntry `json:"items"`
	TotalCount int                  `json:"total_count"`
}

// NewAdminLogsHandler создает handler административного журнала генераций.
// На вход принимает репозиторий журнала, на выход возвращает готовый handler.
func NewAdminLogsHandler(repository adminlogs.Repository) *AdminLogsHandler {
	return &AdminLogsHandler{repository: repository}
}

// RegisterAdminLogsRoutes регистрирует защищенные admin-роуты журнала генераций.
// На вход принимает chi-роутер, admin token и handler; выходных значений нет.
func RegisterAdminLogsRoutes(router chi.Router, secretTokenAdmin string, cache *memcache.Client, handler *AdminLogsHandler) {
	router.Route("/api/v1/admin/logs", func(router chi.Router) {
		router.Use(AdminAuthMiddleware(cache, secretTokenAdmin))
		router.Get("/", handler.ListLogs)
	})
}

// ListLogs обрабатывает GET /api/v1/admin/logs.
// На вход принимает ResponseWriter и Request, на выход пишет JSON-ответ со списком логов.
func (h *AdminLogsHandler) ListLogs(w http.ResponseWriter, r *http.Request) {
	options, err := parseAdminLogsListOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.repository.List(r.Context(), options)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch admin logs")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Admin logs fetched successfully",
		Data: adminLogsListResponse{
			Items:      sanitizeAdminLogEntries(result.Items),
			TotalCount: result.TotalCount,
		},
	})
}

// parseAdminLogsListOptions парсит query-параметры списка admin logs.
// На вход принимает HTTP-запрос, на выход возвращает опции фильтрации или ошибку.
func parseAdminLogsListOptions(r *http.Request) (adminlogs.ListOptions, error) {
	query := r.URL.Query()

	status := strings.TrimSpace(query.Get("status"))
	if status != "" && !adminlogs.IsValidStatus(status) {
		return adminlogs.ListOptions{}, errors.New("status must be one of: pending, processing, completed, failed")
	}

	fromTime, err := parseOptionalRFC3339(query.Get("from"), "from")
	if err != nil {
		return adminlogs.ListOptions{}, err
	}

	toTime, err := parseOptionalRFC3339(query.Get("to"), "to")
	if err != nil {
		return adminlogs.ListOptions{}, err
	}
	if fromTime != nil && toTime != nil && fromTime.After(*toTime) {
		return adminlogs.ListOptions{}, errors.New("from must be before or equal to to")
	}

	limit, err := parseBoundedPositiveInt(query.Get("limit"), defaultAdminLogsLimit, 1, maxAdminLogsLimit, "limit")
	if err != nil {
		return adminlogs.ListOptions{}, err
	}

	return adminlogs.ListOptions{
		Status: status,
		From:   fromTime,
		To:     toTime,
		Limit:  limit,
	}, nil
}

// parseOptionalRFC3339 парсит необязательную ISO 8601 дату из query-параметра.
// На вход принимает сырое значение и имя поля, на выход возвращает время или ошибку.
func parseOptionalRFC3339(rawValue, fieldName string) (*time.Time, error) {
	rawValue = strings.TrimSpace(rawValue)
	if rawValue == "" {
		return nil, nil
	}

	parsedValue, err := time.Parse(time.RFC3339, rawValue)
	if err != nil {
		return nil, errors.New(fieldName + " must be a valid ISO 8601 timestamp")
	}

	return &parsedValue, nil
}

// sanitizeAdminLogEntries подготавливает записи журнала к HTTP-ответу.
// На вход принимает записи журнала, на выход возвращает копию с обрезанными error_message.
func sanitizeAdminLogEntries(items []adminlogs.LogEntry) []adminlogs.LogEntry {
	result := make([]adminlogs.LogEntry, len(items))
	copy(result, items)

	for i := range result {
		result[i].ErrorMessage = adminlogs.TruncateErrorMessage(result[i].ErrorMessage)
	}

	return result
}
