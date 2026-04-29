package alisa

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientGenerateRetriesOn503(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "gpt://folder/model/latest")

	result, err := client.Generate(context.Background(), "test prompt")

	require.Error(t, err)
	require.Empty(t, result)
	require.Equal(t, int32(4), attempts.Load())
	require.Contains(t, err.Error(), fmt.Sprintf("status=%d", http.StatusServiceUnavailable))
}
