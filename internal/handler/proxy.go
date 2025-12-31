// Package handler provides HTTP handlers for the ContextForge proxy.
package handler

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/bgruszka/contextforge/internal/config"
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
func NewProxyHandler(cfg *config.ProxyConfig) *ProxyHandler {
	targetURL, err := url.Parse("http://" + cfg.TargetHost)
	if err != nil {
		log.Fatal().Err(err).Str("target", cfg.TargetHost).Msg("Failed to parse target host URL")
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
	}
}

// ServeHTTP implements the http.Handler interface.
// It extracts configured headers, stores them in context, and forwards to the target.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headerMap := h.extractHeaders(r)

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

	h.reverseProxy.ServeHTTP(w, r)
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
