package middleware

import "github.com/gin-gonic/gin"

// secureHeaders is the static header set applied to every response.
func secureHeaders() map[string]string {
	return map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-XSS-Protection":       "0",
	}
}

// SecureHeaders sets a minimal set of standard security headers on every response.
func SecureHeaders() gin.HandlerFunc {
	headers := secureHeaders()
	return func(c *gin.Context) {
		for k, v := range headers {
			c.Header(k, v)
		}
		c.Next()
	}
}
