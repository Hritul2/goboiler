package email

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

// html/template (not text/template) so user-supplied values are escaped.
var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// welcomeData feeds welcome.html.
type welcomeData struct {
	Name string
}

// renderTemplate executes the named embedded template with data.
func renderTemplate(name string, data any) (string, error) {
	tpl := templates.Lookup(name)
	if tpl == nil {
		return "", fmt.Errorf("email: unknown template %q", name)
	}

	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("email: render %q: %w", name, err)
	}
	return b.String(), nil
}
