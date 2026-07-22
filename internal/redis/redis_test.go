package redis

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// setInstance installs a test double as the singleton and restores the
// previous state on cleanup.
func setInstance(t *testing.T, r *Redis) {
	t.Helper()
	mu.Lock()
	prev := instance
	instance = r
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		instance = prev
		mu.Unlock()
	})
}

func TestGet_PanicsBeforeInit(t *testing.T) {
	setInstance(t, nil)
	require.PanicsWithValue(t, "redis: Get called before redis.Init", func() { Get() })
}

func TestGet_ReturnsInstance(t *testing.T) {
	r := &Redis{}
	setInstance(t, r)
	require.Same(t, r, Get())
}

func TestInit_ErrorsWhenAlreadyInitialised(t *testing.T) {
	setInstance(t, &Redis{})

	// The double-init check precedes dialing, so no network is touched.
	cfg := &config.Config{RedisConfig: config.RedisConfig{Addr: "localhost:6379"}}
	_, err := Init(context.Background(), cfg, zerolog.Nop())
	require.ErrorContains(t, err, "Init called more than once")
}

func TestClientOptions(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.RedisConfig
	}{
		{"local_no_auth", config.RedisConfig{Addr: "localhost:6379", Password: "", DB: 0}},
		{"remote_with_auth", config.RedisConfig{Addr: "redis.example.com:6380", Password: "s3cret", DB: 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := clientOptions(&tt.cfg)
			require.Equal(t, tt.cfg.Addr, opts.Addr)
			require.Equal(t, tt.cfg.Password, opts.Password)
			require.Equal(t, tt.cfg.DB, opts.DB)
		})
	}
}
