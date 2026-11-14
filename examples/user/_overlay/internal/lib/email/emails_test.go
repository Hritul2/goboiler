package email

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	th "github.com/hritul2/goboiler/internal/testing"
)

func TestSendWelcome(t *testing.T) {
	fake := &fakeSender{}

	err := SendWelcome(context.Background(), fake, "ada@example.com", "Ada")
	require.NoError(t, err)

	assert.Equal(t, []string{"ada@example.com"}, fake.last.To)
	assert.Contains(t, fake.last.Subject, "Welcome")
	th.AssertStringContains(t, fake.last.HTML, "Ada")
}
