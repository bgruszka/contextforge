// Package server provides the HTTP server for the ContextForge proxy.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/bgruszka/contextforge/internal/config"
	"github.com/rs/zerolog/log"
)

// Server represents the HTTP server for the proxy.
type Server struct {
	config     *config.ProxyConfig
	httpServer *http.Server
	mux        *http.ServeMux
}

// HealthResponse represents the JSON response for health check endpoints.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// ReadyResponse represents the JSON response for the readiness endpoint.
type ReadyResponse struct {
	Status          string `json:"status"`
	TargetHost      string `json:"targetHost"`
	TargetReachable bool   `json:"targetReachable"`
	Timestamp       string `json:"timestamp"`
}

// NewServer creates a new Server with the given configuration and proxy handler.
func NewServer(cfg *config.ProxyConfig, proxyHandler http.Handler) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/ready", readyHandler(cfg.TargetHost))

	mux.Handle("/", proxyHandler)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.ProxyPort),
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Server{
		config:     cfg,
		httpServer: httpServer,
		mux:        mux,
	}
}

// Start begins listening for HTTP requests.
// This method blocks until the server is shut down or an error occurs.
func (s *Server) Start() error {
	log.Info().
		Str("addr", s.httpServer.Addr).
		Str("target", s.config.TargetHost).
		Strs("headers", s.config.HeadersToPropagate).
		Msg("Starting HTTP server")

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server with the given context.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("Shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// healthHandler responds with a simple health check status.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// readyHandler returns a handler that checks if the target host is reachable.
func readyHandler(targetHost string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetReachable := checkTargetReachable(targetHost)

		response := ReadyResponse{
			Status:          "ready",
			TargetHost:      targetHost,
			TargetReachable: targetReachable,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
		}

		if !targetReachable {
			response.Status = "not_ready"
		}

		w.Header().Set("Content-Type", "application/json")
		if targetReachable {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(response)
	}
}

// checkTargetReachable attempts a TCP connection to verify the target is reachable.
func checkTargetReachable(targetHost string) bool {
	conn, err := net.DialTimeout("tcp", targetHost, 2*time.Second)
	if err != nil {
		log.Debug().Err(err).Str("target", targetHost).Msg("Target not reachable")
		return false
	}
	conn.Close()
	return true
}
