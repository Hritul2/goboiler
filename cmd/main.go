package main

import (
	"context"
	"log"
	"time"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/logger"
	"github.com/hritul2/goboiler/internal/observability"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// app holds every long-lived dependency started by start().
// Add new fields here as subsequent steps (DB, Redis, server, …) land.
type app struct {
	log zerolog.Logger
	obs *observability.Provider
}

func main() {
	cfg, err := config.LoadConfig(nil)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	a, err := start(cfg)
	defer stop(a)
	if err != nil {
		a.log.Error().Err(err).Msg("startup failed")
		return
	}

	a.log.Info().Msg("application started")
}

// start initialises every dependency in order and returns the running app.
// It always returns a non-nil *app so stop() can safely clean up whatever
// was initialised before an error occurred.
func start(cfg *config.Config) (*app, error) {
	a := &app{}

	a.log = logger.New(&cfg.AppConfig)

	ctx := context.Background()

	obs, err := observability.New(ctx, cfg, a.log)
	if err != nil {
		return a, err
	}
	a.obs = obs

	// Bridge zerolog into the OTel Logs signal.
	a.log = a.log.Hook(obs.ZerologHook())

	return a, nil
}

// stop shuts down every started dependency in reverse order.
// Add teardown calls here as new fields are added to app.
func stop(a *app) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if a.obs != nil {
		if err := a.obs.Shutdown(ctx); err != nil {
			a.log.Error().Err(err).Msg("otel shutdown error")
		}
	}

	a.log.Info().Msg("shutdown complete")
}
