// Package router mounts the gin engine: the hand-composed middleware stack
// and three independent route families — GET /ping, one gin group per RPC
// API version (POST /api/<ver>/rpc/<function>), and a /webhook group for
// third-party callbacks.
package router

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/handler"
	"github.com/hritul2/goboiler/internal/middleware"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// New builds the gin engine. Uses gin.New(), not gin.Default() — gin's
// built-in Logger()/Recovery() would double-log every request and
// reintroduce the plaintext-panic-body problem middleware.Recover exists to
// avoid.
func New(cfg *config.Config, log zerolog.Logger, h *handler.Handlers) *gin.Engine {
	gin.SetMode(ginMode(cfg.App.Env))

	engine := gin.New()
	engine.Use(middleware.New(cfg, log)...)

	// Family 1: liveness/readiness. Plain gin, unversioned, not generated.
	engine.GET("/ping", h.Health.Ping)

	// Family 2: typed RPC. Every version in APIVersions is mounted as its
	// own independent gin group (and generated to its own TS folder). Adding
	// /api/v2 is a new entry in versions.go — nothing here changes.
	for _, v := range APIVersions(h) {
		api.Mount(engine, v)
	}

	// Family 3: webhooks. Intentionally empty for now, and intentionally
	// separate from the RPC versions: webhook routes do not go through
	// internal/api because they need the RAW request body to verify the
	// provider's signature, are called by the provider (not our frontend),
	// and are never part of the generated client.
	//
	// Pattern for adding one (e.g. Clerk, Dodo Payments): create
	// internal/webhook/<provider>/ with a plain gin.HandlerFunc that reads
	// c.GetRawData(), verifies the signature header against the provider's
	// secret, then decodes and handles the event; register it on this group
	// with webhooks.POST("/<provider>", ...) — optionally behind
	// middleware.MaxReqResSizeOverride for providers that send big payloads.
	// See docs/API_HANDLERS.md ("Webhooks").
	//
	// ADD YOUR DOMAIN: register provider webhooks on this group (e.g.
	// `webhooks := engine.Group("/webhook")`, then
	// `webhooks.POST("/clerk", h.Clerk.Handle)`) — see docs/EXAMPLE.md step 10.
	_ = engine.Group("/webhook")

	return engine
}
