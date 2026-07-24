package handler

import "github.com/rs/zerolog"

// Handlers aggregates every HTTP handler. Mirrors services.Services's
// aggregation shape.
type Handlers struct {
	Health *HealthHandler
	// User *UserHandler // added when User HTTP endpoints land
}

// New constructs every handler. db/redis are Pingers (db.Database and
// *redis.Redis both satisfy this structurally) — passed through from
// internal/services/server, which already holds both via db.Get()/redis.Get().
func New(db, redis Pinger, log zerolog.Logger) *Handlers {
	return &Handlers{
		Health: NewHealthHandler(db, redis, log),
	}
}
