package retailcrm

import (
	astrologger "astroapi/internal/infrastructure/logger"
	"astroapi/internal/repositories/domain"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RetailCRMClient — клиент для взаимодействия с RetailCRM API.
type RetailCRMClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// CustomerResponse — упрощенная структура ответа для GetCustomer.
type CustomerResponse struct {
	Success  bool           `json:"success"`
	Customer map[string]any `json:"customer,omitempty"`
}

// NewClient создает и настраивает новый экземпляр клиента RetailCRM.
func NewClient(baseURL, apiKey string) *RetailCRMClient {
	return &RetailCRMClient{
		baseURL: strings.TrimRight(baseURL, "/"), // Убираем слэш на конце, если он есть
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendProfile отправляет данные астропрофиля.
func (c *RetailCRMClient) SendProfile(ctx context.Context, profile domain.AstroProfile) error {
	// TODO:  implement method
	return nil
}

// SendRecommend отправляет данные рекомендаций.
func (c *RetailCRMClient) SendRecommend(ctx context.Context, recommend string) error {
	// TODO:  implement method
	return nil
}

// doRequest выполняет HTTP-запрос, инжектит X-API-KEY и логирует endpoint.
func (c *RetailCRMClient) doRequest(ctx context.Context, method, endpoint string, payload []byte) ([]byte, error) {
	var bodyReader io.Reader
	if len(payload) > 0 {
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Аутентификация через заголовок (по требованию DoD)
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Логирование согласно спецификации
	astrologger.Debug(ctx, "retailcrm request",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error during retailcrm request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			astrologger.Warn(ctx, "failed to close response body", zap.Error(closeErr))
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read retailcrm response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("retailcrm returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GetCustomer запрашивает данные покупателя по externalID.
func (c *RetailCRMClient) GetCustomer(ctx context.Context, externalID string) (*CustomerResponse, error) {
	endpoint := fmt.Sprintf("/api/v5/customers?externalId=%s", externalID)

	respBody, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result CustomerResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal retailcrm response: %w", err)
	}

	return &result, nil
}

// UpdateCustomer обновляет кастомные поля покупателя.
func (c *RetailCRMClient) UpdateCustomer(ctx context.Context, externalID string, customFields map[string]any) error {
	endpoint := fmt.Sprintf("/api/v5/customers/%s/edit", externalID)

	// Формируем payload. В RetailCRM данные обычно оборачиваются в объект customer
	payloadData := map[string]any{
		"customer": map[string]any{
			"externalId":   externalID,
			"customFields": customFields,
		},
	}

	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		return fmt.Errorf("failed to marshal customer payload: %w", err)
	}

	_, err = c.doRequest(ctx, http.MethodPost, endpoint, payloadBytes)
	return err
}
