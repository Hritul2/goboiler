// Package router mounts the gin engine: the hand-composed middleware stack,
// GET /ping outside any group, and an empty /api/v1 group.
package router

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/handler"
	"github.com/hritul2/goboiler/internal/middleware"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// New builds the gin engine. Uses gin.New(), not gin.Default() — gin's
// built-in Logger()/Recovery() would double-log every request and
// reintroduce the plaintext-panic-body problem middleware.Recover exists to
// avoid.
func New(cfg *config.Config, log zerolog.Logger, h *handler.Handlers) *gin.Engine {
	gin.SetMode(ginMode(cfg.AppConfig.Env))

	engine := gin.New()
	engine.Use(middleware.New(cfg, log)...)

	engine.GET("/ping", h.Health.Ping)

	v1 := engine.Group("/api/v1")
	_ = v1 // intentionally empty this pass — proves the group mounts and
	// inherits the middleware stack; real routes land with future
	// User CRUD work.

	return engine
}
