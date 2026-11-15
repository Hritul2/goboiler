package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	th "github.com/hritul2/goboiler/internal/testing"
)

func TestRenderTemplate_Welcome(t *testing.T) {
	out, err := renderTemplate("welcome.html", welcomeData{Name: "Ada"})
	require.NoError(t, err)
	th.AssertStringContains(t, out, "Ada", "Welcome")
}

func TestRenderTemplate_EscapesHTML(t *testing.T) {
	out, err := renderTemplate("welcome.html", welcomeData{Name: "<script>alert(1)</script>"})
	require.NoError(t, err)
	assert.NotContains(t, out, "<script>", "html/template must escape user input")
	assert.Contains(t, out, "&lt;script&gt;")
}

func TestRenderTemplate_UnknownName(t *testing.T) {
	_, err := renderTemplate("nope.html", nil)
	require.Error(t, err)
}
