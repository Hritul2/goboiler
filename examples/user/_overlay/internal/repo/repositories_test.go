package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Replaces the skeleton's test of the same name. With the User repository
// wired in, NewRepositories needs the Mongo handle, so it is only valid after
// db.Init; before that it fails loudly instead of returning a half-built
// aggregate. (Repository behaviour that needs a live Mongo is verified with
// the compose stack, not here.)
func TestNewRepositories_WithUserNeedsDatabase(t *testing.T) {
	assert.Panics(t, func() {
		_, _ = NewRepositories(context.Background())
	})
}
