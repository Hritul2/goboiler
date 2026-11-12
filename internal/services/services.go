// Package services aggregates every domain service (one subpackage per
// domain, e.g. internal/services/<domain>) plus cross-cutting infra services
// like internal/services/asyncprocessor.
package services

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/repo"
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
//
// ADD YOUR DOMAIN: one field per domain service (e.g. `User *user.UserService`)
// — see docs/EXAMPLE.md step 6.
type Services struct{}

// NewServices constructs every domain service from the repositories and the
// job enqueuer.
//
// ADD YOUR DOMAIN: construct each service here (e.g.
// `User: user.NewUserService(repos.User, enqueuer, log)`) — see
// docs/EXAMPLE.md step 6.
func NewServices(repos *repo.Repositories, enqueuer Enqueuer, log zerolog.Logger) *Services {
	_, _, _ = repos, enqueuer, log // used once you construct services
	return &Services{}
}
