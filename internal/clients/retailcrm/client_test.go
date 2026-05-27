package retailcrm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestRetailCRMClient_GetCustomer(t *testing.T) {
	expectedAPIKey := "secret-api-key-123"
	expectedExternalID := "ext-456"

	// 1. Создаем мок-сервер RetailCRM
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем наличие ключа авторизации (DoD: Клиент успешно авторизуется с X-API-KEY)
		if key := r.Header.Get("X-API-KEY"); key != expectedAPIKey {
			t.Errorf("Expected X-API-KEY %q, got %q", expectedAPIKey, key)
		}

		// Проверяем метод и endpoint
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true, "customer": {"id": 1, "externalId": "ext-456"}}`))
	}))
	defer mockServer.Close()

	// 2. Инициализируем клиента
	// zaptest.NewLogger пишет логи прямо в консоль тестов, позволяя проверить вызов logger.Debug
	logger := zaptest.NewLogger(t)
	client := NewClient(mockServer.URL, expectedAPIKey, logger)

	// 3. Выполняем тестируемый метод
	resp, err := client.GetCustomer(context.Background(), expectedExternalID)

	// 4. Проверяем результаты
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Expected success to be true")
	}
	if resp.Customer["externalId"] != expectedExternalID {
		t.Errorf("Expected customer externalId to be %q, got %v", expectedExternalID, resp.Customer["externalId"])
	}
}

func TestRetailCRMClient_UpdateCustomer(t *testing.T) {
	expectedAPIKey := "update-key-789"
	expectedExternalID := "ext-789"
	customFields := map[string]any{"zodiac_sign": "aries"}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := r.Header.Get("X-API-KEY"); key != expectedAPIKey {
			t.Errorf("Expected X-API-KEY %q, got %q", expectedAPIKey, key)
		}

		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// Читаем payload и проверяем переданные поля
		var payload map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		customerData := payload["customer"]
		fields := customerData["customFields"].(map[string]any)
		if fields["zodiac_sign"] != "aries" {
			t.Errorf("Expected customField zodiac_sign=aries, got %v", fields["zodiac_sign"])
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer mockServer.Close()

	logger := zaptest.NewLogger(t)
	client := NewClient(mockServer.URL, expectedAPIKey, logger)

	err := client.UpdateCustomer(context.Background(), expectedExternalID, customFields)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}
