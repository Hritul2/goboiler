// Package server is the central wiring service: builds repos, domain
// services, handlers, and the router, then owns the HTTP server's
// lifecycle. It does not own db/redis/job — those stay owned by
// cmd/main.go's existing singletons and teardown order.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/db"
	"github.com/hritul2/goboiler/internal/handler"
	"github.com/hritul2/goboiler/internal/redis"
	"github.com/hritul2/goboiler/internal/repo"
	"github.com/hritul2/goboiler/internal/router"
	"github.com/hritul2/goboiler/internal/services"
	"github.com/hritul2/goboiler/internal/services/asyncprocessor"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// Server holds only what Shutdown needs — repos/services/handlers/router
// are local to New's construction, not stored, since nothing outside
// construction needs to reach them once the engine is built.
type Server struct {
	httpServer *http.Server
	log        zerolog.Logger
}

// New wires repos -> services -> handlers -> router -> http.Server. It does
// not start listening — call Start() once New succeeds. Split from
// asyncprocessor.Init's do-everything-in-one-call precedent because
// http.Server.ListenAndServe is itself blocking (unlike asynq.Server.Start,
// already non-blocking), so main.go needs the constructed Server back
// before deciding how to run the listen goroutine and log its terminal
// error.
func New(ctx context.Context, cfg *config.Config, log zerolog.Logger) (*Server, error) {
	repos, err := repo.NewRepositories(ctx)
	if err != nil {
		return nil, fmt.Errorf("server: repositories: %w", err)
	}

	svcs := services.NewServices(repos, asyncprocessor.Get().Client, log)
	_ = svcs // not yet consumed by any handler (no User HTTP endpoints this
	// pass) — called now anyway to prove repo.NewRepositories/
	// services.NewServices are reachable from boot; wired into
	// handler.New() once UserHandler exists.

	h := handler.New(db.Get(), redis.Get(), log)
	engine := router.New(cfg, log, h)

	return &Server{
		log: log,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.ServerConfig.Port),
			Handler:      engine,
			ReadTimeout:  time.Duration(cfg.ServerConfig.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(cfg.ServerConfig.WriteTimeout) * time.Second,
			IdleTimeout:  time.Duration(cfg.ServerConfig.IdleTimeout) * time.Second,
		},
	}, nil
}

// Start begins listening in a background goroutine and returns immediately.
// The returned channel receives exactly one value: a genuine
// ListenAndServe failure. The caller must filter out http.ErrServerClosed,
// the expected result of a graceful Shutdown.
func (s *Server) Start() <-chan error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil {
			errCh <- err
		}
	}()
	return errCh
}

// Shutdown drains in-flight requests. It does not touch db/redis/job —
// those stay owned by cmd/main.go's existing teardown, in their existing
// order.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
