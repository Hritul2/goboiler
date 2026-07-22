// Package user is the User domain's business logic layer: calling the repo,
// enqueuing jobs, and hashing passwords. Depends on repo.UserRepository
// (never a concrete driver type) and an Enqueuer interface (never
// asyncprocessor's global singleton directly), so every rule here is
// unit-testable without live Mongo/Redis.
package user

import (
	"context"
	stderrors "errors"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"

	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/lib/email"
	"github.com/hritul2/goboiler/internal/models"
	"github.com/hritul2/goboiler/internal/repo"
	"github.com/hritul2/goboiler/internal/validation"
)

// Enqueuer is the narrow slice of *asynq.Client that UserService needs.
// Depending on this interface (rather than calling asyncprocessor.Get()
// directly) keeps the service unit-testable without a live Redis; the real
// wiring site (the future server struct) passes asyncprocessor.Get().Client.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// UserService owns User business rules.
type UserService struct {
	users    repo.UserRepository
	enqueuer Enqueuer
	log      zerolog.Logger
}

// NewUserService constructs a UserService.
func NewUserService(users repo.UserRepository, enqueuer Enqueuer, log zerolog.Logger) *UserService {
	return &UserService{users: users, enqueuer: enqueuer, log: log}
}

// Create validates input, hashes the password, persists the user, and
// enqueues a welcome-email task on a best-effort basis — a failed enqueue is
// logged, not returned, so signup never fails because the queue hiccuped.
func (s *UserService) Create(ctx context.Context, in validation.UserCreateInput) (*models.User, error) {
	if verrs := validation.Validate(validation.UserCreateSchema, &in); verrs.HasErrors() {
		return nil, errs.BadRequest(verrs.Error())
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errs.Internal(err)
	}

	u := models.NewUser(in.Email, in.Name, string(hash))
	if err := s.users.Create(ctx, u); err != nil {
		if isDuplicateKey(err) {
			return nil, errs.Conflict("an account with this email already exists")
		}
		return nil, err
	}

	s.enqueueWelcomeEmail(ctx, u)

	return u, nil
}

func (s *UserService) enqueueWelcomeEmail(ctx context.Context, u *models.User) {
	task, err := email.NewWelcomeEmailTask(u.Email, u.Name)
	if err != nil {
		s.log.Error().Err(err).Str("email", u.Email).Msg("failed to build welcome email task")
		return
	}
	if _, err := s.enqueuer.EnqueueContext(ctx, task); err != nil {
		s.log.Error().Err(err).Str("email", u.Email).Msg("failed to enqueue welcome email")
	}
}

// isDuplicateKey reports whether err is (or wraps) errs.ErrDuplicateKey.
func isDuplicateKey(err error) bool {
	var appErr *errs.AppError
	return stderrors.As(err, &appErr) && appErr.Code == errs.CodeConflict
}

// Authenticate verifies email/password and returns the matching user.
// Unknown-email and wrong-password return the identical error so callers
// cannot enumerate registered emails.
func (s *UserService) Authenticate(ctx context.Context, email, password string) (*models.User, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, errs.Unauthorized("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, errs.Unauthorized("invalid email or password")
	}

	return u, nil
}

// GetByID returns a user by ID.
func (s *UserService) GetByID(ctx context.Context, id bson.ObjectID) (*models.User, error) {
	return s.users.GetByID(ctx, id)
}

// Update validates input and applies it to the user identified by id.
func (s *UserService) Update(ctx context.Context, id bson.ObjectID, in validation.UserUpdateInput) (*models.User, error) {
	if verrs := validation.Validate(validation.UserUpdateSchema, &in); verrs.HasErrors() {
		return nil, errs.BadRequest(verrs.Error())
	}

	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	u.Name = in.Name
	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Delete removes a user by ID.
func (s *UserService) Delete(ctx context.Context, id bson.ObjectID) error {
	return s.users.Delete(ctx, id)
}

// List returns a page of users.
func (s *UserService) List(ctx context.Context, p models.Pagination) ([]models.User, error) {
	return s.users.List(ctx, p)
}
