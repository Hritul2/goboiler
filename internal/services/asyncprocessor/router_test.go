package asyncprocessor

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_DispatchesToRegisteredHandler(t *testing.T) {
	r := NewRouter()

	var gotTask *asynq.Task
	r.Handle("test:ping", func(_ context.Context, task *asynq.Task) error {
		gotTask = task
		return nil
	})

	task := asynq.NewTask("test:ping", []byte(`{"ok":true}`))
	err := r.mux.ProcessTask(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, gotTask)
	assert.Equal(t, "test:ping", gotTask.Type())
}

func TestRouter_UnregisteredType(t *testing.T) {
	r := NewRouter()

	task := asynq.NewTask("test:unregistered", nil)
	err := r.mux.ProcessTask(context.Background(), task)

	require.Error(t, err)
}
