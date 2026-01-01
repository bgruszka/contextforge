package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecordRequest(t *testing.T) {
	// Just verify it doesn't panic
	RecordRequest("GET", 200, 100*time.Millisecond)
	RecordRequest("POST", 201, 50*time.Millisecond)
	RecordRequest("GET", 500, 200*time.Millisecond)
}

func TestRecordHeadersPropagated(t *testing.T) {
	// Just verify it doesn't panic
	RecordHeadersPropagated(3)
	RecordHeadersPropagated(1)
}

func TestHandler(t *testing.T) {
	handler := Handler()
	assert.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "ctxforge_proxy_requests_total")
	assert.Contains(t, rr.Body.String(), "ctxforge_proxy_request_duration_seconds")
	assert.Contains(t, rr.Body.String(), "ctxforge_proxy_headers_propagated_total")
	assert.Contains(t, rr.Body.String(), "ctxforge_proxy_active_connections")
}

func TestResponseWriter(t *testing.T) {
	tests := []struct {
		name           string
		writeHeader    bool
		statusCode     int
		expectedStatus int
	}{
		{
			name:           "default status is 200",
			writeHeader:    false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "captures 201 status",
			writeHeader:    true,
			statusCode:     http.StatusCreated,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "captures 404 status",
			writeHeader:    true,
			statusCode:     http.StatusNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "captures 500 status",
			writeHeader:    true,
			statusCode:     http.StatusInternalServerError,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			rw := NewResponseWriter(rr)

			if tt.writeHeader {
				rw.WriteHeader(tt.statusCode)
			}

			assert.Equal(t, tt.expectedStatus, rw.StatusCode)
		})
	}
}

func TestResponseWriter_Write(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := NewResponseWriter(rr)

	n, err := rw.Write([]byte("hello"))

	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", rr.Body.String())
}
