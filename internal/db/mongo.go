package db

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// Mongo is the MongoDB-backed Database implementation.
type Mongo struct {
	client *mongo.Client
	db     *mongo.Database
	log    zerolog.Logger
}

var _ Database = (*Mongo)(nil)

// newMongo builds client options from cfg, connects, and verifies the
// deployment is reachable with a pingTimeout deadline. On ping failure the
// client is disconnected so nothing leaks.
func newMongo(ctx context.Context, cfg *config.DbConfig, log zerolog.Logger) (*Mongo, error) {
	// mongo.Connect is lazy in driver v2 — the real dial happens on Ping.
	client, err := mongo.Connect(clientOptions(cfg))
	if err != nil {
		return nil, MapDBError(err)
	}

	m := &Mongo{
		client: client,
		db:     client.Database(cfg.Name),
		log:    log,
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := m.Ping(pingCtx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	log.Info().Str("database", cfg.Name).Msg("mongodb connected")
	return m, nil
}

// clientOptions maps DbConfig onto driver options (see the mapping table on
// config.DbConfig). ConnMaxLifetime has no Mongo equivalent and is ignored.
func clientOptions(cfg *config.DbConfig) *options.ClientOptions {
	return options.Client().
		ApplyURI(cfg.Uri).
		SetMaxPoolSize(uint64(cfg.MaxOpenConns)).
		SetMinPoolSize(uint64(cfg.MaxIdleConns)).
		SetMaxConnIdleTime(time.Duration(cfg.ConnMaxIdleTime) * time.Second).
		SetMonitor(otelmongo.NewMonitor())
}

func (m *Mongo) Ping(ctx context.Context) error {
	if err := m.client.Ping(ctx, readpref.Primary()); err != nil {
		return MapDBError(err)
	}
	return nil
}

func (m *Mongo) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

func (m *Mongo) DriverName() string {
	return "mongodb"
}

// Client exposes the raw driver client for advanced use (transactions,
// change streams).
func (m *Mongo) Client() *mongo.Client {
	return m.client
}

// DB returns the configured *mongo.Database. Named DB rather than Database
// to avoid clashing with the interface type.
func (m *Mongo) DB() *mongo.Database {
	return m.db
}

// Collection returns a handle to the named collection in the configured
// database.
func (m *Mongo) Collection(name string) *mongo.Collection {
	return m.db.Collection(name)
}
