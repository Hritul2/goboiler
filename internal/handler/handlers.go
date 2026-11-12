package handler

import (
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/services"
)

// Handlers aggregates every HTTP handler. Mirrors services.Services's
// aggregation shape.
//
// ADD YOUR DOMAIN: one field per domain handler package (e.g.
// `User *user.Handler`) — see docs/EXAMPLE.md step 7.
type Handlers struct {
	Health *HealthHandler
}

// New constructs every handler. db/redis are Pingers (db.Database and
// *redis.Redis both satisfy this structurally) — passed through from
// internal/services/server, which already holds both via db.Get()/redis.Get().
// svcs supplies the domain services the RPC handlers wrap.
//
// New does not call anything on its arguments, so cmd/gen can pass nil
// Pingers and an empty &services.Services{} to obtain the endpoint list
// (types only) without a database or Redis.
//
// ADD YOUR DOMAIN: build each domain handler here from its service (e.g.
// `User: user.NewHandler(svcs.User, log)`) — see docs/EXAMPLE.md step 7.
func New(db, redis Pinger, svcs *services.Services, log zerolog.Logger) *Handlers {
	_ = svcs // used once you build domain handlers from their services
	return &Handlers{
		Health: NewHealthHandler(db, redis, log),
	}
}
