package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfigFile writes JSON content to a temp file and returns its path.
func writeConfigFile(t *testing.T, content any) string {
	t.Helper()
	b, err := json.Marshal(content)
	require.NoError(t, err)
	f, err := os.CreateTemp(t.TempDir(), "config*.json")
	require.NoError(t, err)
	_, err = f.Write(b)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// validConfigJSON returns a minimal valid config map with no .env: references.
func validConfigJSON() map[string]any {
	return map[string]any{
		"env_file_name": "",
		"config": map[string]any{
			"app": map[string]any{
				"name":      "test-app",
				"env":       "dev",
				"version":   "1.0.0",
				"log_level": "debug",
			},
			"server": map[string]any{
				"port":                 8080,
				"read_timeout":         30,
				"write_timeout":        30,
				"idle_timeout":         120,
				"shutdown_timeout":     30,
				"cors_allowed_origins": []string{"http://localhost:3000"},
			},
			"db": map[string]any{
				"uri":                "mongodb://localhost:27017",
				"name":               "test-db",
				"max_open_conns":     25,
				"max_idle_conns":     10,
				"conn_max_lifetime":  300,
				"conn_max_idle_time": 60,
			},
			"otel": map[string]any{
				"enabled":     false,
				"endpoint":    "",
				"sample_rate": 1.0,
			},
			"redis": map[string]any{
				"addr":     "localhost:6379",
				"password": "",
				"db":       0,
			},
			"auth": map[string]any{
				"access_token_secret":  "test-access-token-secret-min-32-chars",
				"refresh_token_secret": "test-refresh-token-secret-min-32-chars",
				"access_token_expiry":  900,
				"refresh_token_expiry": 604800,
			},
			"email": map[string]any{
				"enabled":   false,
				"host":      "",
				"port":      0,
				"username":  "",
				"password":  "",
				"from":      "",
				"from_name": "",
			},
		},
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"ab", "**"},
		{"abcd", "****"},
		{"abcde", "abcd*"},
		{"abcdefghij", "abcd******"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, maskSecret(tt.input))
		})
	}
}

func TestExpandPath(t *testing.T) {
	t.Run("tilde_expanded", func(t *testing.T) {
		home, err := os.UserHomeDir()
		require.NoError(t, err)
		got, err := expandPath("~/foo/bar")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, "foo/bar"), got)
	})

	t.Run("relative_path_unchanged", func(t *testing.T) {
		got, err := expandPath("configs/config.json")
		require.NoError(t, err)
		assert.Equal(t, "configs/config.json", got)
	})

	t.Run("absolute_path_unchanged", func(t *testing.T) {
		got, err := expandPath("/tmp/foo")
		require.NoError(t, err)
		assert.Equal(t, "/tmp/foo", got)
	})
}

func TestSetDefaults(t *testing.T) {
	t.Run("fills_all_zero_fields", func(t *testing.T) {
		cfg := &Config{}
		setDefaults(cfg)
		assert.Equal(t, "info", cfg.AppConfig.LogLevel)
		assert.Equal(t, 1.0, cfg.OpenTelConfig.SampleRate)
		assert.Equal(t, 30, cfg.ServerConfig.ShutdownTimeout)
		assert.Equal(t, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, cfg.ServerConfig.CORSAllowedMethods)
		assert.Equal(t, []string{"Content-Type", "Authorization"}, cfg.ServerConfig.CORSAllowedHeaders)
		assert.Equal(t, 600, cfg.ServerConfig.CORSMaxAgeSeconds)
		assert.EqualValues(t, 2<<20, cfg.ServerConfig.MaxRequestBodyBytes)
		assert.EqualValues(t, 10<<20, cfg.ServerConfig.MaxResponseBodyBytes)
		assert.Equal(t, 100, cfg.ServerConfig.RateLimitPerMinute)
		assert.Equal(t, 20, cfg.ServerConfig.RateLimitBurst)
	})

	t.Run("does_not_overwrite_set_fields", func(t *testing.T) {
		cfg := &Config{}
		cfg.AppConfig.LogLevel = "warn"
		cfg.OpenTelConfig.SampleRate = 0.5
		cfg.ServerConfig.ShutdownTimeout = 60
		cfg.ServerConfig.CORSAllowedMethods = []string{"GET"}
		cfg.ServerConfig.CORSAllowedHeaders = []string{"X-Custom"}
		cfg.ServerConfig.CORSMaxAgeSeconds = 42
		cfg.ServerConfig.MaxRequestBodyBytes = 1024
		cfg.ServerConfig.MaxResponseBodyBytes = 2048
		cfg.ServerConfig.RateLimitPerMinute = 5
		cfg.ServerConfig.RateLimitBurst = 2
		setDefaults(cfg)
		assert.Equal(t, "warn", cfg.AppConfig.LogLevel)
		assert.Equal(t, 0.5, cfg.OpenTelConfig.SampleRate)
		assert.Equal(t, 60, cfg.ServerConfig.ShutdownTimeout)
		assert.Equal(t, []string{"GET"}, cfg.ServerConfig.CORSAllowedMethods)
		assert.Equal(t, []string{"X-Custom"}, cfg.ServerConfig.CORSAllowedHeaders)
		assert.Equal(t, 42, cfg.ServerConfig.CORSMaxAgeSeconds)
		assert.EqualValues(t, 1024, cfg.ServerConfig.MaxRequestBodyBytes)
		assert.EqualValues(t, 2048, cfg.ServerConfig.MaxResponseBodyBytes)
		assert.Equal(t, 5, cfg.ServerConfig.RateLimitPerMinute)
		assert.Equal(t, 2, cfg.ServerConfig.RateLimitBurst)
	})

	t.Run("credentials_default_to_false_secure_by_default", func(t *testing.T) {
		// CORSAllowCredentials is deliberately NOT defaulted in setDefaults: a
		// bool's zero value (false) is indistinguishable from "unset", so
		// defaulting it to true here would silently override an explicit
		// `false` in config. Leaving it at Go's natural zero value means it's
		// closed (no credential sharing) unless a config explicitly opts in —
		// the safer default for a boilerplate.
		cfg := &Config{}
		setDefaults(cfg)
		assert.False(t, cfg.ServerConfig.CORSAllowCredentials)
	})
}

