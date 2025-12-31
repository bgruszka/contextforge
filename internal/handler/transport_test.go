package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRoundTripper struct {
	fn func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.fn(req)
}

func TestNewHeaderPropagatingTransport(t *testing.T) {
	headers := []string{"x-request-id", "x-dev-id"}

	transport := NewHeaderPropagatingTransport(headers, nil)

	assert.NotNil(t, transport)
	assert.Equal(t, headers, transport.headers)
	assert.Equal(t, http.DefaultTransport, transport.baseTransport)
}

func TestNewHeaderPropagatingTransport_WithCustomBase(t *testing.T) {
	headers := []string{"x-request-id"}
	customBase := &mockRoundTripper{}

	transport := NewHeaderPropagatingTransport(headers, customBase)

	assert.Equal(t, customBase, transport.baseTransport)
}

func TestHeaderPropagatingTransport_RoundTrip_InjectsHeaders(t *testing.T) {
	headerMap := map[string]string{
		"X-Request-Id": "abc123",
		"X-Dev-Id":     "john",
	}

	ctx := context.WithValue(context.Background(), ContextKeyHeaders, headerMap)

	mockTransport := &mockRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "abc123", r.Header.Get("X-Request-Id"))
			assert.Equal(t, "john", r.Header.Get("X-Dev-Id"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}

	transport := NewHeaderPropagatingTransport([]string{"x-request-id", "x-dev-id"}, mockTransport)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req = req.WithContext(ctx)

	resp, err := transport.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHeaderPropagatingTransport_RoundTrip_DoesNotOverwriteExisting(t *testing.T) {
	headerMap := map[string]string{
		"X-Request-Id": "from-context",
	}

	ctx := context.WithValue(context.Background(), ContextKeyHeaders, headerMap)

	mockTransport := &mockRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "already-set", r.Header.Get("X-Request-Id"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}

	transport := NewHeaderPropagatingTransport([]string{"x-request-id"}, mockTransport)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("X-Request-Id", "already-set")
	req = req.WithContext(ctx)

	resp, err := transport.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHeaderPropagatingTransport_RoundTrip_NoHeadersInContext(t *testing.T) {
	mockTransport := &mockRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			assert.Empty(t, r.Header.Get("X-Request-Id"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}

	transport := NewHeaderPropagatingTransport([]string{"x-request-id"}, mockTransport)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)

	resp, err := transport.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHeaderPropagatingTransport_RoundTrip_EmptyHeaderMap(t *testing.T) {
	headerMap := map[string]string{}
	ctx := context.WithValue(context.Background(), ContextKeyHeaders, headerMap)

	mockTransport := &mockRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}

	transport := NewHeaderPropagatingTransport([]string{"x-request-id"}, mockTransport)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req = req.WithContext(ctx)

	resp, err := transport.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHeaderPropagatingTransport_RoundTrip_MultipleHeaders(t *testing.T) {
	headerMap := map[string]string{
		"X-Request-Id":     "req-123",
		"X-Correlation-Id": "corr-456",
		"X-Tenant-Id":      "tenant-789",
		"X-User-Id":        "user-abc",
	}

	ctx := context.WithValue(context.Background(), ContextKeyHeaders, headerMap)

	injectedHeaders := make(map[string]string)
	mockTransport := &mockRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			for key := range headerMap {
				injectedHeaders[key] = r.Header.Get(key)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}

	transport := NewHeaderPropagatingTransport(
		[]string{"x-request-id", "x-correlation-id", "x-tenant-id", "x-user-id"},
		mockTransport,
	)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/v1/resource", nil)
	req = req.WithContext(ctx)

	resp, err := transport.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	for key, expectedValue := range headerMap {
		assert.Equal(t, expectedValue, injectedHeaders[key], "Header %s mismatch", key)
	}
}
