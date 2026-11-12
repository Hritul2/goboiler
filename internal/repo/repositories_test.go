package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The skeleton has no repositories, so NewRepositories needs no database:
// the server must boot (and /ping must work) before any domain is added.
func TestNewRepositories_EmptySkeletonNeedsNoDatabase(t *testing.T) {
	repos, err := NewRepositories(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, repos)
}
