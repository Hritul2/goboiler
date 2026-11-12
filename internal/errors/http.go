package errors

import (
	stderrors "errors"
	"net/http"
)

// ErrorResponse is the JSON body sent to API clients for every error.
// The Internal field of AppError is intentionally excluded.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`

	// Fields is present only for validation failures (see Validation): JSON
	// field path -> messages, so a frontend can attach each message to the
	// matching form input.
	Fields map[string][]string `json:"fields,omitempty"`
}

// ToResponse produces the client-safe response body from an AppError.
func (e *AppError) ToResponse() ErrorResponse {
	return ErrorResponse{Code: e.Code, Message: e.Message, Fields: e.Fields}
}

// FromError normalises any error value into an *AppError.
// If the chain contains an *AppError (even wrapped with fmt.Errorf), it is
// returned as-is. Unknown errors are wrapped as Internal so no internal details
// leak to clients.
func FromError(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr
	}
	return Internal(err)
}

// HTTPStatus extracts the HTTP status code from any error.
// Returns 200 for nil, 500 for unrecognised errors.
func HTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr.StatusCode
	}
	return http.StatusInternalServerError
}

// --- gin error handling (wired in step 17, internal/middleware/{recover,error_handler}.go) ---
//
// This package stays framework-agnostic. The middleware layer imports it
// (aliased as errs) in two places:
//
//   - error_handler.go: gin's idiomatic c.Errors accumulation pattern —
//     after c.Next(), if a handler called c.Error(err) and nothing has
//     written a response yet, convert the last accumulated error:
//
//	appErr := errs.FromError(c.Errors.Last().Err)
//	c.JSON(errs.HTTPStatus(appErr), appErr.ToResponse())
//
//   - recover.go: a panic must produce the same shape, not gin's default
//     plaintext body:
//
//	appErr := errs.Internal(fmt.Errorf("panic: %v", r))
//	c.AbortWithStatusJSON(errs.HTTPStatus(appErr), appErr.ToResponse())
//
// The `!c.Writer.Written()` guard in error_handler.go prevents a double-write
// when a handler already sent a partial response before returning an error —
// gin's equivalent of Echo's Response().Committed check.
