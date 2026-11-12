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

// LoadConfig reads, layers, interpolates, and validates the application config.
//
// Priority, lowest to highest:
//  1. the base JSON file (default "configs/config.json", or *configPath),
//  2. its optional override "<name>.local.json" next to it
//     (configs/config.local.json), deep-merged on top (see mergeMaps),
//  3. values referenced from env files with ".env:VAR" — the override's
//     "env_file_name" is loaded before (so beats) the base's —
//  4. real OS environment variables, which are never overridden by a file.
//
// Steps:
//  1. Defaults configPath to "configs/config.json" when nil.
//  2. Reads the base file, and the override file if it exists.
//  3. Deep-merges them (objects merge, arrays and scalars replace).
//  4. Loads the env file(s) named by the merged "env_file_name" and the base's
//     (OS vars are never overridden; the first file to set a var wins, so the
//     override's file is loaded first).
//  5. Resolves all .env:VAR and .env:type:VAR references in the MERGED config
//     map via interpolateMap.
//  6. Loads the resolved map into a koanf instance using "." as the key delimiter.
//  7. Unmarshals koanf state into a Config struct using "koanf" struct tags.
//  8. Applies sensible defaults for optional fields via setDefaults.
//  9. Runs go-playground/validator constraints on the struct and formats any errors.
//  10. Returns the validated *Config (Sources() says which files fed it), or
//     the first error from any step above.
func LoadConfig(configPath *string) (*Config, error) {
	def := "configs/config.json"
	if configPath == nil {
		configPath = &def
	}

	rawBytes, err := readConfigFile(*configPath)
	if err != nil {
		return nil, err
	}

	var base map[string]any
	if err := json.Unmarshal(rawBytes, &base); err != nil {
		return nil, fmt.Errorf("parse %s: %w", *configPath, err)
	}
	sources := Sources{ConfigFiles: []string{*configPath}}

	merged := base
	localPath := localPathFor(*configPath)
	local, found, err := readOptionalConfigFile(localPath)
	if err != nil {
		return nil, err
	}
	if found {
		merged = mergeMaps(base, local)
		sources.ConfigFiles = append(sources.ConfigFiles, localPath)
	}

	baseEnvFile, _ := base["env_file_name"].(string)
	localEnvFile, _ := local["env_file_name"].(string)
	configMap, _ := merged["config"].(map[string]any)

	// godotenv never overrides a variable that is already set, so whichever
	// file is loaded first wins: load the higher-priority (local) one first.
	// Real OS variables are already set, so they beat both.
	for _, envFile := range []string{localEnvFile, baseEnvFile} {
		if envFile == "" || containsEnvFile(sources.EnvFiles, envFile) {
			continue
		}
		loaded, err := loadEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		sources.EnvFiles = append(sources.EnvFiles, EnvFile{Path: envFile, Loaded: loaded})
	}
	// Report lowest priority first (the load order above was highest first).
	for i, j := 0, len(sources.EnvFiles)-1; i < j; i, j = i+1, j-1 {
		sources.EnvFiles[i], sources.EnvFiles[j] = sources.EnvFiles[j], sources.EnvFiles[i]
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
	cfg.sources = sources

	setDefaults(&cfg)

	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, formatValidationErrors(err)
	}

	return &cfg, nil
}

func containsEnvFile(files []EnvFile, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}

