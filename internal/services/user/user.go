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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/bcrypt"

	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/lib/email"
	"github.com/hritul2/goboiler/internal/models"
	"github.com/hritul2/goboiler/internal/repo"
	"github.com/hritul2/goboiler/internal/validation"
)

// tracer is the package-level tracer, following internal/observability's
// documented convention (one per package, keyed by import path).
var tracer = otel.Tracer("github.com/hritul2/goboiler/internal/services/user")

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
	ctx, span := tracer.Start(ctx, "UserService.Create")
	defer span.End()

	if verrs := validation.Validate(&in); verrs.HasErrors() {
		err := errs.BadRequest(verrs.Error())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		appErr := errs.Internal(err)
		span.RecordError(appErr)
		span.SetStatus(codes.Error, appErr.Error())
		s.log.Error().Err(err).Str("email", in.Email).Msg("failed to hash password")
		return nil, appErr
	}

	u := models.NewUser(in.Email, in.Name, string(hash))
	if err := s.users.Create(ctx, u); err != nil {
		if isDuplicateKey(err) {
			conflictErr := errs.Conflict("an account with this email already exists")
			span.RecordError(conflictErr)
			span.SetStatus(codes.Error, conflictErr.Error())
			s.log.Error().Err(err).Str("email", in.Email).Msg("create user failed: duplicate email")
			return nil, conflictErr
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("email", in.Email).Msg("create user failed")
		return nil, err
	}

	s.enqueueWelcomeEmail(ctx, u)

	s.log.Info().Str("user_id", u.ID.Hex()).Str("email", u.Email).Msg("user created")
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
	ctx, span := tracer.Start(ctx, "UserService.Authenticate")
	defer span.End()

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		authErr := errs.Unauthorized("invalid email or password")
		span.RecordError(authErr)
		span.SetStatus(codes.Error, authErr.Error())
		// Warn, not Error: a bad login attempt is client-caused, not an
		// application fault.
		s.log.Warn().Str("email", email).Msg("authentication failed: unknown email")
		return nil, authErr
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		authErr := errs.Unauthorized("invalid email or password")
		span.RecordError(authErr)
		span.SetStatus(codes.Error, authErr.Error())
		s.log.Warn().Str("email", email).Msg("authentication failed: wrong password")
		return nil, authErr
	}

	return u, nil
}

// GetByID returns a user by ID. Read path: error-level logging only — an
// info line per read would be volume with no operational value beyond what
// the span already gives for free.
func (s *UserService) GetByID(ctx context.Context, id bson.ObjectID) (*models.User, error) {
	ctx, span := tracer.Start(ctx, "UserService.GetByID")
	defer span.End()

	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.Hex()).Msg("get user failed")
		return nil, err
	}
	return u, nil
}

// Update validates input and applies it to the user identified by id.
func (s *UserService) Update(ctx context.Context, id bson.ObjectID, in validation.UserUpdateInput) (*models.User, error) {
	ctx, span := tracer.Start(ctx, "UserService.Update")
	defer span.End()

	if verrs := validation.Validate(&in); verrs.HasErrors() {
		err := errs.BadRequest(verrs.Error())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.Hex()).Msg("update user failed: lookup")
		return nil, err
	}

	u.Name = in.Name
	if err := s.users.Update(ctx, u); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.Hex()).Msg("update user failed")
		return nil, err
	}

	s.log.Info().Str("user_id", id.Hex()).Msg("user updated")
	return u, nil
}

// Delete removes a user by ID.
func (s *UserService) Delete(ctx context.Context, id bson.ObjectID) error {
	ctx, span := tracer.Start(ctx, "UserService.Delete")
	defer span.End()

	if err := s.users.Delete(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.Hex()).Msg("delete user failed")
		return err
	}

	s.log.Info().Str("user_id", id.Hex()).Msg("user deleted")
	return nil
}

// List returns a page of users. Read path: error-level logging only, same
// reasoning as GetByID.
func (s *UserService) List(ctx context.Context, p models.Pagination) ([]models.User, error) {
	ctx, span := tracer.Start(ctx, "UserService.List")
	defer span.End()

	users, err := s.users.List(ctx, p)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Msg("list users failed")
		return nil, err
	}
	return users, nil
}
