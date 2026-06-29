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
// 8. Runs go-playground/validator constraints on the struct and formats any errors.
// 9. Returns the validated *Config, or the first error from any step above.
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

	k := koanf.New(".")                                                      // use "." as the delimiter for nested keys
	if err := k.Load(confmap.Provider(interpolated, "."), nil); err != nil { // Load the interpolated config map into koanf
		return nil, fmt.Errorf("load config: %w", err)
	}

	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, formatValidationErrors(err)
	}

	return &cfg, nil
}

// readConfigFile returns the raw bytes of the config file at p.
// 1. Expands ~ and absolute paths via expandPath.
// 2. Reads the file with os.ReadFile and returns its bytes.
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
// 1. Returns immediately when p is empty (no env file declared).
// 2. Expands ~ and absolute paths via expandPath.
// 3. Calls godotenv.Load; the error is discarded so a missing file is silently ignored.
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

// expandPath normalises a file path for use with os.ReadFile.
// 1. If p starts with "~/", resolves the home directory via os.UserHomeDir and joins the remainder.
// 2. Otherwise returns p unchanged (handles absolute and relative paths).
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
