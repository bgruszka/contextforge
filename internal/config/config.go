// Package config provides configuration loading for the ContextForge proxy.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bgruszka/contextforge/internal/generator"
)

// HeaderRule defines a header propagation rule with optional generation settings.
type HeaderRule struct {
	// Name is the HTTP header name.
	Name string `json:"name"`

	// Generate indicates whether to auto-generate this header if missing.
	Generate bool `json:"generate,omitempty"`

	// GeneratorType specifies how to generate the header value (uuid, ulid, timestamp).
	GeneratorType generator.Type `json:"generatorType,omitempty"`

	// Propagate indicates whether to propagate this header (default: true).
	Propagate bool `json:"propagate"`

	// PathRegex is an optional regex pattern to match request paths.
	PathRegex string `json:"pathRegex,omitempty"`

	// Methods is an optional list of HTTP methods this rule applies to.
	Methods []string `json:"methods,omitempty"`

	// CompiledPathRegex is the compiled path regex (set after validation).
	CompiledPathRegex *regexp.Regexp `json:"-"`
}

// MatchesRequest checks if this rule applies to the given request path and method.
func (r *HeaderRule) MatchesRequest(path, method string) bool {
	// Check path regex if specified
	if r.CompiledPathRegex != nil {
		if !r.CompiledPathRegex.MatchString(path) {
			return false
		}
	}

	// Check methods if specified
	if len(r.Methods) > 0 {
		methodMatches := false
		upperMethod := strings.ToUpper(method)
		for _, m := range r.Methods {
			if strings.ToUpper(m) == upperMethod {
				methodMatches = true
				break
			}
		}
		if !methodMatches {
			return false
		}
	}

	return true
}

// ProxyConfig holds the configuration for the proxy sidecar.
type ProxyConfig struct {
	// HeadersToPropagate is a list of HTTP header names to extract and propagate.
	// Deprecated: Use HeaderRules for more control.
	HeadersToPropagate []string

	// HeaderRules defines header propagation rules with generation and filtering options.
	HeaderRules []HeaderRule

	// TargetHost is the address of the application container to forward requests to.
	TargetHost string

	// ProxyPort is the port the proxy listens on for incoming requests.
	ProxyPort int

	// LogLevel defines the logging verbosity (debug, info, warn, error).
	LogLevel string

	// MetricsPort is the port for Prometheus metrics endpoint.
	MetricsPort int

	// ReadTimeout is the maximum duration for reading the entire request, including the body.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration

	// IdleTimeout is the maximum amount of time to wait for the next request.
	IdleTimeout time.Duration

	// ReadHeaderTimeout is the amount of time allowed to read request headers.
	ReadHeaderTimeout time.Duration

	// TargetDialTimeout is the timeout for dialing the target application.
	TargetDialTimeout time.Duration

	// RateLimitEnabled enables rate limiting middleware.
	RateLimitEnabled bool

	// RateLimitRPS is the requests per second limit.
	RateLimitRPS float64

	// RateLimitBurst is the maximum burst size for rate limiting.
	RateLimitBurst int
}

// Default timeout values with rationale:
//
// ReadTimeout (15s): Maximum time to read the entire request including body.
// Set to 15s to accommodate typical API requests while preventing slow-loris attacks.
// Adjust higher (30s-60s) for file uploads or long-polling endpoints.
//
// WriteTimeout (15s): Maximum time to write the response.
// Matches ReadTimeout for symmetry. Increase for endpoints returning large responses.
//
// IdleTimeout (60s): Time to keep idle connections open for reuse.
// 60s balances connection reuse benefits against resource consumption.
// Increase for high-latency networks, decrease for memory-constrained environments.
//
// ReadHeaderTimeout (5s): Time to read request headers only.
// 5s is sufficient for normal requests while protecting against slowloris attacks.
//
// TargetDialTimeout (5s): Time to establish connection to target application.
// Increased from 2s to 5s to handle Kubernetes DNS resolution delays during
// pod restarts and rolling updates. Adjust higher for cross-cluster communication.
const (
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 15 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultReadHeaderTimeout = 5 * time.Second
	defaultTargetDialTimeout = 5 * time.Second
)

