package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	th "github.com/hritul2/goboiler/internal/testing"
)

func TestNewUser(t *testing.T) {
	u := NewUser("ada@example.com", "Ada", "hash123")

	require.False(t, u.ID.IsZero(), "Base must be populated")
	assert.Equal(t, "ada@example.com", u.Email)
	assert.Equal(t, "Ada", u.Name)
	assert.Equal(t, "hash123", u.PasswordHash)
	assert.False(t, u.CreatedAt.IsZero())
}

// TestUser_BSONInline proves Base flattens into the document root — this is
// the test that catches a forgotten `bson:",inline"` tag.
func TestUser_BSONInline(t *testing.T) {
	u := NewUser("ada@example.com", "Ada", "hash123")

	raw, err := bson.Marshal(u)
	require.NoError(t, err)

	var doc bson.M
	require.NoError(t, bson.Unmarshal(raw, &doc))

	for _, key := range []string{"_id", "created_at", "updated_at", "email", "name", "password_hash"} {
		assert.Contains(t, doc, key)
	}
	assert.NotContains(t, doc, "base", "Base must be inlined, not nested")
}

func TestUser_JSONHidesPasswordHash(t *testing.T) {
	u := NewUser("ada@example.com", "Ada", "supersecrethash")

	out, err := json.Marshal(u)
	require.NoError(t, err)

	s := string(out)
	th.AssertStringContains(t, s, `"id":"`, `"email":"ada@example.com"`, `"name":"Ada"`)
	assert.NotContains(t, s, "password_hash")
	assert.NotContains(t, s, "supersecrethash")
}
