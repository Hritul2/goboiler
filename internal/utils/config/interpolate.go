package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

var envRefPattern = regexp.MustCompile(`^\.env:(string|int|float|bool|array):(.+)$`)

func interpolateMap(m map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			resolved, err := resolveEnvRef(val)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", k, err)
			}
			result[k] = resolved
		case map[string]any:
			nested, err := interpolateMap(val)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", k, err)
			}
			result[k] = nested
		default:
			result[k] = v
		}
	}
	return result, nil
}

func resolveEnvRef(s string) (any, error) {
	matches := envRefPattern.FindStringSubmatch(s)
	if matches == nil {
		return s, nil
	}
	typ, varName := matches[1], matches[2]
	raw := os.Getenv(varName)

	switch typ {
	case "string":
		return raw, nil

	case "int":
		if raw == "" {
			return 0, nil
		}
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("env var %q: expected int, got %q", varName, raw)
		}
		return v, nil

	case "float":
		if raw == "" {
			return 0.0, nil
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("env var %q: expected float, got %q", varName, raw)
		}
		return v, nil

	case "bool":
		if raw == "" {
			return false, nil
		}
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("env var %q: expected bool (true/false/1/0), got %q", varName, raw)
		}
		return v, nil

	case "array":
		if raw == "" {
			return []string{}, nil
		}
		parts := strings.Split(raw, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}
		return parts, nil
	}

	return raw, nil
}

func formatValidationErrors(err error) error {
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		return fmt.Errorf("config validation: %w", err)
	}
	var b strings.Builder
	b.WriteString("config validation failed:\n")
	for _, fe := range ve {
		fmt.Fprintf(&b, "  - %s: failed '%s' constraint\n", fe.Namespace(), fe.Tag())
	}
	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}
