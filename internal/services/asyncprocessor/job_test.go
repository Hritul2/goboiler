package asyncprocessor

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// setInstance installs a test double as the singleton and restores the
// previous state on cleanup.
func setInstance(t *testing.T, j *JobService) {
	t.Helper()
	mu.Lock()
	prev := instance
	instance = j
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		instance = prev
		mu.Unlock()
	})
}

func TestGet_PanicsBeforeInit(t *testing.T) {
	setInstance(t, nil)
	require.PanicsWithValue(t, "job: Get called before job.Init", func() { Get() })
}

func TestGet_ReturnsInstance(t *testing.T) {
	j := &JobService{}
	setInstance(t, j)
	require.Same(t, j, Get())
}

func TestInit_ErrorsWhenAlreadyInitialised(t *testing.T) {
	setInstance(t, &JobService{})

	// The double-init check precedes fetching the shared redis client, so
	// redis.Init need not have run for this assertion.
	cfg := &config.Config{RedisConfig: config.RedisConfig{Addr: "localhost:6379"}}
	_, err := Init(context.Background(), cfg, zerolog.Nop(), NewRouter())
	require.ErrorContains(t, err, "Init called more than once")
}

func TestQueues(t *testing.T) {
	q := queues()
	require.Equal(t, map[string]int{"critical": 6, "default": 3, "low": 1}, q)
}

// Compile-time check that ServeMux satisfies what registerHandlers expects,
// and that asynq.Handler is the shape Start needs.
var _ asynq.Handler = (*asynq.ServeMux)(nil)
