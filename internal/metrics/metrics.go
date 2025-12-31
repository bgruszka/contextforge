// Package metrics provides Prometheus metrics for the ContextForge proxy.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "ctxforge"
	subsystem = "proxy"
)

var (
	// RequestsTotal counts the total number of HTTP requests processed.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Total number of HTTP requests processed by the proxy.",
		},
		[]string{"method", "status"},
	)

	// RequestDuration tracks the duration of HTTP requests.
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "Duration of HTTP requests in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method"},
	)

	// HeadersPropagatedTotal counts the total number of headers propagated.
	HeadersPropagatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "headers_propagated_total",
			Help:      "Total number of headers propagated to target requests.",
		},
	)

	// ActiveConnections tracks the number of active connections.
	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "active_connections",
			Help:      "Number of active connections being processed.",
		},
	)
)

// RecordRequest records metrics for a completed HTTP request.
func RecordRequest(method string, statusCode int, duration time.Duration) {
	RequestsTotal.WithLabelValues(method, strconv.Itoa(statusCode)).Inc()
	RequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordHeadersPropagated increments the counter for propagated headers.
func RecordHeadersPropagated(count int) {
	HeadersPropagatedTotal.Add(float64(count))
}

// Handler returns the Prometheus HTTP handler for exposing metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}

// ResponseWriter wraps http.ResponseWriter to capture the status code.
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode int
}

// NewResponseWriter creates a new ResponseWriter wrapper.
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		StatusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code before writing it.
func (rw *ResponseWriter) WriteHeader(code int) {
	rw.StatusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
