package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"astroapi/internal/handlers/mocks"

	"go.uber.org/mock/gomock"
)

// TestHelloWorldHandler проверяет работу базового эндпоинта (БД подключена)
func TestHelloWorldHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Создаем МОК
	mockService := mocks.NewMockHelloService(ctrl)

	// ПРОГРАММИРУЕМ МОК:
	mockService.EXPECT().GetDBStatus().Return("connected").Times(1)

	handler := NewHelloHandler(mockService)

	// Эмулируем HTTP-запрос
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/", nil)
	rr := httptest.NewRecorder()

	handler.HelloWorldHandler(rr, req)

	// Проверяем, что хендлер не упал и вернул 200 OK
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Ожидали статус %v, получили %v", http.StatusOK, status)
	}
}

// TestHelloWorldHandler_DBDisconnected проверяет работу, когда БД отключена
func TestHelloWorldHandler_DBDisconnected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Создаем МОК
	mockService := mocks.NewMockHelloService(ctrl)

	// ПРОГРАММИРУЕМ МОК:
	mockService.EXPECT().GetDBStatus().Return("disconnected").Times(1)

	handler := NewHelloHandler(mockService)

	// Эмулируем HTTP-запрос
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/", nil)
	rr := httptest.NewRecorder()

	handler.HelloWorldHandler(rr, req)

	// Проверяем, что хендлер не упал и вернул 200 OK
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Ожидали статус %v, получили %v", http.StatusOK, status)
	}
}
