// Package adminlogs описывает чтение административного журнала генераций.
package adminlogs

import (
	"context"
	"time"

	"astroapi/internal/requests"
)

const maxErrorMessageLength = 500

var allowedStatuses = map[string]struct{}{
	requests.StatusPending:    {},
	requests.StatusProcessing: {},
	requests.StatusRetry:      {},
	requests.StatusCompleted:  {},
	requests.StatusFailed:     {},
}

// LogEntry описывает одну запись журнала генераций для административного API.
type LogEntry struct {
	RequestID    string     `json:"request_id"`
	UserID       string     `json:"user_id"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at"`
}

// ListOptions задает фильтры и лимит выборки журнала.
type ListOptions struct {
	Status string
	From   *time.Time
	To     *time.Time
	Limit  int
}

// ListResult содержит записи журнала и общее количество по фильтрам.
type ListResult struct {
	Items      []LogEntry
	TotalCount int
}

// Repository описывает чтение журнала генераций из хранилища.
type Repository interface {
	List(ctx context.Context, options ListOptions) (ListResult, error)
}

// IsValidStatus проверяет, входит ли статус в разрешенный набор.
// На вход принимает строковый статус, на выход возвращает true для допустимого значения.
func IsValidStatus(status string) bool {
	_, ok := allowedStatuses[status]
	return ok
}

// TruncateErrorMessage обрезает текст ошибки до лимита административного ответа.
// На вход принимает исходную ошибку, на выход возвращает строку длиной не больше 500 символов.
func TruncateErrorMessage(message string) string {
	runes := []rune(message)
	if len(runes) <= maxErrorMessageLength {
		return message
	}

	return string(runes[:maxErrorMessageLength])
}
