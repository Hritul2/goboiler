package email

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWelcomeEmailTaskOptions(t *testing.T) {
	opts := welcomeEmailTaskOptions()

	got := make(map[asynq.OptionType]any, len(opts))
	for _, o := range opts {
		got[o.Type()] = o.Value()
	}

	require.Contains(t, got, asynq.MaxRetryOpt)
	assert.Equal(t, 3, got[asynq.MaxRetryOpt])

	require.Contains(t, got, asynq.QueueOpt)
	assert.Equal(t, "default", got[asynq.QueueOpt])

	require.Contains(t, got, asynq.TimeoutOpt)
	assert.Equal(t, 30*time.Second, got[asynq.TimeoutOpt])
}

func TestNewWelcomeEmailTask(t *testing.T) {
	task, err := NewWelcomeEmailTask("ada@example.com", "Ada")
	require.NoError(t, err)
	assert.Equal(t, TaskWelcomeEmail, task.Type())

	var p WelcomeEmailPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &p))
	assert.Equal(t, "ada@example.com", p.To)
	assert.Equal(t, "Ada", p.Name)
}

func TestNewWelcomeEmailHandler(t *testing.T) {
	sender := &fakeSender{}
	handler := NewWelcomeEmailHandler(sender, zerolog.Nop())

	task, err := NewWelcomeEmailTask("ada@example.com", "Ada")
	require.NoError(t, err)

	err = handler(context.Background(), task)
	require.NoError(t, err)

	require.True(t, sender.sent)
	assert.Equal(t, []string{"ada@example.com"}, sender.last.To)
	assert.Contains(t, sender.last.HTML, "Ada")
}

func TestNewWelcomeEmailHandler_BadPayload(t *testing.T) {
	sender := &fakeSender{}
	handler := NewWelcomeEmailHandler(sender, zerolog.Nop())

	task := asynq.NewTask(TaskWelcomeEmail, []byte("not json"))

	err := handler(context.Background(), task)
	require.Error(t, err)
	assert.False(t, sender.sent, "sender must not be called on a bad payload")
}
