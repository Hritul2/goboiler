// Package services aggregates every domain service (one subpackage per
// domain, e.g. internal/services/user) plus cross-cutting infra services like
// internal/services/asyncprocessor.
package services

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/repo"
	"github.com/hritul2/goboiler/internal/services/user"
)

// Enqueuer is the narrow slice of *asynq.Client that services need to enqueue
// background jobs. NewServices takes it (instead of the asyncprocessor
// singleton) so services stay unit-testable without Redis; a domain service
// declares its own identical interface and receives this value.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Services aggregates every domain service; internal/services/server builds it
// once at boot and hands it to handler.New.
type Services struct {
	User *user.UserService
}

// NewServices constructs every domain service.
func NewServices(repos *repo.Repositories, enqueuer Enqueuer, log zerolog.Logger) *Services {
	return &Services{
		User: user.NewUserService(repos.User, enqueuer, log),
	}
}
