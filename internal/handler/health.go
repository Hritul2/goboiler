package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// Pinger is the narrow interface HealthHandler needs — satisfied
// structurally by db.Database and *redis.Redis, both already exposing
// Ping(ctx) error. Constructor injection (not global db.Get()/redis.Get()
// calls) keeps this handler unit-testable with fakes.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler serves liveness/readiness endpoints.
type HealthHandler struct {
	db    Pinger
	redis Pinger
	log   zerolog.Logger
}

func NewHealthHandler(db, redis Pinger, log zerolog.Logger) *HealthHandler {
	return &HealthHandler{db: db, redis: redis, log: log}
}

// Ping checks real dependency connectivity (db, redis) with a short timeout
// each. No job-queue check: asynq has no Ping equivalent, and the queue
// rides on the same Redis connection already being checked here, so a
// separate check would be redundant.
func (h *HealthHandler) Ping(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	checks := gin.H{}
	healthy := true

	if err := h.db.Ping(ctx); err != nil {
		checks["db"] = "down"
		healthy = false
		h.log.Error().Err(err).Msg("health check: db down")
	} else {
		checks["db"] = "ok"
	}

	if err := h.redis.Ping(ctx); err != nil {
		checks["redis"] = "down"
		healthy = false
		h.log.Error().Err(err).Msg("health check: redis down")
	} else {
		checks["redis"] = "ok"
	}

	status := http.StatusOK
	statusText := "ok"
	if !healthy {
		status = http.StatusServiceUnavailable
		statusText = "degraded"
	}

	c.JSON(status, gin.H{"status": statusText, "checks": checks})
}
