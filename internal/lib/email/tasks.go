package email

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// meter is the package-level instrumentation handle, following
// internal/observability's documented convention (one per package, keyed by
// import path). Reusable if a second email task is ever added — the
// "reason" attribute distinguishes failure causes across task kinds.
var (
	meter = otel.Meter("github.com/hritul2/goboiler/internal/lib/email")

	emailSentCounter, _ = meter.Int64Counter(
		"email.sent",
		metric.WithDescription("Total emails successfully sent"),
	)
	emailFailedCounter, _ = meter.Int64Counter(
		"email.failed",
		metric.WithDescription("Total email send failures, by reason"),
	)
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
			wrapped := fmt.Errorf("email: unmarshal welcome email payload: %w", err)
			log.Error().Err(wrapped).Msg("failed to unmarshal welcome email payload")
			emailFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "unmarshal")))
			return wrapped
		}

		if err := SendWelcome(ctx, sender, p.To, p.Name); err != nil {
			log.Error().Err(err).Str("to", p.To).Msg("failed to send welcome email")
			emailFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "send")))
			return err
		}

		log.Info().Str("to", p.To).Msg("sent welcome email")
		emailSentCounter.Add(ctx, 1)
		return nil
	}
}
