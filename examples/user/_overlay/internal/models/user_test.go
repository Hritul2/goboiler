package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	th "github.com/hritul2/goboiler/internal/testing"
)

func TestNewUser(t *testing.T) {
	u := NewUser("ada@example.com", "Ada", "hash123")

	assert.True(t, u.ID.IsZero(), "ID is assigned by the repository on Create")
	assert.Equal(t, "ada@example.com", u.Email)
	assert.Equal(t, "Ada", u.Name)
	assert.Equal(t, "hash123", u.PasswordHash)
	assert.False(t, u.CreatedAt.IsZero())
}

func TestUser_JSONHidesPasswordHash(t *testing.T) {
	u := NewUser("ada@example.com", "Ada", "supersecrethash")
	u.ID = "u_123"

	out, err := json.Marshal(u)
	require.NoError(t, err)

	s := string(out)
	th.AssertStringContains(t, s, `"id":"u_123"`, `"email":"ada@example.com"`, `"name":"Ada"`)
	assert.NotContains(t, s, "password_hash")
	assert.NotContains(t, s, "supersecrethash")
}
