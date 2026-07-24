package middleware

import (
	stderrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"

	errs "github.com/hritul2/goboiler/internal/errors"
)

// ErrorHandler is gin's idiomatic error-accumulation pattern: handlers call
// c.Error(err) instead of writing a response themselves, and this
// terminal middleware converts the last accumulated error into the uniform
// JSON {code,message} shape via errs.FromError/HTTPStatus.
//
// *http.MaxBytesError (from a body wrapped by MaxReqResSize/
// MaxReqResSizeOverride) is special-cased to 413 — without this, any
// handler that reads the body and forwards the resulting error via c.Error
// would fall through to errs.FromError's generic Internal(err) → 500, which
// is the wrong status for "the client sent too much data."
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err

		var maxBytesErr *http.MaxBytesError
		if stderrors.As(err, &maxBytesErr) {
			appErr := errs.PayloadTooLarge("request body too large")
			c.JSON(errs.HTTPStatus(appErr), appErr.ToResponse())
			return
		}

		appErr := errs.FromError(err)
		c.JSON(errs.HTTPStatus(appErr), appErr.ToResponse())
	}
}
