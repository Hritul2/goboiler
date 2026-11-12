package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const loggerKey = "logger"

// RequestLogger builds a per-request logger — carrying request_id and, when
// a valid span is active (via Tracing/otelgin having already run), trace_id
// and span_id for log/trace correlation — and stores it on the gin context.
// log is the boot-time base logger, closed over like
// the job-handler factories' pattern (docs/EXAMPLE.md step 5).
func RequestLogger(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := log.With().Str("request_id", RequestIDFromContext(c))

		if span := trace.SpanFromContext(c.Request.Context()); span.SpanContext().IsValid() {
			ctx = ctx.
				Str("trace_id", span.SpanContext().TraceID().String()).
				Str("span_id", span.SpanContext().SpanID().String())
		}

		c.Set(loggerKey, ctx.Logger())
		c.Next()
	}
}

// LoggerFromContext returns the per-request logger set by RequestLogger, or
// a no-op logger if called before that middleware ran — never panics.
func LoggerFromContext(c *gin.Context) zerolog.Logger {
	v, ok := c.Get(loggerKey)
	if !ok {
		return zerolog.Nop()
	}
	log, ok := v.(zerolog.Logger)
	if !ok {
		return zerolog.Nop()
	}
	return log
}
