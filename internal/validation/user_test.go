package validation

import "testing"

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserCreateSchema(t *testing.T) {
	tests := []struct {
		name      string
		in        UserCreateInput
		wantField string // empty means no error expected
	}{
		{"valid", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: "password123"}, ""},
		{"bad_email", UserCreateInput{Email: "not-an-email", Name: "Ada", Password: "password123"}, "email"},
		{"missing_email", UserCreateInput{Email: "", Name: "Ada", Password: "password123"}, "email"},
		{"name_too_short", UserCreateInput{Email: "ada@example.com", Name: "A", Password: "password123"}, "name"},
		{"missing_name", UserCreateInput{Email: "ada@example.com", Name: "", Password: "password123"}, "name"},
		{"password_too_short", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: "short"}, "password"},
		{"password_too_long", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: stringOfLen(73)}, "password"},
		{"password_max_ok", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: stringOfLen(72)}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(UserCreateSchema, &tt.in)
			if tt.wantField == "" {
				assert.False(t, errs.HasErrors(), "expected no errors, got %v", errs)
				return
			}
			require.True(t, errs.HasErrors())
			assert.Contains(t, errs, tt.wantField)
		})
	}
}

func TestUserUpdateSchema(t *testing.T) {
	tests := []struct {
		name      string
		in        UserUpdateInput
		wantField string
	}{
		{"valid", UserUpdateInput{Name: "Ada"}, ""},
		{"too_short", UserUpdateInput{Name: "A"}, "name"},
		{"missing", UserUpdateInput{Name: ""}, "name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(UserUpdateSchema, &tt.in)
			if tt.wantField == "" {
				assert.False(t, errs.HasErrors())
				return
			}
			require.True(t, errs.HasErrors())
			assert.Contains(t, errs, tt.wantField)
		})
	}
}

func stringOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