// Load reads configuration from environment variables and returns a ProxyConfig.
// Returns an error if required configuration is missing or invalid.
func Load() (*ProxyConfig, error) {
	cfg := &ProxyConfig{
		TargetHost:        getEnv("TARGET_HOST", "localhost:8080"),
		ProxyPort:         getEnvInt("PROXY_PORT", 9090),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		MetricsPort:       getEnvInt("METRICS_PORT", 9091),
		ReadTimeout:       getEnvDuration("READ_TIMEOUT", defaultReadTimeout),
		WriteTimeout:      getEnvDuration("WRITE_TIMEOUT", defaultWriteTimeout),
		IdleTimeout:       getEnvDuration("IDLE_TIMEOUT", defaultIdleTimeout),
		ReadHeaderTimeout: getEnvDuration("READ_HEADER_TIMEOUT", defaultReadHeaderTimeout),
		TargetDialTimeout: getEnvDuration("TARGET_DIAL_TIMEOUT", defaultTargetDialTimeout),
		RateLimitEnabled:  getEnvBool("RATE_LIMIT_ENABLED", false),
		RateLimitRPS:      getEnvFloat("RATE_LIMIT_RPS", 1000),
		RateLimitBurst:    getEnvInt("RATE_LIMIT_BURST", 100),
	}

	// Parse header rules - prefer HEADER_RULES over HEADERS_TO_PROPAGATE
	headerRulesStr := getEnv("HEADER_RULES", "")
	headersStr := getEnv("HEADERS_TO_PROPAGATE", "")

	if headerRulesStr != "" {
		// Parse JSON header rules
		rules, err := parseHeaderRules(headerRulesStr)
		if err != nil {
			return nil, fmt.Errorf("invalid HEADER_RULES: %w", err)
		}
		cfg.HeaderRules = rules
		// Also populate HeadersToPropagate for backward compatibility
		for _, rule := range rules {
			if rule.Propagate {
				cfg.HeadersToPropagate = append(cfg.HeadersToPropagate, rule.Name)
			}
		}
	} else if headersStr != "" {
		// Parse simple comma-separated headers (backward compatible)
		headers, err := parseHeaders(headersStr)
		if err != nil {
			return nil, fmt.Errorf("invalid HEADERS_TO_PROPAGATE: %w", err)
		}
		cfg.HeadersToPropagate = headers
		// Convert to HeaderRules for unified processing
		for _, h := range headers {
			cfg.HeaderRules = append(cfg.HeaderRules, HeaderRule{
				Name:      h,
				Propagate: true,
			})
		}
	} else {
		return nil, fmt.Errorf("HEADERS_TO_PROPAGATE or HEADER_RULES environment variable is required (e.g., HEADERS_TO_PROPAGATE=x-request-id,x-tenant-id)")
	}

	if len(cfg.HeaderRules) == 0 {
		return nil, fmt.Errorf("at least one header must be specified (e.g., HEADERS_TO_PROPAGATE=x-request-id,x-correlation-id)")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// parseHeaderRules parses a JSON array of header rules.
// Format: [{"name":"x-request-id","generate":true,"generatorType":"uuid"},{"name":"x-tenant-id"}]
func parseHeaderRules(input string) ([]HeaderRule, error) {
	var rules []HeaderRule
	if err := json.Unmarshal([]byte(input), &rules); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w (expected format: [{\"name\":\"x-request-id\",\"generate\":true,\"generatorType\":\"uuid\"}])", err)
	}

	for i := range rules {
		// Validate header name
		if err := validateHeaderName(rules[i].Name); err != nil {
			return nil, err
		}

		// Default Propagate to true if not explicitly set to false
		// JSON unmarshaling sets bool to false by default, so we default to true
		// for most use cases. To disable propagation, explicitly set "propagate": false
		if !rules[i].Propagate {
			rules[i].Propagate = true
		}

		// Validate generator type if generation is enabled
		if rules[i].Generate {
			if rules[i].GeneratorType == "" {
				rules[i].GeneratorType = generator.TypeUUID // Default to UUID
			}
			if _, err := generator.New(rules[i].GeneratorType); err != nil {
				return nil, fmt.Errorf("header %q: %w", rules[i].Name, err)
			}
		}

		// Compile path regex if specified
		if rules[i].PathRegex != "" {
			compiled, err := regexp.Compile(rules[i].PathRegex)
			if err != nil {
				return nil, fmt.Errorf("header %q: invalid path regex %q: %w", rules[i].Name, rules[i].PathRegex, err)
			}
			rules[i].CompiledPathRegex = compiled
		}

		// Validate HTTP methods if specified
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true, "DELETE": true,
			"PATCH": true, "HEAD": true, "OPTIONS": true, "TRACE": true, "CONNECT": true,
		}
		for _, m := range rules[i].Methods {
			if !validMethods[strings.ToUpper(m)] {
				return nil, fmt.Errorf("header %q: invalid HTTP method %q", rules[i].Name, m)
			}
		}
	}

	return rules, nil
}

// Validate checks if the configuration values are valid.
func (c *ProxyConfig) Validate() error {
	if c.ProxyPort < 1 || c.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port: %d (must be 1-65535, e.g., PROXY_PORT=9090)", c.ProxyPort)
	}

	if c.MetricsPort < 1 || c.MetricsPort > 65535 {
		return fmt.Errorf("invalid metrics port: %d (must be 1-65535, e.g., METRICS_PORT=9091)", c.MetricsPort)
	}

	if c.ProxyPort == c.MetricsPort {
		return fmt.Errorf("proxy port and metrics port cannot be the same: %d (use different ports, e.g., PROXY_PORT=9090 METRICS_PORT=9091)", c.ProxyPort)
	}

	if c.TargetHost == "" {
		return fmt.Errorf("target host cannot be empty (e.g., TARGET_HOST=localhost:8080)")
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

	// Validate timeouts
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("invalid read timeout: %v (must be positive, e.g., 15s)", c.ReadTimeout)
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("invalid write timeout: %v (must be positive, e.g., 15s)", c.WriteTimeout)
	}
	if c.IdleTimeout <= 0 {
		return fmt.Errorf("invalid idle timeout: %v (must be positive, e.g., 60s)", c.IdleTimeout)
	}
	if c.ReadHeaderTimeout <= 0 {
		return fmt.Errorf("invalid read header timeout: %v (must be positive, e.g., 5s)", c.ReadHeaderTimeout)
	}
	if c.TargetDialTimeout <= 0 {
		return fmt.Errorf("invalid target dial timeout: %v (must be positive, e.g., 2s)", c.TargetDialTimeout)
	}

	return nil
}

// headerNameRegex validates HTTP header names per RFC 7230.
// Header names must contain only alphanumeric characters and hyphens.
var headerNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

// validateHeaderName checks if a header name is valid per RFC 7230.
// Valid header names contain only alphanumeric characters and hyphens,
// must start with an alphanumeric character, and be 1-256 characters long.
func validateHeaderName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("header name cannot be empty")
	}
	if len(name) > 256 {
		return fmt.Errorf("header name %q exceeds maximum length of 256 characters", name)
	}
	if !headerNameRegex.MatchString(name) {
		return fmt.Errorf("header name %q is invalid: must contain only alphanumeric characters and hyphens, starting with alphanumeric (e.g., x-request-id, X-Correlation-ID)", name)
	}
	return nil
}

// parseHeaders splits a comma-separated header string into a slice of trimmed header names.
// Returns an error if any header name is invalid.
func parseHeaders(input string) ([]string, error) {
	parts := strings.Split(input, ",")
	headers := make([]string, 0, len(parts))

	for _, part := range parts {
		header := strings.TrimSpace(part)
		if header != "" {
			if err := validateHeaderName(header); err != nil {
				return nil, err
			}
			headers = append(headers, header)
		}
	}

	return headers, nil
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

// getEnvDuration returns the duration value of an environment variable or a default value.
// Duration strings are parsed using time.ParseDuration (e.g., "15s", "1m30s", "500ms").
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// getEnvBool returns the boolean value of an environment variable or a default value.
// Accepts "true", "1", "yes" as true; "false", "0", "no" as false (case-insensitive).
func getEnvBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	switch strings.ToLower(valueStr) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return defaultValue
	}
}

// getEnvFloat returns the float64 value of an environment variable or a default value.
func getEnvFloat(key string, defaultValue float64) float64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return defaultValue
	}

	return value
}
