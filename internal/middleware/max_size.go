package middleware

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	maxReqResSizeKey = "max_req_res_size"
	originalBodyKey  = "original_request_body"
)

// MaxReqResSize is the top-level, global request/response size guard.
//
// Request-side is hard-enforced: the body is wrapped with
// http.MaxBytesReader, so anything that reads past maxReqBytes gets a
// *http.MaxBytesError (mapped to 413 by ErrorHandler). Rejecting is safe
// here since nothing downstream has committed to processing the request
// yet.
//
// Response-side is observability-only: gin.ResponseWriter already tracks
// bytes written via c.Writer.Size(), so after the handler runs this just
// logs a warning if the response exceeded maxResBytes. It does NOT
// truncate — a partially-written JSON response cut off mid-stream would
// corrupt the client's view worse than just serving it large; this is a
// monitoring signal ("this response is unusually big, go look"), not
// enforcement. maxResBytes <= 0 disables the check.
func MaxReqResSize(maxReqBytes, maxResBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(originalBodyKey, c.Request.Body)
		c.Set(maxReqResSizeKey, maxReqBytes)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReqBytes)

		c.Next()

		if maxResBytes > 0 {
			if size := int64(c.Writer.Size()); size > maxResBytes {
				log := LoggerFromContext(c)
				log.Warn().
					Int64("response_bytes", size).
					Int64("limit_bytes", maxResBytes).
					Str("path", c.FullPath()).
					Msg("response exceeded configured size threshold")
			}
		}
	}
}

// MaxReqResSizeOverride replaces the effective request-size limit for a
// specific route or group. Must be registered AFTER MaxReqResSize (i.e. as
// a route/group-level .Use or handler-chain entry — gin always runs global
// middlewares, registered via engine.Use, before any route-specific ones).
//
// It retrieves the ORIGINAL, pre-wrap body stashed by MaxReqResSize and
// re-wraps THAT with the override's limit, discarding the global's wrap
// entirely. This is required, not stylistic: nesting a second
// http.MaxBytesReader on top of an already-wrapped smaller-limited body does
// NOT raise the effective limit — the inner reader still errors first once
// its own (smaller) threshold is crossed, regardless of what a larger outer
// wrap would otherwise allow.
func MaxReqResSizeOverride(maxReqBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := c.Request.Body
		if origAny, ok := c.Get(originalBodyKey); ok {
			if orig, ok := origAny.(io.ReadCloser); ok {
				body = orig
			}
		}

		c.Set(maxReqResSizeKey, maxReqBytes)
		c.Request.Body = http.MaxBytesReader(c.Writer, body, maxReqBytes)

		c.Next()
	}
}
