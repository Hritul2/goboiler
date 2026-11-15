package services

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/models"
	"github.com/hritul2/goboiler/internal/repo"
)

// stubUserRepo is a minimal repo.UserRepository — this test only checks that
// NewServices wires a non-nil UserService, not any repo behavior.
type stubUserRepo struct{}

func (stubUserRepo) Create(context.Context, *models.User) error                     { return nil }
func (stubUserRepo) GetByID(context.Context, models.ID) (*models.User, error)       { return nil, nil }
func (stubUserRepo) GetByEmail(context.Context, string) (*models.User, error)       { return nil, nil }
func (stubUserRepo) Update(context.Context, *models.User) error                     { return nil }
func (stubUserRepo) Delete(context.Context, models.ID) error                        { return nil }
func (stubUserRepo) List(context.Context, models.Pagination) ([]models.User, error) { return nil, nil }
func (stubUserRepo) EnsureIndexes(context.Context) error                            { return nil }

// stubEnqueuer is a minimal user.Enqueuer.
type stubEnqueuer struct{}

func (stubEnqueuer) EnqueueContext(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, nil
}

func TestNewServices(t *testing.T) {
	repos := &repo.Repositories{User: stubUserRepo{}}

	svcs := NewServices(repos, stubEnqueuer{}, zerolog.Nop())

	require.NotNil(t, svcs)
	require.NotNil(t, svcs.User)
}
