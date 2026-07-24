// Package validation is the application's single validation layer, built on
// github.com/go-playground/validator/v10. Callers tag their input structs
// with `validate:"..."` rules and pass a pointer to Validate; failures are
// returned as an Errors map keyed by the lowercased Go struct field name
// (e.g. "email", not "Email") so the map-key convention matches existing
// callers/tests.
package validation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate is the package-wide validator instance, built once (reflection
// caching makes construction relatively expensive) and safe for concurrent use.
var validate = validator.New()

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

// Validate runs go-playground/validator's `validate:"..."` struct-tag rules
// against dataPtr (a pointer to a tagged struct) and returns the per-field
// errors, or a nil Errors (HasErrors() == false) when validation passes.
func Validate(dataPtr any) Errors {
	err := validate.Struct(dataPtr)
	if err == nil {
		return nil
	}

	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		return Errors{"_error": {err.Error()}}
	}

	errs := make(Errors, len(ve))
	for _, fe := range ve {
		key := strings.ToLower(fe.Field())
		errs[key] = append(errs[key], tagMessage(fe))
	}
	return errs
}

// tagMessage renders a human-readable message for the tags this application
// currently uses, falling back to a generic message for any other tag.
func tagMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return fmt.Sprintf("must be at least %s characters", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s characters", fe.Param())
	default:
		return fmt.Sprintf("failed '%s' validation", fe.Tag())
	}
}