func TestLoadConfig_valid(t *testing.T) {
	path := writeConfigFile(t, validConfigJSON())
	cfg, err := LoadConfig(&path)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "test-app", cfg.AppConfig.Name)
	assert.Equal(t, "dev", cfg.AppConfig.Env)
	assert.Equal(t, "1.0.0", cfg.AppConfig.Version)
	assert.Equal(t, "debug", cfg.AppConfig.LogLevel)
	assert.Equal(t, 8080, cfg.ServerConfig.Port)
	assert.Equal(t, 30, cfg.ServerConfig.ShutdownTimeout)
	assert.Equal(t, []string{"http://localhost:3000"}, cfg.ServerConfig.CORSAllowedOrigins)
	assert.False(t, cfg.OpenTelConfig.Enabled)
	assert.Equal(t, 1.0, cfg.OpenTelConfig.SampleRate)
}

func TestLoadConfig_invalidJSON(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "config*.json")
	require.NoError(t, err)
	_, err = f.WriteString("{bad json")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	path := f.Name()

	_, err = LoadConfig(&path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestLoadConfig_missingRequired(t *testing.T) {
	raw := validConfigJSON()
	// Remove the required "name" field from app config
	appSection := raw["config"].(map[string]any)["app"].(map[string]any)
	delete(appSection, "name")

	path := writeConfigFile(t, raw)
	_, err := LoadConfig(&path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config validation failed")
}

func TestLoadConfig_fileNotFound(t *testing.T) {
	path := "/tmp/this-file-does-not-exist-12345.json"
	_, err := LoadConfig(&path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read")
}

func TestLoadConfig_defaults(t *testing.T) {
	raw := validConfigJSON()
	cfg := raw["config"].(map[string]any)
	app := cfg["app"].(map[string]any)
	server := cfg["server"].(map[string]any)
	otel := cfg["otel"].(map[string]any)

	delete(app, "log_level")
	delete(otel, "sample_rate")
	delete(server, "shutdown_timeout")

	path := writeConfigFile(t, raw)
	result, err := LoadConfig(&path)
	require.NoError(t, err)
	assert.Equal(t, "info", result.AppConfig.LogLevel)
	assert.Equal(t, 1.0, result.OpenTelConfig.SampleRate)
	assert.Equal(t, 30, result.ServerConfig.ShutdownTimeout)
}

func TestLoadConfig_envInterpolation(t *testing.T) {
	t.Setenv("TESTCFG_VERSION", "9.9.9")

	raw := validConfigJSON()
	raw["config"].(map[string]any)["app"].(map[string]any)["version"] = ".env:TESTCFG_VERSION"

	path := writeConfigFile(t, raw)
	cfg, err := LoadConfig(&path)
	require.NoError(t, err)
	assert.Equal(t, "9.9.9", cfg.AppConfig.Version)
}

func TestLoadConfig_otelDisabledNoEndpoint(t *testing.T) {
	raw := validConfigJSON()
	raw["config"].(map[string]any)["otel"] = map[string]any{
		"enabled":     false,
		"endpoint":    "",
		"sample_rate": 1.0,
	}
	path := writeConfigFile(t, raw)
	_, err := LoadConfig(&path)
	require.NoError(t, err)
}

func TestLoadConfig_emailDisabledNoHost(t *testing.T) {
	// The fixture's email block is already disabled with empty fields —
	// mirroring the otel required_if pattern, this must load fine.
	path := writeConfigFile(t, validConfigJSON())
	cfg, err := LoadConfig(&path)
	require.NoError(t, err)
	assert.False(t, cfg.EmailConfig.Enabled)
}

func TestLoadConfig_emailEnabledMissingHost(t *testing.T) {
	raw := validConfigJSON()
	raw["config"].(map[string]any)["email"].(map[string]any)["enabled"] = true

	path := writeConfigFile(t, raw)
	_, err := LoadConfig(&path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config validation failed")
}

func TestLoadConfig_emailEnabledValid(t *testing.T) {
	raw := validConfigJSON()
	raw["config"].(map[string]any)["email"] = map[string]any{
		"enabled":   true,
		"host":      "smtp.example.com",
		"port":      587,
		"username":  "mailer",
		"password":  "s3cret",
		"from":      "noreply@example.com",
		"from_name": "Example",
	}

	path := writeConfigFile(t, raw)
	cfg, err := LoadConfig(&path)
	require.NoError(t, err)
	assert.True(t, cfg.EmailConfig.Enabled)
	assert.Equal(t, "smtp.example.com", cfg.EmailConfig.Host)
	assert.Equal(t, 587, cfg.EmailConfig.Port)
	assert.Equal(t, "mailer", cfg.EmailConfig.Username)
	assert.Equal(t, "s3cret", cfg.EmailConfig.Password)
	assert.Equal(t, "noreply@example.com", cfg.EmailConfig.From)
	assert.Equal(t, "Example", cfg.EmailConfig.FromName)
}
