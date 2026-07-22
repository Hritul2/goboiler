package email

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
)

// TaskWelcomeEmail is the asynq task type for the post-signup welcome email.
// Register a handler for it against an asyncprocessor.Router at composition
// time (see NewWelcomeEmailHandler).
const TaskWelcomeEmail = "email:welcome"

// WelcomeEmailPayload carries the fields NewWelcomeEmailHandler needs. Name
// matches models.User/SendWelcome's single-name shape (not split
// first/last names).
type WelcomeEmailPayload struct {
	To   string `json:"to"`
	Name string `json:"name"`
}

// welcomeEmailTaskOptions is the task-processing config for welcome emails
// (pure, unit-tested).
func welcomeEmailTaskOptions() []asynq.Option {
	return []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue("default"),
		asynq.Timeout(30 * time.Second),
	}
}

// NewWelcomeEmailTask is the producer: builds the enqueue-ready task for a
// welcome email. Callers (e.g. internal/services/user) hand this to an
// Enqueuer; they never need to know the payload shape.
func NewWelcomeEmailTask(to, name string) (*asynq.Task, error) {
	payload, err := json.Marshal(WelcomeEmailPayload{To: to, Name: name})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskWelcomeEmail, payload, welcomeEmailTaskOptions()...), nil
}

// NewWelcomeEmailHandler is the consumer factory: returns the function to
// register against an asyncprocessor.Router for TaskWelcomeEmail. The
// returned func matches asyncprocessor.HandlerFunc structurally — no import
// of asyncprocessor needed here.
func NewWelcomeEmailHandler(sender Sender, log zerolog.Logger) func(ctx context.Context, task *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var p WelcomeEmailPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("email: unmarshal welcome email payload: %w", err)
		}

		if err := SendWelcome(ctx, sender, p.To, p.Name); err != nil {
			log.Error().Err(err).Str("to", p.To).Msg("failed to send welcome email")
			return err
		}

		log.Info().Str("to", p.To).Msg("sent welcome email")
		return nil
	}
}
