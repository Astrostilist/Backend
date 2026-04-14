package handlers

import (
	"astroapi/internal/handlers/mocks"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/mock/gomock"
	"astroapi/internal/database"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHelloWorldHandler проверяет работу базового эндпоинта
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

	// 1. Проверяем успешный GET-запрос
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(HelloWorldHandler)

	handler.ServeHTTP(rr, req)

	// Ожидаем 200 OK
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Ожидали статус %v, получили %v", http.StatusOK, status)
	}

	// 2. Проверяем защиту от других методов (например, POST)
	reqPost, _ := http.NewRequest(http.MethodPost, "/api/v1/", nil)
	rrPost := httptest.NewRecorder()

	handler.ServeHTTP(rrPost, reqPost)

	// Ожидаем 405 Method Not Allowed
	if status := rrPost.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("Ожидали статус %v, получили %v", http.StatusMethodNotAllowed, status)
	}
}

func TestHelloWorldHandler_DBDisconnected(t *testing.T) {
	// Имитируем наличие базы, но без реального подключения
	database.DB = &database.PostgresDB{}

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/", nil)
	rr := httptest.NewRecorder()
	http.HandlerFunc(HelloWorldHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Ожидали статус %v, получили %v", http.StatusOK, status)
	}

	database.DB = nil // Очищаем за собой
}
