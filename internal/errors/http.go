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
}

// ToResponse produces the client-safe response body from an AppError.
func (e *AppError) ToResponse() ErrorResponse {
	return ErrorResponse{Code: e.Code, Message: e.Message}
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

// --- Echo custom error handler (wired in step 17, internal/middleware/global.go) ---
//
// This package stays framework-agnostic. The middleware layer imports it
// (aliased as errs) and registers the handler on the Echo instance:
//
//	e.HTTPErrorHandler = func(err error, c echo.Context) {
//	    if c.Response().Committed {
//	        return
//	    }
//	    appErr := errs.FromError(err)
//	    _ = c.JSON(errs.HTTPStatus(appErr), appErr.ToResponse())
//	}
//
// The Committed guard prevents a double-write when a handler already sent a
// partial response before returning an error.
