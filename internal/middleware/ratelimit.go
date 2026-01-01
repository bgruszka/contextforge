// Package middleware provides HTTP middleware components for the ContextForge proxy.
package middleware

import (
	"net/http"

	"golang.org/x/time/rate"
)

// RateLimiter is an HTTP middleware that limits requests using a token bucket algorithm.
type RateLimiter struct {
	limiter *rate.Limiter
	enabled bool
}

// NewRateLimiter creates a new rate limiter middleware.
// If enabled is false, the middleware will pass all requests through without limiting.
// rps is the requests per second limit, burst is the maximum burst size.
func NewRateLimiter(enabled bool, rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
		enabled: enabled,
	}
}

// Middleware returns an HTTP middleware function that applies rate limiting.
// When the rate limit is exceeded, it returns HTTP 429 Too Many Requests.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.enabled {
			next.ServeHTTP(w, r)
			return
		}

		if !rl.limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Allow checks if a request is allowed under the current rate limit.
// Returns true if allowed, false if rate limited.
func (rl *RateLimiter) Allow() bool {
	if !rl.enabled {
		return true
	}
	return rl.limiter.Allow()
}
