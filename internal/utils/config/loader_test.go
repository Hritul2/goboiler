package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
		assert.Equal(t, "info", cfg.App.LogLevel)
		assert.Equal(t, 1.0, cfg.OTel.SampleRate)
		assert.Equal(t, 30, cfg.Server.ShutdownTimeout)
		assert.Equal(t, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, cfg.Server.CORSAllowedMethods)
		assert.Equal(t, []string{"Content-Type", "Authorization"}, cfg.Server.CORSAllowedHeaders)
		assert.Equal(t, 600, cfg.Server.CORSMaxAgeSeconds)
		assert.EqualValues(t, 2<<20, cfg.Server.MaxRequestBodyBytes)
		assert.EqualValues(t, 10<<20, cfg.Server.MaxResponseBodyBytes)
		assert.Equal(t, 100, cfg.Server.RateLimitPerMinute)
		assert.Equal(t, 20, cfg.Server.RateLimitBurst)
	})

	t.Run("does_not_overwrite_set_fields", func(t *testing.T) {
		cfg := &Config{}
		cfg.App.LogLevel = "warn"
		cfg.OTel.SampleRate = 0.5
		cfg.Server.ShutdownTimeout = 60
		cfg.Server.CORSAllowedMethods = []string{"GET"}
		cfg.Server.CORSAllowedHeaders = []string{"X-Custom"}
		cfg.Server.CORSMaxAgeSeconds = 42
		cfg.Server.MaxRequestBodyBytes = 1024
		cfg.Server.MaxResponseBodyBytes = 2048
		cfg.Server.RateLimitPerMinute = 5
		cfg.Server.RateLimitBurst = 2
		setDefaults(cfg)
		assert.Equal(t, "warn", cfg.App.LogLevel)
		assert.Equal(t, 0.5, cfg.OTel.SampleRate)
		assert.Equal(t, 60, cfg.Server.ShutdownTimeout)
		assert.Equal(t, []string{"GET"}, cfg.Server.CORSAllowedMethods)
		assert.Equal(t, []string{"X-Custom"}, cfg.Server.CORSAllowedHeaders)
		assert.Equal(t, 42, cfg.Server.CORSMaxAgeSeconds)
		assert.EqualValues(t, 1024, cfg.Server.MaxRequestBodyBytes)
		assert.EqualValues(t, 2048, cfg.Server.MaxResponseBodyBytes)
		assert.Equal(t, 5, cfg.Server.RateLimitPerMinute)
		assert.Equal(t, 2, cfg.Server.RateLimitBurst)
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
		assert.False(t, cfg.Server.CORSAllowCredentials)
	})
}

func TestLoadConfig_valid(t *testing.T) {
	path := writeConfigFile(t, validConfigJSON())
	cfg, err := LoadConfig(&path)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "test-app", cfg.App.Name)
	assert.Equal(t, "dev", cfg.App.Env)
	assert.Equal(t, "1.0.0", cfg.App.Version)
	assert.Equal(t, "debug", cfg.App.LogLevel)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30, cfg.Server.ShutdownTimeout)
	assert.Equal(t, []string{"http://localhost:3000"}, cfg.Server.CORSAllowedOrigins)
	assert.False(t, cfg.OTel.Enabled)
	assert.Equal(t, 1.0, cfg.OTel.SampleRate)
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
	assert.Equal(t, "info", result.App.LogLevel)
	assert.Equal(t, 1.0, result.OTel.SampleRate)
	assert.Equal(t, 30, result.Server.ShutdownTimeout)
}

func TestLoadConfig_envInterpolation(t *testing.T) {
	t.Setenv("TESTCFG_VERSION", "9.9.9")

	raw := validConfigJSON()
	raw["config"].(map[string]any)["app"].(map[string]any)["version"] = ".env:TESTCFG_VERSION"

	path := writeConfigFile(t, raw)
	cfg, err := LoadConfig(&path)
	require.NoError(t, err)
	assert.Equal(t, "9.9.9", cfg.App.Version)
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
	assert.False(t, cfg.Email.Enabled)
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
	assert.True(t, cfg.Email.Enabled)
	assert.Equal(t, "smtp.example.com", cfg.Email.Host)
	assert.Equal(t, 587, cfg.Email.Port)
	assert.Equal(t, "mailer", cfg.Email.Username)
	assert.Equal(t, "s3cret", cfg.Email.Password)
	assert.Equal(t, "noreply@example.com", cfg.Email.From)
	assert.Equal(t, "Example", cfg.Email.FromName)
}

// TestConfig_SectionKeysUnchanged pins the config-file section names (koanf
// tags) of the named Config fields, so renaming a Go field can never change a
// config key or the ".env:VAR" mapping users already have.
func TestConfig_SectionKeysUnchanged(t *testing.T) {
	want := map[string]string{
		"App": "app", "Server": "server", "DB": "db", "OTel": "otel",
		"Redis": "redis", "Auth": "auth", "Email": "email",
	}
	typ := reflect.TypeFor[Config]()
	seen := 0
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue // bookkeeping such as Config.sources, not configuration
		}
		seen++
		assert.False(t, f.Anonymous, "%s must be a named field, not embedded", f.Name)
		assert.Equal(t, want[f.Name], f.Tag.Get("koanf"), f.Name)
	}
	require.Equal(t, len(want), seen)
}

// TestLoadConfig_PopulatesEverySection loads a complete config and checks
// one value per section lands in its named field.
func TestLoadConfig_PopulatesEverySection(t *testing.T) {
	path := writeConfigFile(t, validConfigJSON())
	cfg, err := LoadConfig(&path)
	require.NoError(t, err)

	assert.Equal(t, "test-app", cfg.App.Name)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "mongodb://localhost:27017", cfg.DB.Uri)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, 900, cfg.Auth.AccessTokenExpiry)
	assert.False(t, cfg.Email.Enabled)
	assert.False(t, cfg.OTel.Enabled)
}
