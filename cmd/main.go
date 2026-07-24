package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/db"
	"github.com/hritul2/goboiler/internal/logger"
	"github.com/hritul2/goboiler/internal/observability"
	"github.com/hritul2/goboiler/internal/redis"
	"github.com/hritul2/goboiler/internal/services"
	"github.com/hritul2/goboiler/internal/services/asyncprocessor"
	"github.com/hritul2/goboiler/internal/services/server"
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
	redis    *redis.Redis
	job      *asyncprocessor.JobService
	srv      *server.Server
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	a.log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
}

// start initialises every dependency in order and returns the running app.
// It always returns a non-nil *app so stop() can safely clean up whatever
// was initialised before an error occurred.
func start(cfg *config.Config) (*app, error) {
	a := &app{cfg: cfg}

	cfg.PrintConfig() // secrets already masked (see loader.go's maskSecret)

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

	// Redis after db per BOOT_SEQUENCE step 5; Init pings with its own 5s
	// deadline — hard fail on an unreachable server.
	r, err := redis.Init(context.Background(), cfg, a.log)
	if err != nil {
		return a, err
	}
	a.redis = r

	// Job service after redis. Which task handlers exist lives in
	// internal/services.NewJobRouter, not here — this just wires the result in.
	jobRouter, err := services.NewJobRouter(cfg, a.log)
	if err != nil {
		return a, err
	}

	// Init starts consuming — asynq.Server.Start is non-blocking, so no
	// wrapper goroutine is needed here.
	j, err := asyncprocessor.Init(context.Background(), cfg, a.log, jobRouter)
	if err != nil {
		return a, err
	}
	a.job = j

	// HTTP server last — it wires repos/services/handlers/router, all of
	// which need db/redis/job already up.
	srv, err := server.New(context.Background(), cfg, a.log)
	if err != nil {
		return a, err
	}
	a.srv = srv

	errCh := srv.Start()
	go func() {
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error().Err(err).Msg("http server failed")
		}
	}()

	return a, nil
}

// stop shuts down every started dependency in reverse order.
// Add teardown calls here as new fields are added to app.
func stop(a *app) {
	timeout := time.Duration(a.cfg.ServerConfig.ShutdownTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if a.srv != nil {
		if err := a.srv.Shutdown(ctx); err != nil {
			a.log.Error().Err(err).Msg("http server shutdown error")
		}
	}

	if a.job != nil {
		if err := a.job.Close(ctx); err != nil {
			a.log.Error().Err(err).Msg("job shutdown error")
		}
	}

	if a.redis != nil {
		if err := a.redis.Close(ctx); err != nil {
			a.log.Error().Err(err).Msg("redis shutdown error")
		}
	}

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
