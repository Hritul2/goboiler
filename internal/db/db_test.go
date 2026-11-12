package db

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// fakeDB is a no-op Database used to exercise singleton semantics without a
// live store.
type fakeDB struct{}

func (fakeDB) Ping(context.Context) error  { return nil }
func (fakeDB) Close(context.Context) error { return nil }
func (fakeDB) DriverName() string          { return "fake" }

// setInstance installs a test double as the singleton and restores the
// previous state on cleanup.
func setInstance(t *testing.T, d Database) {
	t.Helper()
	mu.Lock()
	prev := instance
	instance = d
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		instance = prev
		mu.Unlock()
	})
}

func TestGet_PanicsBeforeInit(t *testing.T) {
	setInstance(t, nil)
	require.PanicsWithValue(t, "db: Get called before db.Init", func() { Get() })
}

func TestGet_ReturnsInstance(t *testing.T) {
	fake := fakeDB{}
	setInstance(t, fake)
	require.Equal(t, Database(fake), Get())
}

func TestGetMongo_PanicsWhenNotMongo(t *testing.T) {
	setInstance(t, fakeDB{})
	require.PanicsWithValue(t, "db: active database is not MongoDB", func() { GetMongo() })
}

func TestInit_ErrorsWhenAlreadyInitialised(t *testing.T) {
	setInstance(t, fakeDB{})

	cfg := &config.Config{DB: config.DbConfig{
		Uri: "mongodb://localhost:27017", Name: "t",
		MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: 1, ConnMaxIdleTime: 1,
	}}
	_, err := Init(context.Background(), cfg, zerolog.Nop())
	require.ErrorContains(t, err, "Init called more than once")
}
