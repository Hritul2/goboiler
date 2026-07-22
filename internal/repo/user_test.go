package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/hritul2/goboiler/internal/models"
)

// Compile-time interface compliance.
var _ UserRepository = (*mongoUserRepo)(nil)

func TestUserIndexModels(t *testing.T) {
	idx := userIndexModels()
	require.Len(t, idx, 1, "users collection defines exactly one index")

	keys, ok := idx[0].Keys.(bson.D)
	require.True(t, ok, "Keys must be an order-preserving bson.D")
	require.Len(t, keys, 1)
	assert.Equal(t, "email", keys[0].Key)
	assert.EqualValues(t, 1, keys[0].Value)

	require.NotNil(t, idx[0].Options)
	var opts options.IndexOptions
	for _, set := range idx[0].Options.List() {
		require.NoError(t, set(&opts))
	}
	require.NotNil(t, opts.Unique, "email index must set Unique")
	assert.True(t, *opts.Unique)
}

func TestListFindOptions(t *testing.T) {
	p := models.Pagination{Page: 3, Limit: 10}
	p.Normalize()

	var opts options.FindOptions
	for _, set := range listFindOptions(p).List() {
		require.NoError(t, set(&opts))
	}

	require.NotNil(t, opts.Skip)
	assert.EqualValues(t, 20, *opts.Skip)
	require.NotNil(t, opts.Limit)
	assert.EqualValues(t, 10, *opts.Limit)

	sort, ok := opts.Sort.(bson.D)
	require.True(t, ok, "Sort must be an order-preserving bson.D")
	require.Len(t, sort, 1)
	assert.Equal(t, "created_at", sort[0].Key)
	assert.EqualValues(t, -1, sort[0].Value, "newest first")
}
