package testing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Ptr returns a pointer to v. Useful for optional struct fields in test fixtures.
func Ptr[T any](v T) *T { return &v }

// ProjectRoot walks up from cwd until it finds go.mod and returns that directory.
// Fails the test if no go.mod is found.
func ProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod found)")
			return ""
		}
		dir = parent
	}
}

// MustMarshalJSON marshals v to JSON or fails the test.
func MustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err, "failed to marshal to JSON")
	return b
}
