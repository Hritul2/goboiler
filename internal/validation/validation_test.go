package validation

import (
	"testing"

	zog "github.com/Oudwins/zog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrors_Error(t *testing.T) {
	e := Errors{
		"password": {"is required"},
		"email":    {"must be a valid email address"},
	}
	// Deterministic (sorted-key) join, regardless of map iteration order.
	assert.Equal(t, "email: must be a valid email address; password: is required", e.Error())
}

func TestErrors_HasErrors(t *testing.T) {
	assert.False(t, Errors(nil).HasErrors())
	assert.False(t, Errors{}.HasErrors())
	assert.True(t, Errors{"name": {"is required"}}.HasErrors())
}

var testSchema = zog.Struct(zog.Shape{
	"name": zog.String().Min(2).Required(),
})

func TestValidate(t *testing.T) {
	t.Run("valid_returns_nil", func(t *testing.T) {
		data := struct{ Name string }{Name: "Ada"}
		errs := Validate(testSchema, &data)
		assert.False(t, errs.HasErrors())
	})

	t.Run("invalid_returns_field_errors", func(t *testing.T) {
		data := struct{ Name string }{Name: ""}
		errs := Validate(testSchema, &data)
		require.True(t, errs.HasErrors())
		assert.Contains(t, errs, "name")
	})
}
