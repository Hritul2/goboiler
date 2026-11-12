package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// allowedOriginSet is intentionally minimal — a static allow-list check, not
// a spec-complete CORS implementation (no Vary: Origin, no wildcard
// subdomain matching). cfg.Server.CORSAllowedOrigins is a small,
// validated, static list; a full CORS library would be overkill for it. If
// wildcard/subdomain matching is ever needed, reconsider this tradeoff
// rather than bolting it on here.
type allowedOriginSet map[string]struct{}

func newAllowedOriginSet(origins []string) allowedOriginSet {
	set := make(allowedOriginSet, len(origins))
	for _, o := range origins {
		set[o] = struct{}{}
	}
	return set
}

func (s allowedOriginSet) Allowed(origin string) bool {
	if origin == "" {
		return false
	}
	_, ok := s[origin]
	return ok
}

// CORSSettings is entirely config-driven — see
// internal/utils/config.ServerConfig's CORS* fields.
type CORSSettings struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
	MaxAgeSeconds    int
}

// CORS allows only the configured origins, echoing the configured
// methods/headers/credentials/max-age — all previously hardcoded literals,
// now sourced from config.
func CORS(cfg CORSSettings) gin.HandlerFunc {
	set := newAllowedOriginSet(cfg.AllowedOrigins)
	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAgeSeconds)

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if set.Allowed(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", methods)
			c.Header("Access-Control-Allow-Headers", headers)
			c.Header("Access-Control-Max-Age", maxAge)
			if cfg.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
