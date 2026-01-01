package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/bgruszka/contextforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConfig creates a valid test configuration with all required fields
func testConfig(targetHost string, headers []string) *config.ProxyConfig {
	// Create HeaderRules from headers for the new extractHeaders logic
	rules := make([]config.HeaderRule, len(headers))
	for i, h := range headers {
		rules[i] = config.HeaderRule{
			Name:      h,
			Propagate: true,
		}
	}
	return &config.ProxyConfig{
		HeadersToPropagate: headers,
		HeaderRules:        rules,
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

func TestProxyHandler_HeaderGeneration(t *testing.T) {
	cfg := &config.ProxyConfig{
		HeadersToPropagate: []string{"x-request-id"},
		HeaderRules: []config.HeaderRule{
			{
				Name:          "x-request-id",
				Generate:      true,
				GeneratorType: "uuid",
				Propagate:     true,
			},
		},
		TargetHost:        "localhost:8080",
		ProxyPort:         9090,
		LogLevel:          "info",
		MetricsPort:       9091,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		TargetDialTimeout: 2 * time.Second,
	}

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	// Request without the header - should be generated
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	headers := handler.extractHeaders(req)

	// Should have generated a UUID
	assert.Len(t, headers, 1)
	assert.NotEmpty(t, headers["X-Request-Id"])
	// UUID format: 8-4-4-4-12
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, headers["X-Request-Id"])
}

func TestProxyHandler_HeaderGenerationPreservesExisting(t *testing.T) {
	cfg := &config.ProxyConfig{
		HeadersToPropagate: []string{"x-request-id"},
		HeaderRules: []config.HeaderRule{
			{
				Name:          "x-request-id",
				Generate:      true,
				GeneratorType: "uuid",
				Propagate:     true,
			},
		},
		TargetHost:        "localhost:8080",
		ProxyPort:         9090,
		LogLevel:          "info",
		MetricsPort:       9091,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		TargetDialTimeout: 2 * time.Second,
	}

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	// Request with the header already set - should NOT be overwritten
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-Id", "existing-value")
	headers := handler.extractHeaders(req)

	assert.Len(t, headers, 1)
	assert.Equal(t, "existing-value", headers["X-Request-Id"])
}

func TestProxyHandler_PathFiltering(t *testing.T) {
	cfg := &config.ProxyConfig{
		HeadersToPropagate: []string{"x-request-id"},
		HeaderRules: []config.HeaderRule{
			{
				Name:              "x-request-id",
				Propagate:         true,
				PathRegex:         "^/api/.*",
				CompiledPathRegex: mustCompileRegex("^/api/.*"),
			},
		},
		TargetHost:        "localhost:8080",
		ProxyPort:         9090,
		LogLevel:          "info",
		MetricsPort:       9091,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		TargetDialTimeout: 2 * time.Second,
	}

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	// Request matching path pattern
	reqMatch := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	reqMatch.Header.Set("X-Request-Id", "abc123")
	headersMatch := handler.extractHeaders(reqMatch)
	assert.Len(t, headersMatch, 1)
	assert.Equal(t, "abc123", headersMatch["X-Request-Id"])

	// Request NOT matching path pattern
	reqNoMatch := httptest.NewRequest(http.MethodGet, "/health", nil)
	reqNoMatch.Header.Set("X-Request-Id", "abc123")
	headersNoMatch := handler.extractHeaders(reqNoMatch)
	assert.Len(t, headersNoMatch, 0)
}

func TestProxyHandler_MethodFiltering(t *testing.T) {
	cfg := &config.ProxyConfig{
		HeadersToPropagate: []string{"x-request-id"},
		HeaderRules: []config.HeaderRule{
			{
				Name:      "x-request-id",
				Propagate: true,
				Methods:   []string{"POST", "PUT"},
			},
		},
		TargetHost:        "localhost:8080",
		ProxyPort:         9090,
		LogLevel:          "info",
		MetricsPort:       9091,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		TargetDialTimeout: 2 * time.Second,
	}

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	// POST request - should propagate
	reqPost := httptest.NewRequest(http.MethodPost, "/api/users", nil)
	reqPost.Header.Set("X-Request-Id", "abc123")
	headersPost := handler.extractHeaders(reqPost)
	assert.Len(t, headersPost, 1)

	// GET request - should NOT propagate
	reqGet := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	reqGet.Header.Set("X-Request-Id", "abc123")
	headersGet := handler.extractHeaders(reqGet)
	assert.Len(t, headersGet, 0)
}

func mustCompileRegex(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
