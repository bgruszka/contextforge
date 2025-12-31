// Package config provides configuration loading for the ContextForge proxy.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ProxyConfig holds the configuration for the proxy sidecar.
type ProxyConfig struct {
	// HeadersToPropagate is a list of HTTP header names to extract and propagate.
	HeadersToPropagate []string

	// TargetHost is the address of the application container to forward requests to.
	TargetHost string

	// ProxyPort is the port the proxy listens on for incoming requests.
	ProxyPort int

	// LogLevel defines the logging verbosity (debug, info, warn, error).
	LogLevel string

	// MetricsPort is the port for Prometheus metrics endpoint.
	MetricsPort int
}

// Load reads configuration from environment variables and returns a ProxyConfig.
// Returns an error if required configuration is missing or invalid.
func Load() (*ProxyConfig, error) {
	cfg := &ProxyConfig{
		TargetHost:  getEnv("TARGET_HOST", "localhost:8080"),
		ProxyPort:   getEnvInt("PROXY_PORT", 9090),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		MetricsPort: getEnvInt("METRICS_PORT", 9091),
	}

	headersStr := getEnv("HEADERS_TO_PROPAGATE", "")
	if headersStr == "" {
		return nil, fmt.Errorf("HEADERS_TO_PROPAGATE environment variable is required")
	}

	cfg.HeadersToPropagate = parseHeaders(headersStr)
	if len(cfg.HeadersToPropagate) == 0 {
		return nil, fmt.Errorf("at least one header must be specified in HEADERS_TO_PROPAGATE")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration values are valid.
func (c *ProxyConfig) Validate() error {
	if c.ProxyPort < 1 || c.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port: %d (must be 1-65535)", c.ProxyPort)
	}

	if c.MetricsPort < 1 || c.MetricsPort > 65535 {
		return fmt.Errorf("invalid metrics port: %d (must be 1-65535)", c.MetricsPort)
	}

	if c.ProxyPort == c.MetricsPort {
		return fmt.Errorf("proxy port and metrics port cannot be the same: %d", c.ProxyPort)
	}

	if c.TargetHost == "" {
		return fmt.Errorf("target host cannot be empty")
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[strings.ToLower(c.LogLevel)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.LogLevel)
	}

	return nil
}

// parseHeaders splits a comma-separated header string into a slice of trimmed header names.
func parseHeaders(input string) []string {
	parts := strings.Split(input, ",")
	headers := make([]string, 0, len(parts))

	for _, part := range parts {
		header := strings.TrimSpace(part)
		if header != "" {
			headers = append(headers, header)
		}
	}

	return headers
}

// getEnv returns the value of an environment variable or a default value if not set.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns the integer value of an environment variable or a default value.
func getEnvInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}
