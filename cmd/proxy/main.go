// Package main provides the entry point for the ContextForge proxy sidecar.
//
// Logging Architecture:
// The proxy uses zerolog for structured logging because:
// - It provides high-performance, zero-allocation JSON logging ideal for the data path
// - It supports both human-readable (console) and machine-parseable (JSON) output
// - It has minimal overhead which is critical for a sidecar that handles every request
//
// The operator (cmd/main.go) uses controller-runtime's logf package because:
// - It integrates seamlessly with the Kubernetes controller-runtime framework
// - It follows Kubernetes community conventions and patterns
// - It provides context-aware logging that works with reconcile loops
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bgruszka/contextforge/internal/config"
	"github.com/bgruszka/contextforge/internal/handler"
	"github.com/bgruszka/contextforge/internal/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	setupLogger()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	log.Info().
		Strs("headers", cfg.HeadersToPropagate).
		Str("target", cfg.TargetHost).
		Int("port", cfg.ProxyPort).
		Msg("Starting ContextForge proxy")

	proxyHandler, err := handler.NewProxyHandler(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create proxy handler")
	}
	srv := server.NewServer(cfg, proxyHandler)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed to start")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited gracefully")
}

// setupLogger configures zerolog based on LOG_LEVEL environment variable.
func setupLogger() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)

	if os.Getenv("LOG_FORMAT") == "json" {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}
}
