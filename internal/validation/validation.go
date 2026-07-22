// Package validation is the application's Zod-equivalent schema validator,
// built on github.com/Oudwins/zog. Shape keys use lowercase-first field
// names (e.g. "email", not "Email") — zog upper-cases only the first byte
// before binding to the Go struct field via reflection, but preserves the
// original key for the issue path, so lowercase keys read as friendlier
// error map keys ("email: ..." not "Email: ...").
package validation

import (
	"sort"
	"strings"

	zog "github.com/Oudwins/zog"
)

// Errors maps a field name to its validation messages.
type Errors map[string][]string

// Error joins every field's messages into a single deterministic string,
// sorted by field name so output doesn't depend on map iteration order.
func (e Errors) Error() string {
	fields := make([]string, 0, len(e))
	for f := range e {
		fields = append(fields, f)
	}
	sort.Strings(fields)

	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, f+": "+strings.Join(e[f], ", "))
	}
	return strings.Join(parts, "; ")
}

// HasErrors reports whether any field failed validation.
func (e Errors) HasErrors() bool {
	return len(e) > 0
}

// Validate runs schema against dataPtr and returns the per-field errors, or
// a nil Errors (HasErrors() == false) when validation passes.
func Validate(schema *zog.StructSchema, dataPtr any) Errors {
	issues := schema.Validate(dataPtr)
	if len(issues) == 0 {
		return nil
	}
	return zog.Issues.Flatten(issues)
}
