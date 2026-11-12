// Package errors defines the application's root domain error type (AppError)
// and all error construction, classification, and HTTP-mapping utilities.
//
// Because this package's name collides with stdlib "errors", callers must
// import it with an alias:
//
//	import errs "github.com/hritul2/goboiler/internal/errors"
//
// Within this package, stdlib errors is imported as:
//
//	import stderrors "errors"
package errors

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const (
	CodeNotFound           = "NOT_FOUND"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeBadRequest         = "BAD_REQUEST"
	CodeConflict           = "CONFLICT"
	CodeInternal           = "INTERNAL"
	CodeUnprocessable      = "UNPROCESSABLE"
	CodeTooManyRequests    = "TOO_MANY_REQUESTS"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodePayloadTooLarge    = "PAYLOAD_TOO_LARGE"
)

// AppError is the application's unified error type. All layers return *AppError
// so that HTTP handlers can always extract a status code and a safe client message.
type AppError struct {
	Code       string // machine-readable; one of the Code* constants
	Message    string // human-readable; safe to send to clients
	StatusCode int    // HTTP status code
	Internal   error  // root cause; never serialised to API responses

	// Fields carries per-field validation messages keyed by the request's
	// JSON field path (e.g. "email", "address.city"). Only set by
	// Validation; nil for every other error. Serialised as `fields`.
	Fields map[string][]string
}

func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Internal.Error())
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap exposes the internal error so errors.As/Is can traverse the chain.
func (e *AppError) Unwrap() error {
	return e.Internal
}

// Is reports whether two *AppError values are equivalent by Code.
// This makes errors.Is(someErr, errs.ErrRecordNotFound) work even when someErr
// is wrapped with fmt.Errorf.
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

func NotFound(message string) *AppError {
	return &AppError{Code: CodeNotFound, Message: message, StatusCode: http.StatusNotFound}
}

func Unauthorized(message string) *AppError {
	return &AppError{Code: CodeUnauthorized, Message: message, StatusCode: http.StatusUnauthorized}
}

func Forbidden(message string) *AppError {
	return &AppError{Code: CodeForbidden, Message: message, StatusCode: http.StatusForbidden}
}

func BadRequest(message string) *AppError {
	return &AppError{Code: CodeBadRequest, Message: message, StatusCode: http.StatusBadRequest}
}

func Conflict(message string) *AppError {
	return &AppError{Code: CodeConflict, Message: message, StatusCode: http.StatusConflict}
}

// Validation builds the 400 BAD_REQUEST for a failed input validation. fields
// maps a JSON field path to its messages (validation.Errors satisfies this
// directly — this package takes a plain map so it never imports validation).
// Message is a deterministic one-line summary ("validation failed: email:
// ...; name: ..."); Fields is the structured form a frontend maps onto form
// inputs. The map is copied, so later mutation by the caller cannot change
// the error.
func Validation(fields map[string][]string) *AppError {
	copied := make(map[string][]string, len(fields))
	names := make([]string, 0, len(fields))
	for name, msgs := range fields {
		copied[name] = append([]string(nil), msgs...)
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+": "+strings.Join(copied[name], ", "))
	}

	msg := "validation failed"
	if len(parts) > 0 {
		msg += ": " + strings.Join(parts, "; ")
	}
	return &AppError{
		Code:       CodeBadRequest,
		Message:    msg,
		StatusCode: http.StatusBadRequest,
		Fields:     copied,
	}
}

// Internal wraps an infrastructure error as a safe 500. The root cause is stored
// in Internal and never sent to clients.
func Internal(err error) *AppError {
	return &AppError{
		Code:       CodeInternal,
		Message:    "an internal server error occurred",
		StatusCode: http.StatusInternalServerError,
		Internal:   err,
	}
}

func Unprocessable(message string) *AppError {
	return &AppError{Code: CodeUnprocessable, Message: message, StatusCode: http.StatusUnprocessableEntity}
}

func TooManyRequests(message string) *AppError {
	return &AppError{Code: CodeTooManyRequests, Message: message, StatusCode: http.StatusTooManyRequests}
}

func ServiceUnavailable(message string) *AppError {
	return &AppError{Code: CodeServiceUnavailable, Message: message, StatusCode: http.StatusServiceUnavailable}
}

func PayloadTooLarge(message string) *AppError {
	return &AppError{Code: CodePayloadTooLarge, Message: message, StatusCode: http.StatusRequestEntityTooLarge}
}
