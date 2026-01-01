package config

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Success(t *testing.T) {
	t.Setenv("HEADERS_TO_PROPAGATE", "x-request-id,x-dev-id,x-tenant-id")
	t.Setenv("TARGET_HOST", "localhost:8080")
	t.Setenv("PROXY_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("METRICS_PORT", "9091")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, []string{"x-request-id", "x-dev-id", "x-tenant-id"}, cfg.HeadersToPropagate)
	assert.Equal(t, "localhost:8080", cfg.TargetHost)
	assert.Equal(t, 9090, cfg.ProxyPort)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 9091, cfg.MetricsPort)
}

func TestLoad_DefaultValues(t *testing.T) {
	t.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.TargetHost)
	assert.Equal(t, 9090, cfg.ProxyPort)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 9091, cfg.MetricsPort)
	// Check default timeout values
	assert.Equal(t, 15*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 15*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 60*time.Second, cfg.IdleTimeout)
	assert.Equal(t, 5*time.Second, cfg.ReadHeaderTimeout)
	assert.Equal(t, 5*time.Second, cfg.TargetDialTimeout)
}

func TestLoad_CustomTimeouts(t *testing.T) {
	t.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	t.Setenv("READ_TIMEOUT", "30s")
	t.Setenv("WRITE_TIMEOUT", "45s")
	t.Setenv("IDLE_TIMEOUT", "2m")
	t.Setenv("READ_HEADER_TIMEOUT", "10s")
	t.Setenv("TARGET_DIAL_TIMEOUT", "5s")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 45*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 2*time.Minute, cfg.IdleTimeout)
	assert.Equal(t, 10*time.Second, cfg.ReadHeaderTimeout)
	assert.Equal(t, 5*time.Second, cfg.TargetDialTimeout)
}

func TestLoad_MissingHeaders(t *testing.T) {
	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HEADERS_TO_PROPAGATE")
}

func TestLoad_HeaderRules(t *testing.T) {
	t.Setenv("HEADER_RULES", `[{"name":"x-request-id","generate":true,"generatorType":"uuid"},{"name":"x-tenant-id","propagate":true}]`)

	cfg, err := Load()

	require.NoError(t, err)
	assert.Len(t, cfg.HeaderRules, 2)
	assert.Equal(t, "x-request-id", cfg.HeaderRules[0].Name)
	assert.True(t, cfg.HeaderRules[0].Generate)
	assert.Equal(t, "uuid", string(cfg.HeaderRules[0].GeneratorType))
	assert.Equal(t, "x-tenant-id", cfg.HeaderRules[1].Name)
	assert.True(t, cfg.HeaderRules[1].Propagate)
	// HeadersToPropagate should be populated for backward compatibility
	assert.Contains(t, cfg.HeadersToPropagate, "x-request-id")
	assert.Contains(t, cfg.HeadersToPropagate, "x-tenant-id")
}

func TestLoad_HeaderRulesWithPathAndMethods(t *testing.T) {
	t.Setenv("HEADER_RULES", `[{"name":"x-request-id","pathRegex":"^/api/.*","methods":["GET","POST"]}]`)

	cfg, err := Load()

	require.NoError(t, err)
	assert.Len(t, cfg.HeaderRules, 1)
	assert.Equal(t, "^/api/.*", cfg.HeaderRules[0].PathRegex)
	assert.Equal(t, []string{"GET", "POST"}, cfg.HeaderRules[0].Methods)
}

