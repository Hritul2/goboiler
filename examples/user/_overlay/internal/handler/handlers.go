package handler

import (
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/handler/user"
	"github.com/hritul2/goboiler/internal/services"
)

// Handlers aggregates every HTTP handler. Mirrors services.Services's
// aggregation shape.
type Handlers struct {
	Health *HealthHandler
	User   *user.Handler
}

// New constructs every handler. db/redis are Pingers (db.Database and
// *redis.Redis both satisfy this structurally) — passed through from
// internal/services/server, which already holds both via db.Get()/redis.Get().
// svcs supplies the domain services the RPC handlers wrap.
//
// New does not call anything on its arguments, so cmd/gen can pass nil
// Pingers and an empty &services.Services{} to obtain the endpoint list
// (types only) without a database or Redis.
func New(db, redis Pinger, svcs *services.Services, log zerolog.Logger) *Handlers {
	return &Handlers{
		Health: NewHealthHandler(db, redis, log),
		User:   user.NewHandler(svcs.User, log),
	}
}
