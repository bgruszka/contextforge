// Package handler provides HTTP handlers for the ContextForge proxy.
package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/bgruszka/contextforge/internal/config"
	"github.com/bgruszka/contextforge/internal/metrics"
	"github.com/rs/zerolog/log"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

// ContextKeyHeaders is the key used to store propagated headers in the request context.
const ContextKeyHeaders contextKey = "ctxforge-headers"

// ProxyHandler handles incoming HTTP requests, extracts configured headers,
// stores them in the request context, and forwards the request to the target application.
type ProxyHandler struct {
	config       *config.ProxyConfig
	reverseProxy *httputil.ReverseProxy
	headers      []string
}

// NewProxyHandler creates a new ProxyHandler with the given configuration.
// Returns an error if the target host URL is invalid.
func NewProxyHandler(cfg *config.ProxyConfig) (*ProxyHandler, error) {
	targetURL, err := url.Parse("http://" + cfg.TargetHost)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target host URL %q: %w", cfg.TargetHost, err)
	}

	transport := NewHeaderPropagatingTransport(cfg.HeadersToPropagate, http.DefaultTransport)

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = transport

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error().
			Err(err).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("Proxy error forwarding request")
		w.WriteHeader(http.StatusBadGateway)
	}

	return &ProxyHandler{
		config:       cfg,
		reverseProxy: proxy,
		headers:      cfg.HeadersToPropagate,
	}, nil
}

// ServeHTTP implements the http.Handler interface.
// It extracts configured headers, stores them in context, and forwards to the target.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metrics.ActiveConnections.Inc()
	defer metrics.ActiveConnections.Dec()

	headerMap := h.extractHeaders(r)

	// Record propagated headers metric
	if len(headerMap) > 0 {
		metrics.RecordHeadersPropagated(len(headerMap))
	}

	ctx := context.WithValue(r.Context(), ContextKeyHeaders, headerMap)
	r = r.WithContext(ctx)

	if log.Debug().Enabled() {
		log.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Interface("propagated_headers", headerMap).
			Msg("Proxying request")
	}

	// Wrap response writer to capture status code
	rw := metrics.NewResponseWriter(w)
	h.reverseProxy.ServeHTTP(rw, r)

	// Record request metrics
	duration := time.Since(start)
	metrics.RecordRequest(r.Method, rw.StatusCode, duration)
}

// extractHeaders extracts the configured headers from the incoming request.
// Header names are matched case-insensitively.
func (h *ProxyHandler) extractHeaders(r *http.Request) map[string]string {
	headerMap := make(map[string]string)

	for _, headerName := range h.headers {
		headerName = strings.TrimSpace(headerName)
		if value := r.Header.Get(headerName); value != "" {
			headerMap[http.CanonicalHeaderKey(headerName)] = value
		}
	}

	return headerMap
}

// GetHeadersFromContext retrieves the propagated headers from a request context.
// Returns nil if no headers are found in the context.
func GetHeadersFromContext(ctx context.Context) map[string]string {
	headers, ok := ctx.Value(ContextKeyHeaders).(map[string]string)
	if !ok {
		return nil
	}
	return headers
}
