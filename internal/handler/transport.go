package handler

import (
	"net/http"

	"github.com/rs/zerolog/log"
)

// HeaderPropagatingTransport wraps an http.RoundTripper to inject propagated headers
// from the request context into outbound HTTP requests.
type HeaderPropagatingTransport struct {
	headers       []string
	baseTransport http.RoundTripper
}

// NewHeaderPropagatingTransport creates a new HeaderPropagatingTransport.
func NewHeaderPropagatingTransport(headers []string, base http.RoundTripper) *HeaderPropagatingTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &HeaderPropagatingTransport{
		headers:       headers,
		baseTransport: base,
	}
}

// RoundTrip implements the http.RoundTripper interface.
// It retrieves headers from the request context and injects them into the outbound request.
func (t *HeaderPropagatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	headerMap := GetHeadersFromContext(req.Context())

	if headerMap != nil {
		for name, value := range headerMap {
			if req.Header.Get(name) == "" {
				req.Header.Set(name, value)
				if log.Debug().Enabled() {
					log.Debug().
						Str("header", name).
						Str("value", value).
						Str("url", req.URL.String()).
						Msg("Injecting header into outbound request")
				}
			}
		}
	}

	return t.baseTransport.RoundTrip(req)
}
