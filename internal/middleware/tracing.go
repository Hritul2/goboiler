package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// Tracing attaches automatic per-request OTel spans. serviceName should
// match cfg.AppConfig.Name (the same field internal/observability's
// buildResource uses), so span resource attributes and trace service name
// agree.
func Tracing(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}
