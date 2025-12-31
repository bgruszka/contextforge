package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Success(t *testing.T) {
	os.Setenv("HEADERS_TO_PROPAGATE", "x-request-id,x-dev-id,x-tenant-id")
	os.Setenv("TARGET_HOST", "localhost:8080")
	os.Setenv("PROXY_PORT", "9090")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("METRICS_PORT", "9091")
	defer clearEnv()

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, []string{"x-request-id", "x-dev-id", "x-tenant-id"}, cfg.HeadersToPropagate)
	assert.Equal(t, "localhost:8080", cfg.TargetHost)
	assert.Equal(t, 9090, cfg.ProxyPort)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 9091, cfg.MetricsPort)
}

func TestLoad_DefaultValues(t *testing.T) {
	os.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	defer clearEnv()

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.TargetHost)
	assert.Equal(t, 9090, cfg.ProxyPort)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 9091, cfg.MetricsPort)
}

func TestLoad_MissingHeaders(t *testing.T) {
	clearEnv()

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HEADERS_TO_PROPAGATE")
}

func TestLoad_EmptyHeaders(t *testing.T) {
	os.Setenv("HEADERS_TO_PROPAGATE", "  ,  ,  ")
	defer clearEnv()

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one header")
}

func TestLoad_InvalidProxyPort(t *testing.T) {
	os.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	os.Setenv("PROXY_PORT", "99999")
	defer clearEnv()

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "proxy port")
}

func TestLoad_SamePortConflict(t *testing.T) {
	os.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	os.Setenv("PROXY_PORT", "9090")
	os.Setenv("METRICS_PORT", "9090")
	defer clearEnv()

	cfg, err := Load()

	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be the same")
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	os.Setenv("HEADERS_TO_PROPAGATE", "x-request-id")
	os.Setenv("LOG_LEVEL", "invalid")
	defer clearEnv()

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
			result := parseHeaders(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_KEY", "test_value")
	defer os.Unsetenv("TEST_KEY")

	assert.Equal(t, "test_value", getEnv("TEST_KEY", "default"))
	assert.Equal(t, "default", getEnv("NONEXISTENT_KEY", "default"))
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	os.Setenv("TEST_INVALID_INT", "not_a_number")
	defer func() {
		os.Unsetenv("TEST_INT")
		os.Unsetenv("TEST_INVALID_INT")
	}()

	assert.Equal(t, 42, getEnvInt("TEST_INT", 10))
	assert.Equal(t, 10, getEnvInt("NONEXISTENT_INT", 10))
	assert.Equal(t, 10, getEnvInt("TEST_INVALID_INT", 10))
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    ProxyConfig
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid config",
			config: ProxyConfig{
				HeadersToPropagate: []string{"x-request-id"},
				TargetHost:         "localhost:8080",
				ProxyPort:          9090,
				LogLevel:           "info",
				MetricsPort:        9091,
			},
			expectErr: false,
		},
		{
			name: "invalid proxy port - too high",
			config: ProxyConfig{
				HeadersToPropagate: []string{"x-request-id"},
				TargetHost:         "localhost:8080",
				ProxyPort:          70000,
				LogLevel:           "info",
				MetricsPort:        9091,
			},
			expectErr: true,
			errMsg:    "proxy port",
		},
		{
			name: "invalid proxy port - zero",
			config: ProxyConfig{
				HeadersToPropagate: []string{"x-request-id"},
				TargetHost:         "localhost:8080",
				ProxyPort:          0,
				LogLevel:           "info",
				MetricsPort:        9091,
			},
			expectErr: true,
			errMsg:    "proxy port",
		},
		{
			name: "empty target host",
			config: ProxyConfig{
				HeadersToPropagate: []string{"x-request-id"},
				TargetHost:         "",
				ProxyPort:          9090,
				LogLevel:           "info",
				MetricsPort:        9091,
			},
			expectErr: true,
			errMsg:    "target host",
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

func clearEnv() {
	os.Unsetenv("HEADERS_TO_PROPAGATE")
	os.Unsetenv("TARGET_HOST")
	os.Unsetenv("PROXY_PORT")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("METRICS_PORT")
}
