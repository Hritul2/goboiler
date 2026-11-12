// Package db provides the application's database singleton behind a
// store-agnostic interface. MongoDB is the active implementation; a
// Postgres implementation is sketched (commented out) in postgres.go.
//
// Boot order matters: Init must run after observability.New so the
// otelmongo command monitor picks up the global tracer provider.
package db

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// pingTimeout bounds the connectivity check inside Init, mirroring the
// 5-second observability init deadline in cmd/main.go.
const pingTimeout = 5 * time.Second

// Database is the store-agnostic contract every backing store satisfies.
// Store-specific access (e.g. *mongo.Collection) lives on the concrete
// types, reachable via GetMongo.
type Database interface {
	// Ping verifies connectivity; used at boot (hard fail) and by health checks.
	Ping(ctx context.Context) error
	// Close releases all connections. Called from stop() before the OTel flush.
	Close(ctx context.Context) error
	// DriverName identifies the active driver, e.g. "mongodb" — not the
	// name of the configured database (that lives in config.DbConfig.Name).
	DriverName() string
}

var (
	mu       sync.Mutex
	instance Database
)

// Init connects the active store, pings it with a pingTimeout deadline
// (hard fail), and installs the package singleton. Calling Init twice is a
// boot-sequence bug and returns an error.
func Init(ctx context.Context, cfg *config.Config, log zerolog.Logger) (Database, error) {
	mu.Lock()
	defer mu.Unlock()

	if instance != nil {
		return nil, fmt.Errorf("db: Init called more than once")
	}

	m, err := newMongo(ctx, &cfg.DB, log)
	if err != nil {
		return nil, err
	}

	// Postgres (future): swap the active store by replacing the block above with
	//
	//	p, err := newPostgres(ctx, &cfg.DB, log)
	//	if err != nil {
	//		return nil, err
	//	}
	//	instance = p

	instance = m
	return instance, nil
}

// Get returns the initialised singleton. It panics if called before Init —
// that is a boot-order bug, not a runtime condition.
func Get() Database {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		panic("db: Get called before db.Init")
	}
	return instance
}

// GetMongo returns the singleton as *Mongo for repositories that need
// collection access. It panics before Init or if the active store is not
// MongoDB.
func GetMongo() *Mongo {
	m, ok := Get().(*Mongo)
	if !ok {
		panic("db: active database is not MongoDB")
	}
	return m
}
