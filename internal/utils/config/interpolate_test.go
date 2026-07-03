package config

import (
	"fmt"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTypedEnvValue(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		typ     string
		varName string
		want    any
		wantErr bool
	}{
		{"string_passthrough", "hello", "string", "X", "hello", false},
		{"int_valid", "42", "int", "X", 42, false},
		{"int_empty", "", "int", "X", 0, false},
		{"int_invalid", "abc", "int", "MYVAR", nil, true},
		{"float_valid", "3.14", "float", "X", 3.14, false},
		{"float_empty", "", "float", "X", 0.0, false},
		{"float_invalid", "nope", "float", "MYFLOAT", nil, true},
		{"bool_true", "true", "bool", "X", true, false},
		{"bool_false_zero", "0", "bool", "X", false, false},
		{"bool_empty", "", "bool", "X", false, false},
		{"bool_invalid", "maybe", "bool", "MYBOOL", nil, true},
		{"array_csv", "a,b, c", "array", "X", []string{"a", "b", "c"}, false},
		{"array_empty", "", "array", "X", []string{}, false},
		{"array_single", "x", "array", "X", []string{"x"}, false},
		{"unknown_type_passthrough", "x", "uuid", "X", "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTypedEnvValue(tt.raw, tt.typ, tt.varName)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.varName)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveEnvRef(t *testing.T) {
	t.Run("typed_int_ref", func(t *testing.T) {
		t.Setenv("TEST_PORT", "9000")
		got, err := resolveEnvRef(".env:int:TEST_PORT")
		require.NoError(t, err)
		assert.Equal(t, 9000, got)
	})

	t.Run("bare_string_ref", func(t *testing.T) {
		t.Setenv("TEST_MY_VAR", "hello")
		got, err := resolveEnvRef(".env:TEST_MY_VAR")
		require.NoError(t, err)
		assert.Equal(t, "hello", got)
	})

	t.Run("plain_string_unchanged", func(t *testing.T) {
		got, err := resolveEnvRef("not-a-ref")
		require.NoError(t, err)
		assert.Equal(t, "not-a-ref", got)
	})

	t.Run("unset_bare_ref_returns_empty", func(t *testing.T) {
		got, err := resolveEnvRef(".env:DEFINITELY_UNSET_VAR_12345")
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})

	t.Run("typed_bool_ref", func(t *testing.T) {
		t.Setenv("TEST_FLAG", "true")
		got, err := resolveEnvRef(".env:bool:TEST_FLAG")
		require.NoError(t, err)
		assert.Equal(t, true, got)
	})

	t.Run("typed_array_ref", func(t *testing.T) {
		t.Setenv("TEST_ORIGINS", "http://a.com,http://b.com")
		got, err := resolveEnvRef(".env:array:TEST_ORIGINS")
		require.NoError(t, err)
		assert.Equal(t, []string{"http://a.com", "http://b.com"}, got)
	})

	t.Run("typed_invalid_propagates_error", func(t *testing.T) {
		t.Setenv("TEST_BAD_INT", "notanumber")
		_, err := resolveEnvRef(".env:int:TEST_BAD_INT")
		require.Error(t, err)
	})
}

func TestInterpolateMap(t *testing.T) {
	t.Run("non_string_values_copied_unchanged", func(t *testing.T) {
		input := map[string]any{
			"count":  42,
			"active": true,
			"ratio":  1.5,
		}
		result, err := interpolateMap(input)
		require.NoError(t, err)
		assert.Equal(t, 42, result["count"])
		assert.Equal(t, true, result["active"])
		assert.Equal(t, 1.5, result["ratio"])
	})

	t.Run("env_ref_in_string_value_resolved", func(t *testing.T) {
		t.Setenv("INTERP_FOO", "bar")
		input := map[string]any{"k": ".env:INTERP_FOO"}
		result, err := interpolateMap(input)
		require.NoError(t, err)
		assert.Equal(t, "bar", result["k"])
	})

	t.Run("plain_string_unchanged", func(t *testing.T) {
		input := map[string]any{"name": "golang-boilerplate"}
		result, err := interpolateMap(input)
		require.NoError(t, err)
		assert.Equal(t, "golang-boilerplate", result["name"])
	})

	t.Run("nested_map_recurses", func(t *testing.T) {
		t.Setenv("INTERP_NESTED_VAR", "nested-value")
		input := map[string]any{
			"outer": map[string]any{
				"inner": ".env:INTERP_NESTED_VAR",
			},
		}
		result, err := interpolateMap(input)
		require.NoError(t, err)
		outer, ok := result["outer"].(map[string]any)
		require.True(t, ok, "outer should be a map")
		assert.Equal(t, "nested-value", outer["inner"])
	})

	t.Run("error_propagates_with_key_context", func(t *testing.T) {
		t.Setenv("INTERP_BAD_INT", "notanumber")
		input := map[string]any{"mykey": ".env:int:INTERP_BAD_INT"}
		_, err := interpolateMap(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mykey")
	})
}

func TestFormatValidationErrors(t *testing.T) {
	t.Run("non_validation_error_wrapped", func(t *testing.T) {
		err := formatValidationErrors(fmt.Errorf("something broke"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config validation:")
	})

	t.Run("validation_errors_formatted", func(t *testing.T) {
		type inner struct {
			Name string `validate:"required"`
		}
		v := validator.New()
		valErr := v.Struct(&inner{})
		require.Error(t, valErr)

		err := formatValidationErrors(valErr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config validation failed:")
		assert.Contains(t, err.Error(), "Name")
		assert.Contains(t, err.Error(), "required")
	})
}
