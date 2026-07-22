// Package asyncprocessor provides the application's generic async job queue
// singleton, backed by asynq over Redis. It has no knowledge of what any
// task does — dispatch is delegated entirely to a caller-supplied Router
// (router.go), which each domain (email, and whatever comes later)
// registers its own task types and handlers against at composition time
// (cmd/main.go).
//
// Init requires redis.Init to have already run — it reuses the shared,
// already-instrumented *goredis.Client from internal/redis (via
// asynq.NewClientFromRedisClient/NewServerFromRedisClient) rather than
// opening a second connection pool from raw config. asynq deliberately
// refuses to close a shared connection (see Close), so ownership of the
// underlying Redis connection stays with internal/redis: its Close runs
// after this package's, per cmd/main.go's teardown order.
//
// asynq has no Ping equivalent for the server side, so Init performs no
// synchronous connectivity check of its own — it relies on redis.Init
// having already hard-failed against the same address earlier in boot.
//
// The real end-to-end path (Start against a live Redis, task dispatch
// through the running server) is integration-verified manually via the
// local docker compose stack, not unit tested — same trade-off documented
// in internal/repo for *mongo.Collection.
package asyncprocessor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/redis"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// JobService owns the asynq client (for enqueuing) and server (for
// consuming), both built on the shared redis.Get() connection.
type JobService struct {
	Client *asynq.Client
	server *asynq.Server
}

var (
	mu       sync.Mutex
	instance *JobService
)

// queues declares queue priorities, matching the reference implementation
// (github.com/sriniously/go-boilerplate): critical work gets the most
// worker share, low the least. Hardcoded for now — promote to config if
// per-environment tuning is ever needed.
func queues() map[string]int {
	return map[string]int{
		"critical": 6,
		"default":  3,
		"low":      1,
	}
}

// Init starts consuming via the given router — non-blocking, since
// asynq.Server.Start is non-blocking (verified against the installed module
// source: it spawns its own internal goroutines and returns immediately).
// Installs the package singleton. Calling Init twice is a boot-sequence bug
// and returns an error.
func Init(ctx context.Context, cfg *config.Config, log zerolog.Logger, router *Router) (*JobService, error) {
	mu.Lock()
	defer mu.Unlock()

	if instance != nil {
		return nil, fmt.Errorf("job: Init called more than once")
	}

	// Reuse the shared, redisotel-instrumented connection instead of
	// opening a second pool — requires redis.Init to have already run.
	rdb := redis.Get().Client()

	client := asynq.NewClientFromRedisClient(rdb)
	server := asynq.NewServerFromRedisClient(rdb, asynq.Config{
		Concurrency:     10,
		Queues:          queues(),
		ShutdownTimeout: time.Duration(cfg.ServerConfig.ShutdownTimeout) * time.Second,
	})

	if err := server.Start(router.mux); err != nil {
		return nil, fmt.Errorf("job: start server: %w", err)
	}

	log.Info().Msg("job service started")

	instance = &JobService{Client: client, server: server}
	return instance, nil
}

// Get returns the initialised singleton. It panics if called before Init —
// that is a boot-order bug, not a runtime condition.
func Get() *JobService {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		panic("job: Get called before job.Init")
	}
	return instance
}

// Close stops the server (draining in-flight tasks up to
// Config.ShutdownTimeout). It does not close the underlying Redis
// connection — that connection is shared with internal/redis, which owns
// closing it (asynq itself refuses: Client.Close returns an error for a
// shared connection). The ctx parameter keeps the signature symmetric with
// db.Close/redis.Close; asynq's own Shutdown takes none.
func (j *JobService) Close(_ context.Context) error {
	j.server.Shutdown()
	return nil
}
