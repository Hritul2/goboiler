package main

import (
	"context"
	"log"
	"time"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/db"
	"github.com/hritul2/goboiler/internal/logger"
	"github.com/hritul2/goboiler/internal/observability"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// app holds every long-lived dependency started by start().
// Add new fields here as subsequent steps (DB, Redis, server, …) land.
type app struct {
	cfg      *config.Config
	log      zerolog.Logger
	logClose func() error
	obs      *observability.Provider
	db       db.Database
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
	a := &app{cfg: cfg}

	a.log, a.logClose = logger.New(&cfg.AppConfig)

	// 5-second deadline prevents a hung OTLP dial from blocking startup.
	initCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obs, err := observability.New(initCtx, cfg, a.log)
	if err != nil {
		return a, err
	}
	a.obs = obs

	// Bridge zerolog into the OTel Logs signal.
	a.log = a.log.Hook(obs.ZerologHook())

	// DB init comes after observability so the otelmongo command monitor
	// picks up the global tracer provider. Init pings with its own 5s
	// deadline — hard fail on an unreachable deployment.
	database, err := db.Init(context.Background(), cfg, a.log)
	if err != nil {
		return a, err
	}
	a.db = database

	return a, nil
}

// stop shuts down every started dependency in reverse order.
// Add teardown calls here as new fields are added to app.
func stop(a *app) {
	timeout := time.Duration(a.cfg.ServerConfig.ShutdownTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if a.db != nil {
		if err := a.db.Close(ctx); err != nil {
			a.log.Error().Err(err).Msg("db shutdown error")
		}
	}

	if a.obs != nil {
		if err := a.obs.Shutdown(ctx); err != nil {
			a.log.Error().Err(err).Msg("otel shutdown error")
		}
	}

	a.log.Info().Msg("shutdown complete")

	// Close log file last so the shutdown log line is flushed first.
	if a.logClose != nil {
		_ = a.logClose()
	}
}
