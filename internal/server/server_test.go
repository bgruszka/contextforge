package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bgruszka/contextforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHandler struct{}

func (m *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("proxied"))
}

func TestNewServer(t *testing.T) {
	cfg := &config.ProxyConfig{
		HeadersToPropagate: []string{"x-request-id"},
		TargetHost:         "localhost:8080",
		ProxyPort:          9090,
		LogLevel:           "info",
		MetricsPort:        9091,
	}

	srv := NewServer(cfg, &mockHandler{})

	assert.NotNil(t, srv)
	assert.NotNil(t, srv.httpServer)
	assert.NotNil(t, srv.mux)
	assert.Equal(t, ":9090", srv.httpServer.Addr)
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.NotEmpty(t, response.Timestamp)

	_, err = time.Parse(time.RFC3339, response.Timestamp)
	assert.NoError(t, err, "Timestamp should be in RFC3339 format")
}

func TestReadyHandler_TargetReachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	targetHost := listener.Addr().String()

	handler := readyHandler(targetHost)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response ReadyResponse
	err = json.NewDecoder(rr.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "ready", response.Status)
	assert.Equal(t, targetHost, response.TargetHost)
	assert.True(t, response.TargetReachable)
}

func TestReadyHandler_TargetNotReachable(t *testing.T) {
	targetHost := "127.0.0.1:59999"

	handler := readyHandler(targetHost)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var response ReadyResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "not_ready", response.Status)
	assert.Equal(t, targetHost, response.TargetHost)
	assert.False(t, response.TargetReachable)
}

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := &config.ProxyConfig{
		HeadersToPropagate: []string{"x-request-id"},
		TargetHost:         "localhost:8080",
		ProxyPort:          0,
		LogLevel:           "info",
		MetricsPort:        9091,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	cfg.ProxyPort = listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	srv := NewServer(cfg, &mockHandler{})

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	assert.NoError(t, err)

	select {
	case err := <-serverErr:
		assert.Equal(t, http.ErrServerClosed, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server did not shut down in time")
	}
}

func TestServer_RoutesRequests(t *testing.T) {
	cfg := &config.ProxyConfig{
		HeadersToPropagate: []string{"x-request-id"},
		TargetHost:         "localhost:8080",
		ProxyPort:          9090,
		LogLevel:           "info",
		MetricsPort:        9091,
	}

	srv := NewServer(cfg, &mockHandler{})

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "health endpoint",
			path:           "/healthz",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "ready endpoint",
			path:           "/ready",
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "proxy route",
			path:           "/api/v1/test",
			expectedStatus: http.StatusOK,
			expectedBody:   "proxied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			srv.mux.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, rr.Body.String())
			}
		})
	}
}

func TestCheckTargetReachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	assert.True(t, checkTargetReachable(listener.Addr().String()))

	assert.False(t, checkTargetReachable("127.0.0.1:59999"))
}
