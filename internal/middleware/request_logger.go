package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogging emits one access-log line per request after it completes,
// via the per-request logger set by RequestLogger. Info for non-5xx, Error
// for 5xx.
func RequestLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		if full := c.FullPath(); full != "" {
			path = full
		}

		status := c.Writer.Status()
		log := LoggerFromContext(c)

		event := log.Info()
		if status >= http.StatusInternalServerError {
			event = log.Error()
		}

		event.
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Str("request_id", RequestIDFromContext(c)).
			Msg("request handled")
	}
}
