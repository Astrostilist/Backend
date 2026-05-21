package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"

	"astroapi/internal/repositories"
)

func TestOblivionHandler_UserNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repositories.NewMockUserRepository(ctrl)

	//Настраиваем поведение
	mockRepo.EXPECT().
		DeleteUsers(gomock.Any(), "user_123").
		Return(false, nil)

	testLogger := zaptest.NewLogger(t)
	defer testLogger.Sync()

	handler := &Handler{
		Repo:   mockRepo,
		Logger: testLogger,
	}

	r := chi.NewRouter()
	r.Delete("/api/v1/user/{user_id}", handler.OblivionHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/user/user_123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}
