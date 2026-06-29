package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// typedRefPattern matches .env:type:VAR_NAME (explicit type)
var typedRefPattern = regexp.MustCompile(`^\.env:(string|int|float|bool|array):(.+)$`)

// bareRefPattern matches .env:VAR_NAME (string shorthand, as in cv2rust)
var bareRefPattern = regexp.MustCompile(`^\.env:([^:]+)$`)

// interpolateMap resolves env references in a config map.
//  1. Allocates a result map of the same size.
//  2. For each key-value pair:
//     a. string  → calls resolveEnvRef and stores the result.
//     b. map     → recurses into the nested map.
//     c. other   → copies the value unchanged.
//  3. Returns the fully resolved map, or the first error encountered.
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

// resolveEnvRef converts a config value string to its runtime value.
// 1. Tests s against typedRefPattern (.env:type:VAR); if matched, delegates to parseTypedEnvValue.
// 2. Tests s against bareRefPattern (.env:VAR); if matched, returns os.Getenv of the var name.
// 3. Returns s unchanged if neither pattern matches.
func resolveEnvRef(s string) (any, error) {
	if matches := typedRefPattern.FindStringSubmatch(s); matches != nil {
		typ, varName := matches[1], matches[2]
		raw := os.Getenv(varName)
		return parseTypedEnvValue(raw, typ, varName)
	}
	if matches := bareRefPattern.FindStringSubmatch(s); matches != nil {
		return os.Getenv(matches[1]), nil
	}
	return s, nil
}

// parseTypedEnvValue coerces the raw env string to the requested Go type.
// 1. "string" → returns raw as-is.
// 2. "int"    → returns 0 for empty string; otherwise parses with strconv.Atoi.
// 3. "float"  → returns 0.0 for empty string; otherwise parses with strconv.ParseFloat(64).
// 4. "bool"   → returns false for empty string; otherwise parses with strconv.ParseBool.
// 5. "array"  → returns []string{} for empty string; otherwise splits on "," and trims spaces.
// 6. Any unrecognised type falls through and returns raw unchanged.
func parseTypedEnvValue(raw, typ, varName string) (any, error) {

	switch typ {
	case "string":
		return raw, nil

	case "int":
		if raw == "" {
			return 0, nil
		}
		v, err := strconv.Atoi(raw) // Atoi returns an error if the string is not a valid integer
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

// formatValidationErrors converts a validator.ValidationErrors into a human-readable error.
// 1. Type-asserts err to validator.ValidationErrors; wraps generically if assertion fails.
// 2. Writes a header line "config validation failed:".
// 3. Appends one line per field error: "  - <namespace>: failed '<tag>' constraint".
// 4. Trims the trailing newline and returns the formatted error.
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
