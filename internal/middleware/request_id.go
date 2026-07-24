package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is the header used to propagate/return the request ID.
const RequestIDHeader = "X-Request-Id"

const requestIDKey = "request_id"

// RequestID reuses an inbound X-Request-Id (e.g. from an upstream load
// balancer) if present, otherwise generates one. Always echoes it back on
// the response header and stores it on the gin context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(requestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// RequestIDFromContext returns the request ID set by RequestID, or "" if
// called before that middleware ran.
func RequestIDFromContext(c *gin.Context) string {
	v, ok := c.Get(requestIDKey)
	if !ok {
		return ""
	}
	id, _ := v.(string)
	return id
}
