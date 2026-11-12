package api

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/validation"
)

// adapt turns a typed function into a gin.HandlerFunc. It is the ONLY place
// in the package that touches *gin.Context, so the typed function stays a
// plain Go function that is trivial to unit-test without HTTP.
//
// Request flow:
//  1. decode the JSON body into Req (empty body == {}),
//  2. validation.Validate(&req) against the `validate` struct tags,
//  3. call fn with the request context,
//  4. success => 200 + JSON(res); any failure => c.Error(err) and return.
//     Validation failures are errs.Validation: 400 BAD_REQUEST with a
//     `fields` map (JSON field path -> messages) for form UIs.
//
// The adapter never writes an error response itself. It follows the
// repo-wide convention (see middleware.ErrorHandler): handlers accumulate
// the error with c.Error and the terminal middleware renders the uniform
// {code,message} body with the right status. Writing here too would risk a
// double write and a second, different error shape.
func adapt[Req, Res any](fn func(context.Context, Req) (Res, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req Req
		if err := decodeBody(c.Request.Body, &req); err != nil {
			_ = c.Error(err)
			return
		}

		// Same validation entry point and error mapping the services use
		// for their own inputs, so a field error reads identically whichever
		// layer catches it first. errs.Validation puts the per-field
		// messages (keyed by JSON field path) in the response's `fields`.
		if verrs := validation.Validate(&req); verrs.HasErrors() {
			_ = c.Error(errs.Validation(verrs))
			return
		}

		res, err := fn(c.Request.Context(), req)
		if err != nil {
			_ = c.Error(err)
			return
		}

		c.JSON(http.StatusOK, res)
	}
}

// decodeBody reads one JSON value from body into dst.
//
//   - Empty (or whitespace-only) body is treated as {} — dst is left at its
//     zero value. This is what lets api.Empty functions be called with no
//     body, and lets optional-only requests omit it.
//   - A *http.MaxBytesError (body exceeded middleware.MaxReqResSize) is
//     returned UNWRAPPED and unmodified so middleware.ErrorHandler's
//     errors.As check still maps it to 413. Converting it to BadRequest here
//     would silently turn "too large" into "malformed".
//   - Everything else that fails is the client's fault => errs.BadRequest
//     with a message that never leaks Go type or package names.
//
// Unknown JSON fields are ignored (lenient decoding): a newer client talking
// to an older server, or vice versa, should degrade rather than 400.
func decodeBody(body io.Reader, dst any) error {
	dec := json.NewDecoder(body)

	if err := dec.Decode(dst); err != nil {
		if stderrors.Is(err, io.EOF) {
			return nil // empty body => {}
		}
		return classifyDecodeError(err)
	}

	// Exactly one JSON value is allowed. Anything after it ("{} {}", "{}x")
	// is a malformed body rather than something to silently drop.
	if _, err := dec.Token(); !stderrors.Is(err, io.EOF) {
		if err == nil {
			return badBody(nil, "request body must contain a single JSON object")
		}
		return classifyDecodeError(err)
	}
	return nil
}

// classifyDecodeError maps a decoder error to the right response error.
func classifyDecodeError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if stderrors.As(err, &maxBytesErr) {
		return err // pass through untouched: ErrorHandler => 413
	}

	var typeErr *json.UnmarshalTypeError
	if stderrors.As(err, &typeErr) {
		if typeErr.Field != "" {
			return badBody(err, fmt.Sprintf("field %q has the wrong type: got %s", typeErr.Field, typeErr.Value))
		}
		return badBody(err, "request body must be a JSON object")
	}

	return badBody(err, "request body is not valid JSON")
}

// badBody builds the 400, keeping the decoder error as Internal so it shows
// up in logs (AppError.Error) but is never serialised to the client.
func badBody(cause error, msg string) *errs.AppError {
	appErr := errs.BadRequest(msg)
	appErr.Internal = cause
	return appErr
}
