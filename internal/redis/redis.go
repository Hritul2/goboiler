// Package redis provides the application's Redis singleton. Unlike
// internal/db there is no store-agnostic interface: no second implementation
// is planned, and downstream consumers (asynq jobs, rate limiting) need the
// raw go-redis client.
//
// Boot order matters: Init must run after observability.New so the redisotel
// hooks pick up the global tracer and meter providers.
package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// pingTimeout bounds the connectivity check inside Init, mirroring the
// internal/db boot deadline.
const pingTimeout = 5 * time.Second

// Redis wraps the go-redis client with the lifecycle methods the boot
// sequence needs.
type Redis struct {
	rdb *goredis.Client
	log zerolog.Logger
}

var (
	mu       sync.Mutex
	instance *Redis
)

// Init connects to Redis, pings with a pingTimeout deadline (hard fail), and
// installs the package singleton. Calling Init twice is a boot-sequence bug
// and returns an error.
func Init(ctx context.Context, cfg *config.Config, log zerolog.Logger) (*Redis, error) {
	mu.Lock()
	defer mu.Unlock()

	if instance != nil {
		return nil, fmt.Errorf("redis: Init called more than once")
	}

	rdb := goredis.NewClient(clientOptions(&cfg.RedisConfig))

	if err := redisotel.InstrumentTracing(rdb); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis: instrument tracing: %w", err)
	}
	if err := redisotel.InstrumentMetrics(rdb); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis: instrument metrics: %w", err)
	}

	r := &Redis{rdb: rdb, log: log}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := r.Ping(pingCtx); err != nil {
		_ = rdb.Close()
		return nil, err
	}

	log.Info().Str("addr", cfg.RedisConfig.Addr).Int("db", cfg.RedisConfig.DB).Msg("redis connected")

	instance = r
	return instance, nil
}

// Get returns the initialised singleton. It panics if called before Init —
// that is a boot-order bug, not a runtime condition.
func Get() *Redis {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		panic("redis: Get called before redis.Init")
	}
	return instance
}

// clientOptions maps RedisConfig onto go-redis options.
func clientOptions(cfg *config.RedisConfig) *goredis.Options {
	return &goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
}

// Client exposes the raw go-redis client for consumers that need it
// (asynq, rate limiting, repositories).
func (r *Redis) Client() *goredis.Client {
	return r.rdb
}

func (r *Redis) Ping(ctx context.Context) error {
	if err := r.rdb.Ping(ctx).Err(); err != nil {
		return NormalizeRedisError(err)
	}
	return nil
}

// Close releases all connections. The ctx parameter keeps the signature
// symmetric with db.Database; go-redis Close takes none.
func (r *Redis) Close(_ context.Context) error {
	return r.rdb.Close()
}
