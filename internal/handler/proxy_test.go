package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bgruszka/contextforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConfig creates a valid test configuration with all required fields
func testConfig(targetHost string, headers []string) *config.ProxyConfig {
	return &config.ProxyConfig{
		HeadersToPropagate: headers,
		TargetHost:         targetHost,
		ProxyPort:          9090,
		LogLevel:           "info",
		MetricsPort:        9091,
		ReadTimeout:        15 * time.Second,
		WriteTimeout:       15 * time.Second,
		IdleTimeout:        60 * time.Second,
		ReadHeaderTimeout:  5 * time.Second,
		TargetDialTimeout:  2 * time.Second,
	}
}

func TestNewProxyHandler(t *testing.T) {
	cfg := testConfig("localhost:8080", []string{"x-request-id", "x-dev-id"})

	handler, err := NewProxyHandler(cfg)

	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, cfg, handler.config)
	assert.NotNil(t, handler.reverseProxy)
	assert.Equal(t, []string{"x-request-id", "x-dev-id"}, handler.headers)
}

func TestNewProxyHandler_ValidTargetHost(t *testing.T) {
	// Test that various valid host formats work
	validHosts := []string{
		"localhost:8080",
		"127.0.0.1:8080",
		"example.com:80",
		"service.namespace.svc.cluster.local:8080",
	}

	for _, host := range validHosts {
		t.Run(host, func(t *testing.T) {
			cfg := testConfig(host, []string{"x-request-id"})
			handler, err := NewProxyHandler(cfg)
			require.NoError(t, err)
			assert.NotNil(t, handler)
		})
	}
}

func TestProxyHandler_ExtractHeaders(t *testing.T) {
	cfg := testConfig("localhost:8080", []string{"x-request-id", "x-dev-id", "x-tenant-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-Id", "abc123")
	req.Header.Set("X-Dev-Id", "john")
	req.Header.Set("X-Other-Header", "should-be-ignored")

	headers := handler.extractHeaders(req)

	assert.Len(t, headers, 2)
	assert.Equal(t, "abc123", headers["X-Request-Id"])
	assert.Equal(t, "john", headers["X-Dev-Id"])
	assert.NotContains(t, headers, "X-Other-Header")
	assert.NotContains(t, headers, "X-Tenant-Id")
}

func TestProxyHandler_ExtractHeaders_CaseInsensitive(t *testing.T) {
	cfg := testConfig("localhost:8080", []string{"X-Request-ID"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("x-request-id", "abc123")

	headers := handler.extractHeaders(req)

	assert.Len(t, headers, 1)
	assert.Equal(t, "abc123", headers["X-Request-Id"])
}

func TestProxyHandler_ExtractHeaders_EmptyHeaders(t *testing.T) {
	cfg := testConfig("localhost:8080", []string{"x-request-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	headers := handler.extractHeaders(req)

	assert.Empty(t, headers)
}

func TestProxyHandler_ServeHTTP(t *testing.T) {
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "abc123", r.Header.Get("X-Request-Id"))
		assert.Equal(t, "john", r.Header.Get("X-Dev-Id"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id", "x-dev-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-Id", "abc123")
	req.Header.Set("X-Dev-Id", "john")

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	body, _ := io.ReadAll(rr.Body)
	assert.Equal(t, "OK", string(body))
}

func TestGetHeadersFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected map[string]string
	}{
		{
			name: "headers present",
			ctx: context.WithValue(context.Background(), ContextKeyHeaders, map[string]string{
				"X-Request-Id": "abc123",
				"X-Dev-Id":     "john",
			}),
			expected: map[string]string{
				"X-Request-Id": "abc123",
				"X-Dev-Id":     "john",
			},
		},
		{
			name:     "headers not present",
			ctx:      context.Background(),
			expected: nil,
		},
		{
			name:     "wrong type in context",
			ctx:      context.WithValue(context.Background(), ContextKeyHeaders, "invalid"),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetHeadersFromContext(tt.ctx)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestProxyHandler_HeadersPropagatedThroughProxy(t *testing.T) {
	var receivedHeaders http.Header

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id", "x-correlation-id", "x-tenant-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)
	req.Header.Set("X-Request-Id", "req-12345")
	req.Header.Set("X-Correlation-Id", "corr-67890")
	req.Header.Set("X-Tenant-Id", "tenant-abc")
	req.Header.Set("Authorization", "Bearer token")

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "req-12345", receivedHeaders.Get("X-Request-Id"))
	assert.Equal(t, "corr-67890", receivedHeaders.Get("X-Correlation-Id"))
	assert.Equal(t, "tenant-abc", receivedHeaders.Get("X-Tenant-Id"))
	assert.Equal(t, "Bearer token", receivedHeaders.Get("Authorization"))
}
