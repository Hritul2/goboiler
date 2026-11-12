// Package validation is the application's single validation layer, built on
// github.com/go-playground/validator/v10. Callers tag their input structs
// with `validate:"..."` rules and pass a pointer to Validate; failures are
// returned as an Errors map keyed by the field's JSON path, i.e. the same
// names a client sent in the request body:
//
//   - the `json` tag name when present ("userId"),
//   - otherwise (no tag, or `json:"-"`) the lowerCamel Go field name
//     ("Email" -> "email", "UserID" -> "userID", "ID" -> "id"),
//   - nested structs join with dots ("address.city"); slice and map
//     elements keep their index/key ("items[0].name").
//
// That keying is what lets a frontend attach each message to the matching
// form input. errs.Validation (internal/errors) turns an Errors value into
// the 400 response carrying it as `fields`.
package validation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

// validate is the package-wide validator instance, built once (reflection
// caching makes construction relatively expensive) and safe for concurrent use.
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()
	// Make fe.Field()/fe.Namespace() use the JSON name instead of the Go name.
	v.RegisterTagNameFunc(jsonFieldName)
	return v
}

// jsonFieldName is the validator tag-name hook: the name a field has on the
// wire, falling back to lowerCamel(GoName) when the json tag is absent or
// "-". It never returns "" so the validator always uses it.
func jsonFieldName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" || name == "-" {
		return lowerCamel(f.Name)
	}
	return name
}

// lowerCamel lower-cases the leading capital run of a Go identifier, keeping
// the last capital of the run when a lowercase letter follows it, so
// initialisms collapse naturally: "Email" -> "email", "UserID" -> "userID",
// "ID" -> "id", "URLPath" -> "urlPath".
func lowerCamel(s string) string {
	r := []rune(s)
	n := 0
	for n < len(r) && unicode.IsUpper(r[n]) {
		n++
	}
	switch {
	case n == 0:
		return s
	case n == len(r):
		n = len(r) // all capitals: "ID" -> "id"
	case n > 1:
		n-- // "URLPath": keep 'P' as the start of the next word
	}
	for i := 0; i < n; i++ {
		r[i] = unicode.ToLower(r[i])
	}
	return string(r)
}

// Errors maps a JSON field path to its validation messages.
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
		key := fieldPath(fe)
		errs[key] = append(errs[key], tagMessage(fe))
	}
	return errs
}

// fieldPath returns the dotted JSON path of the failing field. The
// validator's namespace is "<StructType>.<jsonPath>" (custom tag names are
// already applied by jsonFieldName); the leading struct-type segment is
// dropped because it is not part of the request body.
func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if _, path, ok := strings.Cut(ns, "."); ok && path != "" {
		return path
	}
	return fe.Field()
}

// tagMessage renders a human-readable message for the tags this application
// uses, falling back to a generic message for any other tag. min/max/len and
// friends read differently per kind: a string is measured in characters, a
// slice/map in items, a number by value.
func tagMessage(fe validator.FieldError) string {
	p := fe.Param()
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "url", "http_url", "uri":
		return "must be a valid URL"
	case "uuid", "uuid4", "uuid5":
		return "must be a valid UUID"
	case "oneof":
		return "must be one of: " + strings.Join(strings.Fields(p), ", ")
	case "min", "gte":
		return "must be at least " + measure(fe.Kind(), p)
	case "max", "lte":
		return "must be at most " + measure(fe.Kind(), p)
	case "gt":
		return "must be greater than " + measure(fe.Kind(), p)
	case "lt":
		return "must be less than " + measure(fe.Kind(), p)
	case "len":
		if isNumber(fe.Kind()) {
			return "must equal " + p
		}
		return "must be exactly " + measure(fe.Kind(), p)
	default:
		return fmt.Sprintf("failed '%s' validation", fe.Tag())
	}
}

// measure formats a limit with the unit that fits the field's kind:
// "8 characters" (string), "2 items" (slice/array/map), "18" (number).
func measure(kind reflect.Kind, param string) string {
	switch kind {
	case reflect.String:
		return pluralize(param, "character")
	case reflect.Slice, reflect.Array, reflect.Map:
		return pluralize(param, "item")
	default:
		return param
	}
}

func pluralize(n, unit string) string {
	if n == "1" {
		return n + " " + unit
	}
	return n + " " + unit + "s"
}

func isNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}
