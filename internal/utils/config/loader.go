package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
)

// LoadConfig reads, interpolates, and validates the application config.
// 1. Defaults configPath to "configs/config.json" when nil.
// 2. Reads the raw JSON bytes from disk via readConfigFile.
// 3. Unmarshals JSON into a flat map to extract "env_file_name" and "config".
// 4. Loads the declared env file (OS vars are never overridden) via loadEnvFile.
// 5. Resolves all .env:VAR and .env:type:VAR references in the config map via interpolateMap.
// 6. Loads the resolved map into a koanf instance using "." as the key delimiter.
// 7. Unmarshals koanf state into a Config struct using "koanf" struct tags.
// 8. Applies sensible defaults for optional fields via setDefaults.
// 9. Runs go-playground/validator constraints on the struct and formats any errors.
// 10. Returns the validated *Config, or the first error from any step above.
func LoadConfig(configPath *string) (*Config, error) {
	def := "configs/config.json"
	if configPath == nil {
		configPath = &def
	}

	rawBytes, err := readConfigFile(*configPath)
	if err != nil {
		return nil, err
	}

	var flat map[string]any
	if err := json.Unmarshal(rawBytes, &flat); err != nil {
		return nil, fmt.Errorf("parse %s: %w", *configPath, err)
	}

	envFile, _ := flat["env_file_name"].(string)
	configMap, _ := flat["config"].(map[string]any)

	if err := loadEnvFile(envFile); err != nil {
		return nil, err
	}

	interpolated, err := interpolateMap(configMap)
	if err != nil {
		return nil, fmt.Errorf("interpolate config: %w", err)
	}

	k := koanf.New(".")
	if err := k.Load(confmap.Provider(interpolated, "."), nil); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	setDefaults(&cfg)

	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, formatValidationErrors(err)
	}

	return &cfg, nil
}

// setDefaults fills in optional fields that are safe to omit from the env file.
// Called after unmarshal and before validation so defaults satisfy any constraints.
func setDefaults(cfg *Config) {
	if cfg.AppConfig.LogLevel == "" {
		cfg.AppConfig.LogLevel = "info"
	}
	if cfg.OpenTelConfig.SampleRate == 0 {
		cfg.OpenTelConfig.SampleRate = 1.0
	}
	if cfg.ServerConfig.ShutdownTimeout == 0 {
		cfg.ServerConfig.ShutdownTimeout = 30
	}
}

// readConfigFile returns the raw bytes of the config file at p.
func readConfigFile(p string) ([]byte, error) {
	expanded, err := expandPath(p)
	if err != nil {
		return nil, fmt.Errorf("expand config file path %q: %w", p, err)
	}
	raw, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", expanded, err)
	}
	return raw, nil
}

// loadEnvFile populates process environment from a .env file without overriding existing vars.
// Returns immediately when p is empty. A missing file is silently ignored.
func loadEnvFile(p string) error {
	if p == "" {
		return nil
	}
	expanded, err := expandPath(p)
	if err != nil {
		return fmt.Errorf("expand env file path %q: %w", p, err)
	}
	_ = godotenv.Load(expanded)
	return nil
}

// expandPath normalises a file path: resolves "~/" prefix, passes everything else through.
func expandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// PrintConfig prints the current configuration to stdout in a human-readable format.
// Sensitive fields (secrets, passwords, URIs) are masked.
func (c *Config) PrintConfig() {
	fmt.Println("=== App ===")
	fmt.Printf("  name:      %s\n", c.AppConfig.Name)
	fmt.Printf("  env:       %s\n", c.AppConfig.Env)
	fmt.Printf("  version:   %s\n", c.AppConfig.Version)
	fmt.Printf("  log_level: %s\n", c.AppConfig.LogLevel)

	fmt.Println("=== Server ===")
	fmt.Printf("  port:                 %d\n", c.ServerConfig.Port)
	fmt.Printf("  read_timeout:         %d\n", c.ServerConfig.ReadTimeout)
	fmt.Printf("  write_timeout:        %d\n", c.ServerConfig.WriteTimeout)
	fmt.Printf("  idle_timeout:         %d\n", c.ServerConfig.IdleTimeout)
	fmt.Printf("  shutdown_timeout:     %d\n", c.ServerConfig.ShutdownTimeout)
	fmt.Printf("  cors_allowed_origins: %v\n", c.ServerConfig.CORSAllowedOrigins)

	fmt.Println("=== Database ===")
	fmt.Printf("  uri:                %s\n", maskSecret(c.DbConfig.Uri))
	fmt.Printf("  max_open_conns:     %d\n", c.DbConfig.MaxOpenConns)
	fmt.Printf("  max_idle_conns:     %d\n", c.DbConfig.MaxIdleConns)
	fmt.Printf("  conn_max_lifetime:  %d\n", c.DbConfig.ConnMaxLifetime)
	fmt.Printf("  conn_max_idle_time: %d\n", c.DbConfig.ConnMaxIdleTime)

	fmt.Println("=== Redis ===")
	fmt.Printf("  addr:     %s\n", c.RedisConfig.Addr)
	fmt.Printf("  password: %s\n", maskSecret(c.RedisConfig.Password))
	fmt.Printf("  db:       %d\n", c.RedisConfig.DB)

	fmt.Println("=== OpenTelemetry ===")
	fmt.Printf("  endpoint:    %s\n", c.OpenTelConfig.Endpoint)
	fmt.Printf("  enabled:     %v\n", c.OpenTelConfig.Enabled)
	fmt.Printf("  sample_rate: %.2f\n", c.OpenTelConfig.SampleRate)

	fmt.Println("=== Auth ===")
	fmt.Printf("  access_token_secret:  %s\n", maskSecret(c.AuthConfig.AccessTokenSecret))
	fmt.Printf("  refresh_token_secret: %s\n", maskSecret(c.AuthConfig.RefreshTokenSecret))
	fmt.Printf("  access_token_expiry:  %d\n", c.AuthConfig.AccessTokenExpiry)
	fmt.Printf("  refresh_token_expiry: %d\n", c.AuthConfig.RefreshTokenExpiry)
}

// maskSecret shows the first 4 characters of s and replaces the rest with asterisks.
func maskSecret(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-4)
}
