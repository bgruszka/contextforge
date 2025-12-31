// Package main provides the entry point for the ContextForge proxy sidecar.
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

	proxyHandler := handler.NewProxyHandler(cfg)
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
