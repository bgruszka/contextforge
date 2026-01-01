package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(true, 100, 10)
	assert.NotNil(t, rl)
	assert.True(t, rl.enabled)
	assert.NotNil(t, rl.limiter)
}

func TestRateLimiter_Disabled(t *testing.T) {
	rl := NewRateLimiter(false, 1, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Should allow all requests when disabled
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		rl.Middleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "Request %d should succeed when rate limiting is disabled", i)
	}
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	// Allow 10 requests per second with burst of 10
	rl := NewRateLimiter(true, 10, 10)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First 10 requests should succeed (burst)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		rl.Middleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "Request %d should succeed within burst limit", i)
	}
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	// Very low limit: 1 request per second with burst of 1
	rl := NewRateLimiter(true, 1, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request should succeed
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	rl.Middleware(handler).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "First request should succeed")

	// Second request should be rate limited (immediately after first)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	rl.Middleware(handler).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTooManyRequests, rr.Code, "Second request should be rate limited")
}

func TestRateLimiter_Allow(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		rps      float64
		burst    int
		requests int
		wantPass int
	}{
		{
			name:     "disabled allows all",
			enabled:  false,
			rps:      1,
			burst:    1,
			requests: 10,
			wantPass: 10,
		},
		{
			name:     "enabled respects burst",
			enabled:  true,
			rps:      1,
			burst:    5,
			requests: 10,
			wantPass: 5,
		},
		{
			name:     "high burst allows more",
			enabled:  true,
			rps:      1,
			burst:    100,
			requests: 50,
			wantPass: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.enabled, tt.rps, tt.burst)
			passed := 0
			for i := 0; i < tt.requests; i++ {
				if rl.Allow() {
					passed++
				}
			}
			assert.Equal(t, tt.wantPass, passed)
		})
	}
}

func TestRateLimiter_ResponseBody(t *testing.T) {
	rl := NewRateLimiter(true, 1, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exhaust the burst
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	rl.Middleware(handler).ServeHTTP(rr, req)

	// Next request should be rate limited
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	rl.Middleware(handler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusTooManyRequests, rr.Code)
	assert.Contains(t, rr.Body.String(), "Too Many Requests")
}
