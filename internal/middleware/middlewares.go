package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// New returns the full middleware stack in mandated mount order (outer to
// inner): CORS -> body size limit -> rate limit -> secure headers ->
// request ID -> tracing -> per-request logger injection -> access-log line
// -> recover -> error handler.
//
// Size limit and rate limit are mounted early, right after CORS, so
// oversized/abusive requests are rejected as cheaply as possible, before any
// real work (including the security-header/request-id bookkeeping) happens.
//
// Auth is deliberately absent — no auth story exists yet in this codebase.
//
// HTTP-layer metrics are deliberately NOT added here: otelgin (wired via
// Tracing below) already records request duration/response size per route
// automatically — a custom counter here would double-count the same signal.
func New(cfg *config.Config, log zerolog.Logger) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		CORS(CORSSettings{
			AllowedOrigins:   cfg.Server.CORSAllowedOrigins,
			AllowedMethods:   cfg.Server.CORSAllowedMethods,
			AllowedHeaders:   cfg.Server.CORSAllowedHeaders,
			AllowCredentials: cfg.Server.CORSAllowCredentials,
			MaxAgeSeconds:    cfg.Server.CORSMaxAgeSeconds,
		}),
		MaxReqResSize(cfg.Server.MaxRequestBodyBytes, cfg.Server.MaxResponseBodyBytes),
		RateLimit(cfg.Server.RateLimitPerMinute, cfg.Server.RateLimitBurst),
		SecureHeaders(),
		RequestID(),
		Tracing(cfg.App.Name),
		RequestLogger(log),
		RequestLogging(),
		Recover(log),
		ErrorHandler(),
	}
}
