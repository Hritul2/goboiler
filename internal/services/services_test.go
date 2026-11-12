package services

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/repo"
)

// stubEnqueuer is a minimal Enqueuer.
type stubEnqueuer struct{}

func (stubEnqueuer) EnqueueContext(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, nil
}

// The aggregate wires with no domains registered (the boilerplate skeleton).
func TestNewServices_EmptySkeleton(t *testing.T) {
	svcs := NewServices(&repo.Repositories{}, stubEnqueuer{}, zerolog.Nop())

	require.NotNil(t, svcs)
}