func TestLoad_HeaderRulesInvalidJSON(t *testing.T) {
	t.Setenv("HEADER_RULES", `not valid json`)

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestLoad_HeaderRulesInvalidGeneratorType(t *testing.T) {
	t.Setenv("HEADER_RULES", `[{"name":"x-request-id","generate":true,"generatorType":"invalid"}]`)

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown generator type")
}

func TestLoad_HeaderRulesInvalidPathRegex(t *testing.T) {
	t.Setenv("HEADER_RULES", `[{"name":"x-request-id","pathRegex":"[invalid"}]`)

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid path regex")
}

func TestLoad_HeaderRulesInvalidMethod(t *testing.T) {
	t.Setenv("HEADER_RULES", `[{"name":"x-request-id","methods":["INVALID"]}]`)

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid HTTP method")
}

func TestHeaderRule_MatchesRequest(t *testing.T) {
	tests := []struct {
		name     string
		rule     HeaderRule
		path     string
		method   string
		expected bool
	}{
		{
			name:     "no filters matches everything",
			rule:     HeaderRule{Name: "x-request-id"},
			path:     "/api/users",
			method:   "GET",
			expected: true,
		},
		{
			name: "path regex matches",
			rule: HeaderRule{
				Name:      "x-request-id",
				PathRegex: "^/api/.*",
			},
			path:     "/api/users",
			method:   "GET",
			expected: true,
		},
		{
			name: "path regex does not match",
			rule: HeaderRule{
				Name:      "x-request-id",
				PathRegex: "^/api/.*",
			},
			path:     "/health",
			method:   "GET",
			expected: false,
		},
		{
			name: "method matches",
			rule: HeaderRule{
				Name:    "x-request-id",
				Methods: []string{"GET", "POST"},
			},
			path:     "/api/users",
			method:   "GET",
			expected: true,
		},
		{
			name: "method does not match",
			rule: HeaderRule{
				Name:    "x-request-id",
				Methods: []string{"GET", "POST"},
			},
			path:     "/api/users",
			method:   "DELETE",
			expected: false,
		},
		{
			name: "method matching is case insensitive",
			rule: HeaderRule{
				Name:    "x-request-id",
				Methods: []string{"get", "post"},
			},
			path:     "/api/users",
			method:   "GET",
			expected: true,
		},
		{
			name: "both path and method must match",
			rule: HeaderRule{
				Name:      "x-request-id",
				PathRegex: "^/api/.*",
				Methods:   []string{"GET"},
			},
			path:     "/api/users",
			method:   "POST",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := tt.rule
			// Compile path regex if present
			if rule.PathRegex != "" {
				compiled, err := regexp.Compile(rule.PathRegex)
				require.NoError(t, err)
				rule.CompiledPathRegex = compiled
			}
			result := rule.MatchesRequest(tt.path, tt.method)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoad_EmptyHeaders(t *testing.T) {
	t.Setenv("HEADERS_TO_PROPAGATE", "  ,  ,  ")

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one header")
}

func TestLoad_InvalidProxyPort(t *testing.T) {
	t.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	t.Setenv("PROXY_PORT", "99999")

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "proxy port")
}

func TestLoad_SamePortConflict(t *testing.T) {
	t.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	t.Setenv("PROXY_PORT", "9090")
	t.Setenv("METRICS_PORT", "9090")

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be the same")
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	t.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	t.Setenv("LOG_LEVEL", "invalid")

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "log level")
}

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single header",
			input:    "x-request-id",
			expected: []string{"x-request-id"},
		},
		{
			name:     "multiple headers",
			input:    "x-request-id,x-dev-id,x-tenant-id",
			expected: []string{"x-request-id", "x-dev-id", "x-tenant-id"},
		},
		{
			name:     "headers with spaces",
			input:    "x-request-id , x-dev-id , x-tenant-id",
			expected: []string{"x-request-id", "x-dev-id", "x-tenant-id"},
		},
		{
			name:     "headers with empty values",
			input:    "x-request-id,,x-dev-id,",
			expected: []string{"x-request-id", "x-dev-id"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "only commas and spaces",
			input:    " , , ",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseHeaders(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseHeaders_InvalidHeaders(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "header with space",
			input: "invalid header",
		},
		{
			name:  "header starting with hyphen",
			input: "-invalid",
		},
		{
			name:  "header with special characters",
			input: "x-request@id",
		},
		{
			name:  "one valid one invalid",
			input: "x-request-id,invalid header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseHeaders(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_KEY", "test_value")

	assert.Equal(t, "test_value", getEnv("TEST_KEY", "default"))
	assert.Equal(t, "default", getEnv("NONEXISTENT_KEY", "default"))
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	t.Setenv("TEST_INVALID_INT", "not_a_number")

	assert.Equal(t, 42, getEnvInt("TEST_INT", 10))
	assert.Equal(t, 10, getEnvInt("NONEXISTENT_INT", 10))
	assert.Equal(t, 10, getEnvInt("TEST_INVALID_INT", 10))
}

func TestGetEnvDuration(t *testing.T) {
	t.Setenv("TEST_DURATION", "30s")
	t.Setenv("TEST_DURATION_COMPLEX", "1m30s")
	t.Setenv("TEST_DURATION_MS", "500ms")
	t.Setenv("TEST_INVALID_DURATION", "not_a_duration")

	assert.Equal(t, 30*time.Second, getEnvDuration("TEST_DURATION", 10*time.Second))
	assert.Equal(t, 90*time.Second, getEnvDuration("TEST_DURATION_COMPLEX", 10*time.Second))
	assert.Equal(t, 500*time.Millisecond, getEnvDuration("TEST_DURATION_MS", 10*time.Second))
	assert.Equal(t, 10*time.Second, getEnvDuration("NONEXISTENT_DURATION", 10*time.Second))
	assert.Equal(t, 10*time.Second, getEnvDuration("TEST_INVALID_DURATION", 10*time.Second))
}

func TestValidate(t *testing.T) {
	validConfig := func() ProxyConfig {
		return ProxyConfig{
			HeadersToPropagate: []string{"x-request-id"},
			TargetHost:         "localhost:8080",
			ProxyPort:          9090,
			LogLevel:           "info",
			MetricsPort:        9091,
			ReadTimeout:        15 * time.Second,
			WriteTimeout:       15 * time.Second,
			IdleTimeout:        60 * time.Second,
			ReadHeaderTimeout:  5 * time.Second,
			TargetDialTimeout:  2 * time.Second,
		}
	}

	tests := []struct {
		name      string
		config    ProxyConfig
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid config",
			config:    validConfig(),
			expectErr: false,
		},
		{
			name: "invalid proxy port - too high",
			config: func() ProxyConfig {
				c := validConfig()
				c.ProxyPort = 70000
				return c
			}(),
			expectErr: true,
			errMsg:    "proxy port",
		},
		{
			name: "invalid proxy port - zero",
			config: func() ProxyConfig {
				c := validConfig()
				c.ProxyPort = 0
				return c
			}(),
			expectErr: true,
			errMsg:    "proxy port",
		},
		{
			name: "empty target host",
			config: func() ProxyConfig {
				c := validConfig()
				c.TargetHost = ""
				return c
			}(),
			expectErr: true,
			errMsg:    "target host",
		},
		{
			name: "invalid read timeout - zero",
			config: func() ProxyConfig {
				c := validConfig()
				c.ReadTimeout = 0
				return c
			}(),
			expectErr: true,
			errMsg:    "read timeout",
		},
		{
			name: "invalid write timeout - negative",
			config: func() ProxyConfig {
				c := validConfig()
				c.WriteTimeout = -1 * time.Second
				return c
			}(),
			expectErr: true,
			errMsg:    "write timeout",
		},
		{
			name: "invalid idle timeout - zero",
			config: func() ProxyConfig {
				c := validConfig()
				c.IdleTimeout = 0
				return c
			}(),
			expectErr: true,
			errMsg:    "idle timeout",
		},
		{
			name: "invalid read header timeout - zero",
			config: func() ProxyConfig {
				c := validConfig()
				c.ReadHeaderTimeout = 0
				return c
			}(),
			expectErr: true,
			errMsg:    "read header timeout",
		},
		{
			name: "invalid target dial timeout - zero",
			config: func() ProxyConfig {
				c := validConfig()
				c.TargetDialTimeout = 0
				return c
			}(),
			expectErr: true,
			errMsg:    "target dial timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