// setDefaults fills in optional fields that are safe to omit from the env file.
// Called after unmarshal and before validation so defaults satisfy any constraints.
func setDefaults(cfg *Config) {
	if cfg.App.LogLevel == "" {
		cfg.App.LogLevel = "info"
	}
	if cfg.OTel.SampleRate == 0 {
		cfg.OTel.SampleRate = 1.0
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 30
	}
	if len(cfg.Server.CORSAllowedMethods) == 0 {
		cfg.Server.CORSAllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(cfg.Server.CORSAllowedHeaders) == 0 {
		cfg.Server.CORSAllowedHeaders = []string{"Content-Type", "Authorization"}
	}
	if cfg.Server.CORSMaxAgeSeconds == 0 {
		cfg.Server.CORSMaxAgeSeconds = 600
	}
	// CORSAllowCredentials intentionally has no default here — see the
	// ServerConfig doc comment.
	if cfg.Server.MaxRequestBodyBytes == 0 {
		cfg.Server.MaxRequestBodyBytes = 2 << 20 // 2MiB
	}
	if cfg.Server.MaxResponseBodyBytes == 0 {
		cfg.Server.MaxResponseBodyBytes = 10 << 20 // 10MiB
	}
	if cfg.Server.RateLimitPerMinute == 0 {
		cfg.Server.RateLimitPerMinute = 100
	}
	if cfg.Server.RateLimitBurst == 0 {
		cfg.Server.RateLimitBurst = 20
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
// It reports whether the file existed and was loaded; a missing file is silently
// ignored (an optional .env.local is the common case), so only a failure to
// resolve the path is an error. Returns (false, nil) immediately when p is empty.
func loadEnvFile(p string) (loaded bool, err error) {
	if p == "" {
		return false, nil
	}
	expanded, err := expandPath(p)
	if err != nil {
		return false, fmt.Errorf("expand env file path %q: %w", p, err)
	}
	return godotenv.Load(expanded) == nil, nil
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
	fmt.Print(c.sources.report())

	fmt.Println("=== App ===")
	fmt.Printf("  name:      %s\n", c.App.Name)
	fmt.Printf("  env:       %s\n", c.App.Env)
	fmt.Printf("  version:   %s\n", c.App.Version)
	fmt.Printf("  log_level: %s\n", c.App.LogLevel)

	fmt.Println("=== Server ===")
	fmt.Printf("  port:                 %d\n", c.Server.Port)
	fmt.Printf("  read_timeout:         %d\n", c.Server.ReadTimeout)
	fmt.Printf("  write_timeout:        %d\n", c.Server.WriteTimeout)
	fmt.Printf("  idle_timeout:         %d\n", c.Server.IdleTimeout)
	fmt.Printf("  shutdown_timeout:     %d\n", c.Server.ShutdownTimeout)
	fmt.Printf("  cors_allowed_origins:   %v\n", c.Server.CORSAllowedOrigins)
	fmt.Printf("  cors_allowed_methods:   %v\n", c.Server.CORSAllowedMethods)
	fmt.Printf("  cors_allowed_headers:   %v\n", c.Server.CORSAllowedHeaders)
	fmt.Printf("  cors_allow_credentials: %v\n", c.Server.CORSAllowCredentials)
	fmt.Printf("  cors_max_age_seconds:   %d\n", c.Server.CORSMaxAgeSeconds)
	fmt.Printf("  max_request_body_bytes:  %d\n", c.Server.MaxRequestBodyBytes)
	fmt.Printf("  max_response_body_bytes: %d\n", c.Server.MaxResponseBodyBytes)
	fmt.Printf("  rate_limit_per_minute:   %d\n", c.Server.RateLimitPerMinute)
	fmt.Printf("  rate_limit_burst:        %d\n", c.Server.RateLimitBurst)

	fmt.Println("=== Database ===")
	fmt.Printf("  uri:                %s\n", maskSecret(c.DB.Uri))
	fmt.Printf("  name:               %s\n", c.DB.Name)
	fmt.Printf("  max_open_conns:     %d\n", c.DB.MaxOpenConns)
	fmt.Printf("  max_idle_conns:     %d\n", c.DB.MaxIdleConns)
	fmt.Printf("  conn_max_lifetime:  %d\n", c.DB.ConnMaxLifetime)
	fmt.Printf("  conn_max_idle_time: %d\n", c.DB.ConnMaxIdleTime)

	fmt.Println("=== Redis ===")
	fmt.Printf("  addr:     %s\n", c.Redis.Addr)
	fmt.Printf("  password: %s\n", maskSecret(c.Redis.Password))
	fmt.Printf("  db:       %d\n", c.Redis.DB)

	fmt.Println("=== OpenTelemetry ===")
	fmt.Printf("  endpoint:    %s\n", c.OTel.Endpoint)
	fmt.Printf("  enabled:     %v\n", c.OTel.Enabled)
	fmt.Printf("  sample_rate: %.2f\n", c.OTel.SampleRate)

	fmt.Println("=== Email ===")
	fmt.Printf("  enabled:   %v\n", c.Email.Enabled)
	fmt.Printf("  host:      %s\n", c.Email.Host)
	fmt.Printf("  port:      %d\n", c.Email.Port)
	fmt.Printf("  username:  %s\n", c.Email.Username)
	fmt.Printf("  password:  %s\n", maskSecret(c.Email.Password))
	fmt.Printf("  from:      %s\n", c.Email.From)
	fmt.Printf("  from_name: %s\n", c.Email.FromName)

	fmt.Println("=== Auth ===")
	fmt.Printf("  access_token_secret:  %s\n", maskSecret(c.Auth.AccessTokenSecret))
	fmt.Printf("  refresh_token_secret: %s\n", maskSecret(c.Auth.RefreshTokenSecret))
	fmt.Printf("  access_token_expiry:  %d\n", c.Auth.AccessTokenExpiry)
	fmt.Printf("  refresh_token_expiry: %d\n", c.Auth.RefreshTokenExpiry)
}

// maskSecret shows the first 4 characters of s and replaces the rest with asterisks.
func maskSecret(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-4)
}
