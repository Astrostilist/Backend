package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"astroapi/internal/handlers"
	"astroapi/internal/handlers/mocks"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRealHelloService_NilDBReturnsNotInitialized(t *testing.T) {
	t.Parallel()
	svc := handlers.NewRealHelloService(nil)
	require.Equal(t, "not initialized", svc.GetDBStatus(context.Background()))
}

func TestHelloWorldHandler_StatusMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dbStatus   string
		wantStatus string
	}{
		{name: "connected", dbStatus: "connected", wantStatus: "connected"},
		{name: "disconnected", dbStatus: "disconnected", wantStatus: "disconnected"},
		{name: "not initialized", dbStatus: "not initialized", wantStatus: "not initialized"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			service := mocks.NewMockHelloService(ctrl)
			service.EXPECT().GetDBStatus(gomock.Any()).Return(tc.dbStatus).Times(1)

			handler := handlers.NewHelloHandler(service)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/", nil)
			rr := httptest.NewRecorder()

			handler.HelloWorldHandler(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)

			var resp handlers.Response
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			data, ok := resp.Data.(map[string]any)
			require.True(t, ok, "expected data object")
			require.Equal(t, tc.wantStatus, data["database_status"])
		})
	}
}
