// Package services aggregates every domain service (internal/services/user,
// and future domains) plus cross-cutting infra services like
// internal/services/asyncprocessor.
package services

import (
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/repo"
	"github.com/hritul2/goboiler/internal/services/user"
)

// Services aggregates every domain service. Wiring this into main.go/the
// server struct is step 16's job — main.go currently stops at db/redis/job.
type Services struct {
	User *user.UserService
}

// NewServices constructs every domain service.
func NewServices(repos *repo.Repositories, enqueuer user.Enqueuer, log zerolog.Logger) *Services {
	return &Services{
		User: user.NewUserService(repos.User, enqueuer, log),
	}
}
