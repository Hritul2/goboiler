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

type rawConfigFile struct {
	EnvFileLocation string         `json:"envFileLocation"`
	Config          map[string]any `json:"config"`
}

// LoadConfig reads the config file at configPath, loads the env file declared
// within it, interpolates .env:type:VAR references, and returns a validated Config.
// configPath supports ~ and absolute paths. OS environment always takes priority
// over values in envFileLocation.
func LoadConfig(configPath *string) (*Config, error) {
	def := "configs/config.json"
	if configPath == nil {
		configPath = &def
	}

	raw, err := readConfigFile(*configPath)
	if err != nil {
		return nil, err
	}

	var rcf rawConfigFile
	if err := json.Unmarshal(raw, &rcf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", *configPath, err)
	}

	if err := loadEnvFile(rcf.EnvFileLocation); err != nil {
		return nil, err
	}

	interpolated, err := interpolateMap(rcf.Config)
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

	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, formatValidationErrors(err)
	}

	return &cfg, nil
}

// readConfigFile reads the file at p after expanding ~ and absolute paths.
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

// loadEnvFile loads the env file at p. godotenv.Load never overrides already-set
// vars, so OS environment always wins. Missing file is silently ignored.
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

// expandPath resolves ~ to the user's home directory.
// Absolute paths (/) are returned as-is.
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
