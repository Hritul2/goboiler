package user

import (
	"context"
	"fmt"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/models"
	th "github.com/hritul2/goboiler/internal/testing"
	"github.com/hritul2/goboiler/internal/validation"
)

// fakeUserRepo is an in-memory repo.UserRepository for unit tests — no live
// Mongo needed.
type fakeUserRepo struct {
	byID       map[models.ID]*models.User
	createErr  error
	createCall int
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: make(map[models.ID]*models.User)}
}

func (f *fakeUserRepo) Create(_ context.Context, u *models.User) error {
	f.createCall++
	if f.createErr != nil {
		return f.createErr
	}
	if u.ID.IsZero() {
		// Like a real store, the repo assigns the ID. Deliberately NOT an
		// ObjectID: the service must work with any ID format.
		u.ID = models.ID(fmt.Sprintf("fake-id-%d", f.createCall))
	}
	f.byID[u.ID] = u
	return nil
}

func (f *fakeUserRepo) GetByID(_ context.Context, id models.ID) (*models.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, errs.ErrRecordNotFound
}

func (f *fakeUserRepo) GetByEmail(_ context.Context, email string) (*models.User, error) {
	for _, u := range f.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, errs.ErrRecordNotFound
}

func (f *fakeUserRepo) Update(_ context.Context, u *models.User) error {
	if _, ok := f.byID[u.ID]; !ok {
		return errs.ErrRecordNotFound
	}
	f.byID[u.ID] = u
	return nil
}

func (f *fakeUserRepo) Delete(_ context.Context, id models.ID) error {
	if _, ok := f.byID[id]; !ok {
		return errs.ErrRecordNotFound
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeUserRepo) List(_ context.Context, _ models.Pagination) ([]models.User, error) {
	out := make([]models.User, 0, len(f.byID))
	for _, u := range f.byID {
		out = append(out, *u)
	}
	return out, nil
}

func (f *fakeUserRepo) EnsureIndexes(_ context.Context) error { return nil }

// fakeEnqueuer captures the last enqueued task and can be told to fail.
type fakeEnqueuer struct {
	lastTask   *asynq.Task
	enqueueErr error
	callCount  int
}

func (f *fakeEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.callCount++
	if f.enqueueErr != nil {
		return nil, f.enqueueErr
	}
	f.lastTask = task
	return &asynq.TaskInfo{}, nil
}

// example:region service-test
func newTestService() (*UserService, *fakeUserRepo, *fakeEnqueuer) {
	repo := newFakeUserRepo()
	enq := &fakeEnqueuer{}
	return NewUserService(repo, enq, zerolog.Nop()), repo, enq
}

// example:endregion service-test

func TestUserService_Create_Success(t *testing.T) {
	svc, repo, enq := newTestService()

	u, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)
	assert.Equal(t, "ada@example.com", u.Email)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("password123")),
		"password must be hashed, not stored in plaintext")
	assert.NotEqual(t, "password123", u.PasswordHash)

	require.Equal(t, 1, repo.createCall)
	require.Equal(t, 1, enq.callCount)
	require.NotNil(t, enq.lastTask)
	assert.Equal(t, "email:welcome", enq.lastTask.Type())
}

func TestUserService_Create_ValidationFailure(t *testing.T) {
	svc, repo, enq := newTestService()

	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "not-an-email", Name: "Ada", Password: "password123",
	})
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeBadRequest)
	var appErr *errs.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, map[string][]string{"email": {"must be a valid email address"}}, appErr.Fields,
		"service validation errors carry the same JSON-keyed fields map as the API adapter")
	assert.Equal(t, 0, repo.createCall, "repo must not be called on validation failure")
	assert.Equal(t, 0, enq.callCount, "enqueuer must not be called on validation failure")
}

// example:region service-test
func TestUserService_Create_DuplicateEmail(t *testing.T) {
	svc, repo, _ := newTestService()
	repo.createErr = errs.ErrDuplicateKey

	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeConflict)
}

// example:endregion service-test

func TestUserService_Create_EnqueueFailureDoesNotFailSignup(t *testing.T) {
	svc, _, enq := newTestService()
	enq.enqueueErr = assert.AnError

	u, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err, "signup must succeed even if the welcome-email enqueue fails")
	assert.Equal(t, "ada@example.com", u.Email)
}

func TestUserService_Authenticate_Success(t *testing.T) {
	svc, _, _ := newTestService()
	created, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	got, err := svc.Authenticate(context.Background(), "ada@example.com", "password123")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}

func TestUserService_Authenticate_WrongPassword(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	_, err = svc.Authenticate(context.Background(), "ada@example.com", "wrong-password")
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeUnauthorized)
}

func TestUserService_Authenticate_UnknownEmail(t *testing.T) {
	svc, _, _ := newTestService()

	_, err := svc.Authenticate(context.Background(), "nobody@example.com", "password123")
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeUnauthorized)
}

func TestUserService_GetByID_NotFound(t *testing.T) {
	svc, _, _ := newTestService()

	_, err := svc.GetByID(context.Background(), "missing-id")
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeNotFound)
}

func TestUserService_Update_ValidationFailure(t *testing.T) {
	svc, _, _ := newTestService()
	created, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), created.ID, validation.UserUpdateInput{Name: "A"})
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeBadRequest)
}

func TestUserService_Update_Success(t *testing.T) {
	svc, _, _ := newTestService()
	created, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	updated, err := svc.Update(context.Background(), created.ID, validation.UserUpdateInput{Name: "Ada Lovelace"})
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", updated.Name)
}

func TestUserService_Delete(t *testing.T) {
	svc, _, _ := newTestService()
	created, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(context.Background(), created.ID))

	_, err = svc.GetByID(context.Background(), created.ID)
	th.AssertErrorCode(t, err, errs.CodeNotFound)
}

func TestUserService_List(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	list, err := svc.List(context.Background(), models.Pagination{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
