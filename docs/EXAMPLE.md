# Example: one feature, end to end (User)

> Last updated: 2026-11-12
> Code verified against commit: c7162da (the tree just before the User code was
> removed in that same commit; the code below was compiled and tested there).

goboiler ships **no domain code**: no users, no endpoints, no jobs. This
document is the missing example. It builds one complete feature — `User`:
sign-up with a hashed password, a welcome email sent by a background job, five
typed RPC functions, a generated TypeScript client, and a Clerk webhook — one
layer at a time, in the order you would write it, with the full code and the
test at every step.

**Where the code comes from.** Steps 1–9 and 11 are the real code this
repository carried before the example was removed: it compiled, its tests
passed, and every block below was copied from those files by a script, not
retyped. The two blocks labelled **sketch** (a Postgres repository in step 2,
and the Clerk webhook in step 10) are not part of the repository: the webhook
was compiled and tested once in a scratch copy when this page was written; the
Postgres block was not compiled.

**How to use it.** Replace `User`/`user` with your entity, copy the files into
the paths shown, and use your module path for imports (the snippets show the module path of
this repository; `make init` rewrote it in the docs along with the code, so
they already show yours). Each step names
the `// ADD YOUR DOMAIN` comment in the skeleton where that step plugs in. You
do not have to build it all: steps 1–4 and 6–8 are a complete feature; step 5
(the job), 9 (frontend), 10 (webhook) and 11 (v2) are independent extras.

| Step | What | Files |
|---|---|---|
| 1 | Model | `internal/models/user.go` |
| 2 | Repository: interface, Mongo implementation | `internal/repo/user.go`, `internal/repo/mongo/user.go`, `internal/repo/repositories.go` |
| 3 | Validation input | `internal/validation/user.go` |
| 4 | Service | `internal/services/user/user.go` |
| 5 | Welcome-email job | `internal/lib/email/{emails,templates,tasks}.go`, `internal/services/registerasynctasks.go` |
| 6 | Service wiring | `internal/services/services.go` |
| 7 | DTOs, handler, endpoints | `internal/handler/user/*.go`, `internal/handler/handlers.go` |
| 8 | Register in the API version | `internal/router/versions.go` |
| 9 | Generate the client, use it | `make gen`, your frontend |
| 10 | Clerk `user.created` webhook | `internal/webhook/clerk/` (sketch) |
| 11 | Add `/api/v2` | `internal/handler/userv2/`, `versions.go` |

The layers only depend downward: handler → service → repo interface ← repo
implementation (the only place that knows MongoDB). Nothing above
`internal/repo/mongo` imports the database driver; `internal/archtest` fails
the tests if it does. See `CODING_SEQUENCE.md` for the reasoning behind each
layer.

---

## Step 1 — Model

*Plugs in at:* the `// ADD YOUR DOMAIN` comment in `internal/models/base.go`.

A model is a plain struct embedding `models.Base` (`ID models.ID`,
`CreatedAt`, `UpdatedAt`; already in the repo). It has no database tags: how it
is stored is the repository's business. `models.ID` is an opaque string — the
store assigns it on `Create`, so a new model has an empty ID. Secrets are hidden
at the model layer (`json:"-"` on the password hash).

**`internal/models/user.go`**

```go
package models

// User is the account entity. PasswordHash is never serialised to JSON.
// The ID is empty until UserRepository.Create assigns it.
type User struct {
	Base
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
}

// NewUser returns a User with timestamps set and no ID yet (the repository
// assigns it on Create). The caller is responsible for hashing the password
// before constructing the model.
func NewUser(email, name, passwordHash string) *User {
	return &User{
		Base:         NewBase(),
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
	}
}
```

Test: pure struct behaviour, no mocks.

**`internal/models/user_test.go`**

```go
package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	th "github.com/hritul2/goboiler/internal/testing"
)

func TestNewUser(t *testing.T) {
	u := NewUser("ada@example.com", "Ada", "hash123")

	assert.True(t, u.ID.IsZero(), "ID is assigned by the repository on Create")
	assert.Equal(t, "ada@example.com", u.Email)
	assert.Equal(t, "Ada", u.Name)
	assert.Equal(t, "hash123", u.PasswordHash)
	assert.False(t, u.CreatedAt.IsZero())
}

func TestUser_JSONHidesPasswordHash(t *testing.T) {
	u := NewUser("ada@example.com", "Ada", "supersecrethash")
	u.ID = "u_123"

	out, err := json.Marshal(u)
	require.NoError(t, err)

	s := string(out)
	th.AssertStringContains(t, s, `"id":"u_123"`, `"email":"ada@example.com"`, `"name":"Ada"`)
	assert.NotContains(t, s, "password_hash")
	assert.NotContains(t, s, "supersecrethash")
}
```

---

## Step 2 — Repository

*Plugs in at:* the `// ADD YOUR DOMAIN` comments in `internal/repo/repositories.go`
and the package comment of `internal/repo/mongo`.

Two parts. The **interface** lives in `internal/repo` and mentions no driver;
services depend on it. The **implementation** lives in `internal/repo/<driver>`.

**`internal/repo/user.go`**

```go
package repo

import (
	"context"

	"github.com/hritul2/goboiler/internal/models"
)

// UserRepository is the data-access contract for User entities. It is
// driver-agnostic: IDs are opaque models.ID strings and every implementation
// returns errs sentinels (errs.ErrRecordNotFound, errs.ErrDuplicateKey, ...)
// rather than driver errors.
//
// Contract every implementation must honour:
//   - Create assigns u.ID when it is empty (the ID format is the store's
//     decision) and fills any zero timestamp.
//   - GetByID/Update/Delete return errs.ErrRecordNotFound for an ID that does
//     not exist — including one that is malformed for this store (it cannot
//     exist there).
//   - A duplicate email on Create returns errs.ErrDuplicateKey.
type UserRepository interface {
	Create(ctx context.Context, u *models.User) error
	GetByID(ctx context.Context, id models.ID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, u *models.User) error
	Delete(ctx context.Context, id models.ID) error
	List(ctx context.Context, p models.Pagination) ([]models.User, error)
	// EnsureIndexes idempotently prepares the store for queries (indexes,
	// constraints). Called once at wiring time by NewRepositories. A SQL
	// implementation whose schema comes from migrations can make it a no-op.
	EnsureIndexes(ctx context.Context) error
}
```

The Mongo implementation has a private document struct (the only place with
`bson` tags), explicit conversion to and from the model, ID parsing (a string
that is not a valid ObjectID cannot exist in this store, so it is
`errs.ErrRecordNotFound` — a 404, not a validation error), index definitions,
and every driver error passed through `db.MapDBError` so callers only ever see
`errs` sentinels (`ErrRecordNotFound`, `ErrDuplicateKey`, ...). No tracing or
logging is added here (otelmongo already traces every query); metrics are, via
`instrumentOperation` from `internal/repo/mongo/instrument.go`, which stays in
the skeleton.

**`internal/repo/mongo/user.go`**

```go
package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/hritul2/goboiler/internal/db"
	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/models"
)

const usersCollection = "users"

// userDoc is the persisted shape of models.User. It is private on purpose:
// the bson layout is this package's business. The field names match what the
// previous model-with-bson-tags layout wrote, so existing documents stay
// readable.
type userDoc struct {
	ID           bson.ObjectID `bson:"_id"`
	Email        string        `bson:"email"`
	Name         string        `bson:"name"`
	PasswordHash string        `bson:"password_hash"`
	CreatedAt    time.Time     `bson:"created_at"`
	UpdatedAt    time.Time     `bson:"updated_at"`
}

// parseObjectID converts a model ID into an ObjectID. An ID that is not a
// valid ObjectID hex string (including "") cannot exist in this store, so
// the answer to "find/update/delete this ID" is "not found".
func parseObjectID(id models.ID) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(string(id))
	if err != nil {
		return bson.ObjectID{}, errs.ErrRecordNotFound
	}
	return oid, nil
}

// toUserDoc converts a model into its document. An empty ID stays zero (the
// caller decides whether to generate one); a non-empty ID must be a valid
// ObjectID hex string.
func toUserDoc(u *models.User) (userDoc, error) {
	var oid bson.ObjectID
	if !u.ID.IsZero() {
		var err error
		if oid, err = parseObjectID(u.ID); err != nil {
			return userDoc{}, err
		}
	}
	return userDoc{
		ID:           oid,
		Email:        u.Email,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}, nil
}

// toModel converts a document back into the domain model.
func (d userDoc) toModel() *models.User {
	return &models.User{
		Base: models.Base{
			ID:        models.ID(d.ID.Hex()),
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		},
		Email:        d.Email,
		Name:         d.Name,
		PasswordHash: d.PasswordHash,
	}
}

// UserRepository is the Mongo-backed implementation of repo.UserRepository.
// It does not import internal/repo (that would be an import cycle, since
// repo's wiring constructs it); the interface is asserted at compile time in
// internal/repo/repositories.go.
type UserRepository struct {
	col *mongodrv.Collection
}

// NewUserRepository returns the Mongo-backed user repository. Requires
// db.Init to have run (boot order).
func NewUserRepository() *UserRepository {
	return &UserRepository{col: db.GetMongo().Collection(usersCollection)}
}

// userIndexModels declares the users collection's indexes (pure, unit-tested).
func userIndexModels() []mongodrv.IndexModel {
	return []mongodrv.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}
}

// listFindOptions maps normalized pagination onto find options, newest first
// (pure, unit-tested).
func listFindOptions(p models.Pagination) *options.FindOptionsBuilder {
	return options.Find().
		SetSkip(p.Skip()).
		SetLimit(int64(p.Limit)).
		SetSort(bson.D{{Key: "created_at", Value: -1}})
}

// EnsureIndexes idempotently creates the collection's indexes.
func (r *UserRepository) EnsureIndexes(ctx context.Context) error {
	return instrumentOperation(ctx, "ensure_indexes", func() error {
		if _, err := r.col.Indexes().CreateMany(ctx, userIndexModels()); err != nil {
			return db.MapDBError(err)
		}
		return nil
	})
}

// Create persists u and assigns u.ID (the hex of a fresh ObjectID) when it
// is empty. A caller-supplied non-empty ID must be a valid ObjectID hex
// string; timestamps left zero are set to now (UTC).
func (r *UserRepository) Create(ctx context.Context, u *models.User) error {
	return instrumentOperation(ctx, "create", func() error {
		doc, err := toUserDoc(u)
		if err != nil {
			return errs.BadRequest("user id is not a valid ObjectID")
		}
		if doc.ID.IsZero() {
			doc.ID = bson.NewObjectID()
		}
		if now := time.Now().UTC(); doc.CreatedAt.IsZero() || doc.UpdatedAt.IsZero() {
			if doc.CreatedAt.IsZero() {
				doc.CreatedAt = now
			}
			if doc.UpdatedAt.IsZero() {
				doc.UpdatedAt = doc.CreatedAt
			}
		}

		if _, err := r.col.InsertOne(ctx, doc); err != nil {
			// Duplicate email surfaces as errs.ErrDuplicateKey via the mapper.
			return db.MapDBError(err)
		}

		u.ID = models.ID(doc.ID.Hex())
		u.CreatedAt, u.UpdatedAt = doc.CreatedAt, doc.UpdatedAt
		return nil
	})
}

// GetByID returns the user with the given ID, or errs.ErrRecordNotFound —
// including for an ID that is not a valid ObjectID.
func (r *UserRepository) GetByID(ctx context.Context, id models.ID) (*models.User, error) {
	return r.findOne(ctx, "get_by_id", func() (bson.M, error) {
		oid, err := parseObjectID(id)
		if err != nil {
			return nil, err
		}
		return bson.M{"_id": oid}, nil
	})
}

// GetByEmail returns the user with the given email, or errs.ErrRecordNotFound.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	return r.findOne(ctx, "get_by_email", func() (bson.M, error) {
		return bson.M{"email": email}, nil
	})
}

// findOne runs a single-document lookup. The filter is built inside the
// instrumented section so a rejected ID is counted like any other failed
// lookup.
func (r *UserRepository) findOne(ctx context.Context, operation string, filter func() (bson.M, error)) (*models.User, error) {
	var doc userDoc
	err := instrumentOperation(ctx, operation, func() error {
		f, err := filter()
		if err != nil {
			return err
		}
		if err := r.col.FindOne(ctx, f).Decode(&doc); err != nil {
			return db.MapDBError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return doc.toModel(), nil
}

// Update replaces the stored user identified by u.ID and bumps u.UpdatedAt.
func (r *UserRepository) Update(ctx context.Context, u *models.User) error {
	return instrumentOperation(ctx, "update", func() error {
		doc, err := toUserDoc(u)
		if err != nil {
			return err
		}
		if doc.ID.IsZero() {
			return errs.ErrRecordNotFound // never persisted
		}

		u.Touch()
		doc.UpdatedAt = u.UpdatedAt

		res, err := r.col.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc)
		if err != nil {
			return db.MapDBError(err)
		}
		if res.MatchedCount == 0 {
			return errs.ErrRecordNotFound
		}
		return nil
	})
}

// Delete removes the user with the given ID, or returns
// errs.ErrRecordNotFound (also for an ID that is not a valid ObjectID).
func (r *UserRepository) Delete(ctx context.Context, id models.ID) error {
	return instrumentOperation(ctx, "delete", func() error {
		oid, err := parseObjectID(id)
		if err != nil {
			return err
		}
		res, err := r.col.DeleteOne(ctx, bson.M{"_id": oid})
		if err != nil {
			return db.MapDBError(err)
		}
		if res.DeletedCount == 0 {
			return errs.ErrRecordNotFound
		}
		return nil
	})
}

// List returns one page of users, newest first.
func (r *UserRepository) List(ctx context.Context, p models.Pagination) ([]models.User, error) {
	var users []models.User
	err := instrumentOperation(ctx, "list", func() error {
		p.Normalize()

		cursor, err := r.col.Find(ctx, bson.M{}, listFindOptions(p))
		if err != nil {
			return db.MapDBError(err)
		}

		var docs []userDoc
		if err := cursor.All(ctx, &docs); err != nil {
			return db.MapDBError(err)
		}

		users = make([]models.User, 0, p.Limit)
		for _, d := range docs {
			users = append(users, *d.toModel())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}
```

Test: the pure parts (ID parsing, conversion, BSON layout, index and find
options) and the "malformed ID never reaches the driver" rule, using a
repository with a nil collection — no database needed. The CRUD glue itself is
checked against a real Mongo from `docker-compose.local.yml`, not mocked.

**`internal/repo/mongo/user_test.go`**

```go
package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/models"
)

func TestUserIndexModels(t *testing.T) {
	idx := userIndexModels()
	require.Len(t, idx, 1, "users collection defines exactly one index")

	keys, ok := idx[0].Keys.(bson.D)
	require.True(t, ok, "Keys must be an order-preserving bson.D")
	require.Len(t, keys, 1)
	assert.Equal(t, "email", keys[0].Key)
	assert.EqualValues(t, 1, keys[0].Value)

	require.NotNil(t, idx[0].Options)
	var opts options.IndexOptions
	for _, set := range idx[0].Options.List() {
		require.NoError(t, set(&opts))
	}
	require.NotNil(t, opts.Unique, "email index must set Unique")
	assert.True(t, *opts.Unique)
}

func TestListFindOptions(t *testing.T) {
	p := models.Pagination{Page: 3, Limit: 10}
	p.Normalize()

	var opts options.FindOptions
	for _, set := range listFindOptions(p).List() {
		require.NoError(t, set(&opts))
	}

	require.NotNil(t, opts.Skip)
	assert.EqualValues(t, 20, *opts.Skip)
	require.NotNil(t, opts.Limit)
	assert.EqualValues(t, 10, *opts.Limit)

	sort, ok := opts.Sort.(bson.D)
	require.True(t, ok, "Sort must be an order-preserving bson.D")
	require.Len(t, sort, 1)
	assert.Equal(t, "created_at", sort[0].Key)
	assert.EqualValues(t, -1, sort[0].Value, "newest first")
}

func TestParseObjectID(t *testing.T) {
	oid := bson.NewObjectID()

	got, err := parseObjectID(models.ID(oid.Hex()))
	require.NoError(t, err)
	assert.Equal(t, oid, got)

	for _, bad := range []models.ID{"", "not-an-object-id", "abc123", models.ID(oid.Hex() + "0"), "zzzzzzzzzzzzzzzzzzzzzzzz"} {
		_, err := parseObjectID(bad)
		assert.ErrorIs(t, err, errs.ErrRecordNotFound, "id %q cannot exist in this store", bad)
	}
}

func TestUserDoc_RoundTrip(t *testing.T) {
	oid := bson.NewObjectID()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	in := &models.User{
		Base:         models.Base{ID: models.ID(oid.Hex()), CreatedAt: created, UpdatedAt: created.Add(time.Hour)},
		Email:        "ada@example.com",
		Name:         "Ada",
		PasswordHash: "hash",
	}

	doc, err := toUserDoc(in)
	require.NoError(t, err)
	assert.Equal(t, oid, doc.ID)

	assert.Equal(t, in, doc.toModel())
}

func TestUserDoc_EmptyIDStaysZero(t *testing.T) {
	doc, err := toUserDoc(models.NewUser("a@b.c", "A", "h"))
	require.NoError(t, err)
	assert.True(t, doc.ID.IsZero(), "the repo, not the converter, generates IDs")
}

func TestUserDoc_RejectsMalformedID(t *testing.T) {
	_, err := toUserDoc(&models.User{Base: models.Base{ID: "nope"}})
	assert.ErrorIs(t, err, errs.ErrRecordNotFound)
}

// TestUserDoc_BSONLayout pins the persisted document shape: flat (no nested
// "base"), with the field names existing data already uses. It replaces the
// old model-level BSON-inline test now that bson tags live only here.
func TestUserDoc_BSONLayout(t *testing.T) {
	doc, err := toUserDoc(models.NewUser("ada@example.com", "Ada", "hash123"))
	require.NoError(t, err)
	doc.ID = bson.NewObjectID()

	raw, err := bson.Marshal(doc)
	require.NoError(t, err)
	var m bson.M
	require.NoError(t, bson.Unmarshal(raw, &m))

	assert.ElementsMatch(t,
		[]string{"_id", "email", "name", "password_hash", "created_at", "updated_at"},
		mapKeys(m))
	assert.IsType(t, bson.ObjectID{}, m["_id"])
}

// TestInvalidIDs_NeverReachTheDriver proves the "malformed ID => not found"
// rule without a live Mongo: the repo has a nil collection, so any path that
// touched it would panic.
func TestInvalidIDs_NeverReachTheDriver(t *testing.T) {
	r := &UserRepository{} // nil collection on purpose
	ctx := context.Background()

	_, err := r.GetByID(ctx, "not-an-object-id")
	assert.ErrorIs(t, err, errs.ErrRecordNotFound)

	assert.ErrorIs(t, r.Delete(ctx, "not-an-object-id"), errs.ErrRecordNotFound)
	assert.ErrorIs(t, r.Delete(ctx, ""), errs.ErrRecordNotFound)

	u := models.NewUser("a@b.c", "A", "h")
	u.ID = "not-an-object-id"
	assert.ErrorIs(t, r.Update(ctx, u), errs.ErrRecordNotFound)
	u.ID = ""
	assert.ErrorIs(t, r.Update(ctx, u), errs.ErrRecordNotFound, "never-persisted user cannot be updated")

	u.ID = "not-an-object-id"
	err = r.Create(ctx, u)
	var appErr *errs.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 400, appErr.StatusCode, "supplying a malformed ID on Create is a caller bug")
}

func mapKeys(m bson.M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

Wire it into the aggregate — the one function that names a concrete driver.

**`internal/repo/repositories.go (with User added)`**

```go
// Package repo is the data-access layer's DRIVER-AGNOSTIC surface: the
// repository interfaces (user.go) and the Repositories aggregate with its
// wiring (this file). It imports no database driver. Each database has its
// own implementation package underneath:
//
//	internal/repo/mongo      MongoDB (active)
//	internal/repo/postgres   not implemented — see below
//
// The repo layer (plus internal/db, the connection/error-mapping layer) is
// the ONLY database-specific code in the project. models, services and
// handlers see models.ID strings and the interfaces here; internal/archtest
// fails the test run if any other package imports the Mongo driver.
//
// # Adding a Postgres or SQLite implementation
//
//  1. Create internal/repo/<driver> (e.g. internal/repo/postgres) with a
//     UserRepository type implementing the interface in user.go. Give it a
//     private row/document struct (db tags live there, never on models) and
//     explicit to/from models.User conversion — copy the shape of
//     internal/repo/mongo/user.go.
//  2. Own the ID format in that package: generate it in Create, parse it in
//     GetByID/Update/Delete, and return errs.ErrRecordNotFound for an ID that
//     is malformed for that store.
//  3. Add the driver and its connection (internal/db/postgres.go is sketched)
//     and map its errors onto the errs sentinels next to db.MapDBError.
//  4. In NewRepositories below, construct the new implementation instead of
//     Mongo's, and add a `var _ UserRepository = (*postgres.UserRepository)(nil)`
//     assertion. Services, handlers and models do not change.
//  5. Extend the allow-list in internal/archtest for the new driver's import
//     path so the same isolation rule protects it.
package repo

import (
	"context"

	mongorepo "github.com/hritul2/goboiler/internal/repo/mongo"
)

// Compile-time proof the active implementation satisfies the contract.
var _ UserRepository = (*mongorepo.UserRepository)(nil)

// Repositories aggregates every domain repository for injection into the
// service layer (CODING_SEQUENCE steps 15–16).
type Repositories struct {
	User UserRepository
}

// NewRepositories picks the database implementation (today: Mongo),
// constructs every repository and runs its EnsureIndexes — fail-fast at
// wiring time rather than on first insert. Requires db.Init to have run.
// This function is the single place that names a concrete driver package.
//
// Not unit-tested: it needs a live database (covered by the compose
// integration check).
func NewRepositories(ctx context.Context) (*Repositories, error) {
	users := mongorepo.NewUserRepository()
	if err := users.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	return &Repositories{User: users}, nil
}
```

### Where Postgres or SQLite would go

**Sketch, not compiled.** A second implementation is a new package next to
`mongo`, implementing the same interface, with its own row struct and its own ID
rule; then you switch the constructor in `NewRepositories` and add the driver to
the rules in `internal/archtest`. Nothing above the repo layer changes.

```go
// internal/repo/postgres/user.go — sketch, not compiled
package postgres

type userRow struct { // db tags live HERE, never on models.User
	ID        string    `db:"id"` // e.g. a UUID
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	Password  string    `db:"password_hash"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type UserRepository struct{ pool *pgxpool.Pool }

func (r *UserRepository) GetByID(ctx context.Context, id models.ID) (*models.User, error) {
	if _, err := uuid.Parse(string(id)); err != nil {
		return nil, errs.ErrRecordNotFound // malformed for this store => cannot exist
	}
	// SELECT ... WHERE id = $1; pgx.ErrNoRows => errs.ErrRecordNotFound
	// (add that mapping next to db.MapDBError; unique_violation => errs.ErrDuplicateKey)
}

// internal/repo/repositories.go, in NewRepositories:
//   users := postgres.NewUserRepository()
```

---

## Step 3 — Validation input

*Plugs in at:* nothing to edit; `internal/validation/validation.go` is generic.

The service validates its own input with the same `validate` tags the API uses.
The `json` tags make failures come back keyed by the same field names the
client sends (`email`, `password`).

**`internal/validation/user.go`**

```go
package validation

// UserCreateInput is the payload for UserService.Create. Password is capped
// at 72 bytes to guard against bcrypt's silent truncation past that length.
type UserCreateInput struct {
	Email    string `json:"email" validate:"required,email"`
	Name     string `json:"name" validate:"required,min=2,max=100"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// UserUpdateInput is the payload for UserService.Update.
type UserUpdateInput struct {
	Name string `json:"name" validate:"required,min=2,max=100"`
}
```

**`internal/validation/user_test.go`**

```go
package validation

import "testing"

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserCreateSchema(t *testing.T) {
	tests := []struct {
		name      string
		in        UserCreateInput
		wantField string // empty means no error expected
	}{
		{"valid", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: "password123"}, ""},
		{"bad_email", UserCreateInput{Email: "not-an-email", Name: "Ada", Password: "password123"}, "email"},
		{"missing_email", UserCreateInput{Email: "", Name: "Ada", Password: "password123"}, "email"},
		{"name_too_short", UserCreateInput{Email: "ada@example.com", Name: "A", Password: "password123"}, "name"},
		{"missing_name", UserCreateInput{Email: "ada@example.com", Name: "", Password: "password123"}, "name"},
		{"password_too_short", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: "short"}, "password"},
		{"password_too_long", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: stringOfLen(73)}, "password"},
		{"password_max_ok", UserCreateInput{Email: "ada@example.com", Name: "Ada", Password: stringOfLen(72)}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(&tt.in)
			if tt.wantField == "" {
				assert.False(t, errs.HasErrors(), "expected no errors, got %v", errs)
				return
			}
			require.True(t, errs.HasErrors())
			assert.Contains(t, errs, tt.wantField)
		})
	}
}

func TestUserUpdateSchema(t *testing.T) {
	tests := []struct {
		name      string
		in        UserUpdateInput
		wantField string
	}{
		{"valid", UserUpdateInput{Name: "Ada"}, ""},
		{"too_short", UserUpdateInput{Name: "A"}, "name"},
		{"missing", UserUpdateInput{Name: ""}, "name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(&tt.in)
			if tt.wantField == "" {
				assert.False(t, errs.HasErrors())
				return
			}
			require.True(t, errs.HasErrors())
			assert.Contains(t, errs, tt.wantField)
		})
	}
}

func stringOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
```

The validator itself (`internal/validation/validation.go`) is part of the
skeleton: it keys errors by JSON field path (`address.city`, `lines[1].sku`) and
words `min`/`max` by kind ("characters" for strings, "items" for slices, a bare
number for numerics).

---

## Step 4 — Service

*Plugs in at:* nothing to edit yet; wiring is step 6.

The service owns the business rules, depends on the repository **interface** and
a narrow `Enqueuer` interface (so it is testable without a database or Redis),
and wraps every public method in a trace span. Logging policy: mutations log
success and failure; reads log errors only; a failed login is a warning, not an
error.

What is worth noticing:

- **Errors.** Invalid input returns `errs.Validation(verrs)`: HTTP 400 with the
  per-field `fields` map. A duplicate email becomes `errs.Conflict` (409). Not
  found comes up from the repo as 404. Anything unexpected stays an
  `*errs.AppError` or is wrapped as `INTERNAL` (500, cause logged, never sent).
- **Password hashing.** bcrypt with the default cost; the input is capped at 72
  bytes because bcrypt silently truncates beyond that. Unknown email and wrong
  password return the identical error so emails cannot be enumerated.
- **Welcome email.** After the user is saved, `enqueueWelcomeEmail` builds a task
  and enqueues it **best effort**: if the queue hiccups, sign-up still succeeds
  and the failure is logged. The job itself is step 5.

**`internal/services/user/user.go`**

```go
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
		err := errs.Validation(verrs)
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

	s.log.Info().Str("user_id", u.ID.String()).Str("email", u.Email).Msg("user created")
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
func (s *UserService) GetByID(ctx context.Context, id models.ID) (*models.User, error) {
	ctx, span := tracer.Start(ctx, "UserService.GetByID")
	defer span.End()

	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.String()).Msg("get user failed")
		return nil, err
	}
	return u, nil
}

// Update validates input and applies it to the user identified by id.
func (s *UserService) Update(ctx context.Context, id models.ID, in validation.UserUpdateInput) (*models.User, error) {
	ctx, span := tracer.Start(ctx, "UserService.Update")
	defer span.End()

	if verrs := validation.Validate(&in); verrs.HasErrors() {
		err := errs.Validation(verrs)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.String()).Msg("update user failed: lookup")
		return nil, err
	}

	u.Name = in.Name
	if err := s.users.Update(ctx, u); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.String()).Msg("update user failed")
		return nil, err
	}

	s.log.Info().Str("user_id", id.String()).Msg("user updated")
	return u, nil
}

// Delete removes a user by ID.
func (s *UserService) Delete(ctx context.Context, id models.ID) error {
	ctx, span := tracer.Start(ctx, "UserService.Delete")
	defer span.End()

	if err := s.users.Delete(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error().Err(err).Str("user_id", id.String()).Msg("delete user failed")
		return err
	}

	s.log.Info().Str("user_id", id.String()).Msg("user deleted")
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
```

Test: a fake repository and a fake enqueuer, so every rule runs in memory.

**`internal/services/user/user_test.go`**

```go
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

func newTestService() (*UserService, *fakeUserRepo, *fakeEnqueuer) {
	repo := newFakeUserRepo()
	enq := &fakeEnqueuer{}
	return NewUserService(repo, enq, zerolog.Nop()), repo, enq
}

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

func TestUserService_Create_DuplicateEmail(t *testing.T) {
	svc, repo, _ := newTestService()
	repo.createErr = errs.ErrDuplicateKey

	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeConflict)
}

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
```

A second test checks the span the service emits (it installs an in-memory trace
exporter):

**`internal/services/user/tracing_test.go`**

```go
package user

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hritul2/goboiler/internal/validation"
)

func TestUserService_Create_RecordsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	svc := NewUserService(newFakeUserRepo(), &fakeEnqueuer{}, zerolog.Nop())

	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "UserService.Create", spans[0].Name)
}
```

---

## Step 5 — The welcome-email job

*Plugs in at:* the `// ADD YOUR DOMAIN` comment in `NewJobRouter`
(`internal/services/registerasynctasks.go`).

Background jobs use asynq over Redis. The `asyncprocessor` package knows nothing
about any job; each domain owns its task type, payload, a **producer**
(`New...Task`) and a **consumer factory** (`New...Handler`). The composition
root registers consumers on a router before the worker starts. The email
package keeps the sender (`client.go`, `smtp.go`, which stay in the skeleton: a
disabled config gives a logging no-op sender so local development needs no SMTP)
and the welcome pieces below.

The typed send function and its HTML template (embedded, `html/template` so
values are escaped):

**`internal/lib/email/emails.go`**

```go
package email

import "context"

// Typed send functions — one per email the application sends. Services call
// these rather than constructing Email values themselves.

// SendWelcome sends the post-signup welcome email.
func SendWelcome(ctx context.Context, s Sender, to, name string) error {
	html, err := renderTemplate("welcome.html", welcomeData{Name: name})
	if err != nil {
		return err
	}

	return s.Send(ctx, Email{
		To:      []string{to},
		Subject: "Welcome aboard, " + name + "!",
		HTML:    html,
	})
}
```

**`internal/lib/email/templates.go`**

```go
package email

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

// html/template (not text/template) so user-supplied values are escaped.
var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// welcomeData feeds welcome.html.
type welcomeData struct {
	Name string
}

// renderTemplate executes the named embedded template with data.
func renderTemplate(name string, data any) (string, error) {
	tpl := templates.Lookup(name)
	if tpl == nil {
		return "", fmt.Errorf("email: unknown template %q", name)
	}

	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("email: render %q: %w", name, err)
	}
	return b.String(), nil
}
```

**`internal/lib/email/templates/welcome.html`**

```html
<div style="font-family: -apple-system, 'Segoe UI', Roboto, sans-serif; max-width: 560px; margin: 0 auto; padding: 24px;">
  <h1 style="font-size: 22px; color: #1a1a2e;">Welcome, {{.Name}}!</h1>
  <p style="font-size: 15px; line-height: 1.6; color: #444;">
    Your account has been created. We're glad to have you on board.
  </p>
  <p style="font-size: 13px; color: #999; margin-top: 32px;">
    If you didn't sign up for this account, you can safely ignore this email.
  </p>
</div>
```

The task: type constant, payload, producer with retry/queue/timeout options, the
consumer factory, and the `email.sent` / `email.failed` counters.

**`internal/lib/email/tasks.go`**

```go
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
```

Tests: the producer is pure; the consumer is called with a fake `Sender`, so no
Redis or SMTP is involved. The fake lived in the skeleton's
`internal/lib/email/client_test.go` and was removed with the example; put it back
in that file (or in any `_test.go` of the package):

**`internal/lib/email/client_test.go (the fake used by the tests below)`**

```go
// fakeSender captures the last Email for assertion in emails_test.go and
// tasks_test.go.
type fakeSender struct {
	last Email
	sent bool
}

func (f *fakeSender) Send(_ context.Context, e Email) error {
	f.last = e
	f.sent = true
	return nil
}
```

**`internal/lib/email/emails_test.go`**

```go
package email

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	th "github.com/hritul2/goboiler/internal/testing"
)

func TestSendWelcome(t *testing.T) {
	fake := &fakeSender{}

	err := SendWelcome(context.Background(), fake, "ada@example.com", "Ada")
	require.NoError(t, err)

	assert.Equal(t, []string{"ada@example.com"}, fake.last.To)
	assert.Contains(t, fake.last.Subject, "Welcome")
	th.AssertStringContains(t, fake.last.HTML, "Ada")
}
```

**`internal/lib/email/templates_test.go`**

```go
package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	th "github.com/hritul2/goboiler/internal/testing"
)

func TestRenderTemplate_Welcome(t *testing.T) {
	out, err := renderTemplate("welcome.html", welcomeData{Name: "Ada"})
	require.NoError(t, err)
	th.AssertStringContains(t, out, "Ada", "Welcome")
}

func TestRenderTemplate_EscapesHTML(t *testing.T) {
	out, err := renderTemplate("welcome.html", welcomeData{Name: "<script>alert(1)</script>"})
	require.NoError(t, err)
	assert.NotContains(t, out, "<script>", "html/template must escape user input")
	assert.Contains(t, out, "&lt;script&gt;")
}

func TestRenderTemplate_UnknownName(t *testing.T) {
	_, err := renderTemplate("nope.html", nil)
	require.Error(t, err)
}
```

**`internal/lib/email/tasks_test.go`**

```go
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
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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

func collectMetricNames(t *testing.T, reader *sdkmetric.ManualReader) map[string]bool {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	names := make(map[string]bool)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names[m.Name] = true
		}
	}
	return names
}

// TestNewWelcomeEmailHandler_RecordsMetrics exercises both the success and
// failure paths against a SINGLE registered MeterProvider/reader. The OTel
// Go SDK's global package-level instruments (created once at package init)
// bind to the first real MeterProvider ever set via otel.SetMeterProvider;
// a later, separate SetMeterProvider call in a different test does not
// rebind them — so this must be one test, not two, to reliably observe both
// counters.
func TestNewWelcomeEmailHandler_RecordsMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	sender := &fakeSender{}
	handler := NewWelcomeEmailHandler(sender, zerolog.Nop())

	task, err := NewWelcomeEmailTask("ada@example.com", "Ada")
	require.NoError(t, err)
	require.NoError(t, handler(context.Background(), task))

	badTask := asynq.NewTask(TaskWelcomeEmail, []byte("not json"))
	require.Error(t, handler(context.Background(), badTask))

	names := collectMetricNames(t, reader)
	assert.True(t, names["email.sent"], "email.sent counter must be recorded")
	assert.True(t, names["email.failed"], "email.failed counter must be recorded")
}
```

Register the consumer. This is the single place "which job kinds exist" is
decided:

**`internal/services/registerasynctasks.go (with the welcome job registered)`**

```go
package services

import (
	"fmt"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/lib/email"
	"github.com/hritul2/goboiler/internal/services/asyncprocessor"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// NewJobRouter builds every dependency a task handler needs and registers
// each domain's handlers against a fresh asyncprocessor.Router. This is the
// single place "which job kinds exist" is decided — adding a new one means
// adding its dependency setup and one router.Handle call here, nothing in
// cmd/main.go or internal/services/asyncprocessor itself.
func NewJobRouter(cfg *config.Config, log zerolog.Logger) (*asyncprocessor.Router, error) {
	sender, err := email.New(&cfg.Email, log)
	if err != nil {
		return nil, fmt.Errorf("job router: email sender: %w", err)
	}

	router := asyncprocessor.NewRouter()
	router.Handle(email.TaskWelcomeEmail, email.NewWelcomeEmailHandler(sender, log))
	// router.Handle(<domain>.TaskX, <domain>.NewXHandler(...))

	return router, nil
}
```

---

## Step 6 — Service wiring

*Plugs in at:* the `// ADD YOUR DOMAIN` comments on `services.Services` and
`NewServices`.

Add a field per domain service and construct it from the repository aggregate.
The `Enqueuer` is satisfied by `asyncprocessor.Get().Client`, passed in by
`internal/services/server`.

**`internal/services/services.go (with User added)`**

```go
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
```

**`internal/services/services_test.go`**

```go
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
```

---

## Step 7 — DTOs, handler, endpoints

*Plugs in at:* the `// ADD YOUR DOMAIN` comments on `handler.Handlers` and
`handler.New`.

The handler package has three files, none of which touches gin or JSON.
Decoding the body, validating it and rendering the response/error are done by the
`internal/api` adapter.

**DTOs** are the wire contract. Every field has an explicit camelCase `json`
tag (it is the TypeScript property name *and* the key in validation errors);
inputs carry `validate` tags that are enforced on the server and translated to
zod checks by the generator; IDs are plain strings bounded with
`required,max=64` (no database-specific tag); responses are separate types so a
new model field or a password hash can never leak.

**`internal/handler/user/dto.go`**

```go
// Package user exposes the User domain over the typed-RPC layer
// (internal/api): request/response DTOs (dto.go), the typed functions that
// translate between DTOs and the UserService (handler.go), and the list of
// endpoints registered into a version (endpoints.go).
//
// ⚠️ These endpoints are UNAUTHENTICATED until auth lands: anyone who can
// reach the server can create, read, update and delete any user. When auth
// exists, attach it per endpoint with api.WithMiddleware (or version-wide
// via api.Version.Middleware) — do not bake auth into the typed functions.
package user

import "time"

// DTO RULES (enforced by convention and documented in docs/API_HANDLERS.md):
//
//   - Every field has an explicit camelCase `json` tag. The tag IS the wire
//     contract AND the generated zod/TypeScript property name; a missing tag
//     would leak Go's PascalCase field name to the frontend.
//   - Inputs carry `validate` tags. They are enforced server-side by
//     validation.Validate in the api adapter AND translated to zod checks by
//     the generator, so client and server validate identically.
//   - IDs are plain strings (`validate:"required,max=64"`). The wire format
//     says nothing about how the database formats IDs (ObjectID hex, UUID,
//     a Clerk "user_..." string): the DTO only bounds the size, the handler
//     passes the string through as a models.ID, and the repository decides
//     whether it exists — an ID the store cannot parse is simply NOT FOUND.
//   - Response DTOs are separate from models.User on purpose, so a new
//     model field (or PasswordHash) can never reach a client by accident.

// CreateUserReq is the input of createUser. The validate rules mirror
// validation.UserCreateInput (the service re-validates; keeping the two in
// sync means the client gets the same rules the service enforces, one layer
// earlier).
type CreateUserReq struct {
	Email    string `json:"email"    validate:"required,email"`
	Name     string `json:"name"     validate:"required,min=2,max=100"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// GetUserReq is the input of getUser and deleteUser: just the user's ID.
type GetUserReq struct {
	UserID string `json:"userId" validate:"required,max=64"`
}

// ListUsersReq is the input of listUsers. Both fields are optional;
// zero/omitted values fall back to models.DefaultPage / DefaultLimit
// (models.Pagination.Normalize), so `{}` is a valid request.
type ListUsersReq struct {
	Page  int `json:"page,omitempty"  validate:"omitempty,min=1"`
	Limit int `json:"limit,omitempty" validate:"omitempty,min=1,max=100"`
}

// UpdateUserReq is the input of updateUser. Only Name is updatable today
// (see validation.UserUpdateInput).
type UpdateUserReq struct {
	UserID string `json:"userId" validate:"required,max=64"`
	Name   string `json:"name"   validate:"required,min=2,max=100"`
}

// UserRes is the public representation of a user. There is deliberately no
// password-hash field: it cannot be serialised because it does not exist
// here.
type UserRes struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListUsersRes is one page of users plus the effective (normalised) paging
// parameters that produced it. Items carries `validate:"required"` purely
// for the generator: a Go slice can be nil (JSON null), so zen emits
// `.nullable()` for it unless the tag says it is never nil. The handler
// guarantees a non-nil slice, so the TypeScript type is UserRes[] instead of
// UserRes[] | null. (Response structs are never run through the validator.)
type ListUsersRes struct {
	Items []UserRes `json:"items" validate:"required"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}
```

**The handler** is a set of typed functions `func(context.Context, Req) (Res, error)`
over a narrow service interface (a fake satisfies it in tests). IDs are passed
through as `models.ID`; mapping a model to a response is done in exactly one place
(`toUserRes`).

**`internal/handler/user/handler.go`**

```go
package user

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/models"
	"github.com/hritul2/goboiler/internal/validation"
)

// userService is the narrow slice of *usersvc.UserService this package
// needs. Depending on an interface (not the concrete type) keeps the typed
// functions unit-testable with a fake, and keeps this package from caring
// how the service is built. *user.UserService satisfies it structurally.
type userService interface {
	Create(ctx context.Context, in validation.UserCreateInput) (*models.User, error)
	GetByID(ctx context.Context, id models.ID) (*models.User, error)
	Update(ctx context.Context, id models.ID, in validation.UserUpdateInput) (*models.User, error)
	Delete(ctx context.Context, id models.ID) error
	List(ctx context.Context, p models.Pagination) ([]models.User, error)
}

// Handler holds the typed User RPC functions. Each method has the exact
// signature api.Query/api.Mutation expect —
// func(context.Context, Req) (Res, error) — so it knows nothing about gin,
// HTTP or JSON: decoding, validation and error rendering all happen in the
// api adapter.
type Handler struct {
	svc userService
	log zerolog.Logger
}

// NewHandler constructs a Handler. svc is constructor-injected (no global
// lookups), matching HealthHandler and UserService.
func NewHandler(svc userService, log zerolog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// CreateUser registers a new account. Field-level rules were already checked
// by the adapter; the service validates again, hashes the password and maps
// a duplicate email to 409.
func (h *Handler) CreateUser(ctx context.Context, req CreateUserReq) (UserRes, error) {
	u, err := h.svc.Create(ctx, validation.UserCreateInput{
		Email:    req.Email,
		Name:     req.Name,
		Password: req.Password,
	})
	if err != nil {
		return UserRes{}, err
	}
	return toUserRes(u), nil
}

// GetUser returns one user, or a 404 from the service when it doesn't exist.
func (h *Handler) GetUser(ctx context.Context, req GetUserReq) (UserRes, error) {
	u, err := h.svc.GetByID(ctx, models.ID(req.UserID))
	if err != nil {
		return UserRes{}, err
	}
	return toUserRes(u), nil
}

// ListUsers returns one page of users. Page/limit are normalised here (not
// only in the repo) so the response can echo the values actually used.
func (h *Handler) ListUsers(ctx context.Context, req ListUsersReq) (ListUsersRes, error) {
	p := models.Pagination{Page: req.Page, Limit: req.Limit}
	p.Normalize()

	users, err := h.svc.List(ctx, p)
	if err != nil {
		return ListUsersRes{}, err
	}

	// Always a non-nil slice: a nil slice marshals to JSON null, which the
	// generated zod array schema would reject.
	items := make([]UserRes, 0, len(users))
	for i := range users {
		items = append(items, toUserRes(&users[i]))
	}
	return ListUsersRes{Items: items, Page: p.Page, Limit: p.Limit}, nil
}

// UpdateUser renames a user and returns the updated record.
func (h *Handler) UpdateUser(ctx context.Context, req UpdateUserReq) (UserRes, error) {
	u, err := h.svc.Update(ctx, models.ID(req.UserID), validation.UserUpdateInput{Name: req.Name})
	if err != nil {
		return UserRes{}, err
	}
	return toUserRes(u), nil
}

// DeleteUser removes a user. It returns api.Empty (rendered as `{}`) since
// there is nothing useful to send back.
func (h *Handler) DeleteUser(ctx context.Context, req GetUserReq) (api.Empty, error) {
	if err := h.svc.Delete(ctx, models.ID(req.UserID)); err != nil {
		return api.Empty{}, err
	}
	return api.Empty{}, nil
}

// toUserRes is the single place a models.User becomes a wire value. Adding a
// field to the response is a deliberate edit here and in UserRes.
func toUserRes(u *models.User) UserRes {
	return UserRes{
		ID:        u.ID.String(),
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
```

**Endpoints** list the functions, in the order they appear in the generated
client.

- `api.Query` for reads (`getUser`, `listUsers`) — safe to repeat; the generated
  hook is `useQuery`, cached and refetched by TanStack Query.
- `api.Mutation` for writes (`createUser`, `updateUser`, `deleteUser`) — the hook
  is `useMutation`, called imperatively, never cached.
- Both are `POST` with a JSON body; the kind only changes the hook.

**`internal/handler/user/endpoints.go`**

```go
package user

import "github.com/hritul2/goboiler/internal/api"

// Endpoints lists the User domain's RPC functions in the order they appear
// in the generated TypeScript client. The result is spliced into a
// api.Version in internal/router/versions.go; one Handler can feed several
// versions (or be wrapped for a v2 that diverges).
//
// Kinds: reads (getUser, listUsers) are Queries => useQuery hooks; writes
// (createUser, updateUser, deleteUser) are Mutations => useMutation hooks.
//
// ⚠️ UNAUTHENTICATED until auth lands. When it does, add
// api.WithMiddleware(auth) to the endpoints that need it (or set
// api.Version.Middleware for the whole version).
func Endpoints(h *Handler) []api.Endpoint {
	return []api.Endpoint{
		api.Mutation("createUser", h.CreateUser,
			api.WithDescription("Creates a user account. Fails with CONFLICT if the email is already registered.")),
		api.Query("getUser", h.GetUser,
			api.WithDescription("Fetches one user by id. Fails with NOT_FOUND if the user does not exist.")),
		api.Query("listUsers", h.ListUsers,
			api.WithDescription("Lists users one page at a time. page defaults to 1 and limit to 20 (max 100).")),
		api.Mutation("updateUser", h.UpdateUser,
			api.WithDescription("Updates a user's name and returns the updated user.")),
		api.Mutation("deleteUser", h.DeleteUser,
			api.WithDescription("Permanently deletes a user by id.")),
	}
}
```

**How errors reach the client.** A handler returns an `*errs.AppError`
(`errs.NotFound("...")`, `errs.Conflict("...")`, or whatever the service
returned). The adapter calls `c.Error(err)` and never writes a response itself;
`middleware.ErrorHandler` renders it with the status from the error. Anything
that is not an `AppError` becomes `500 INTERNAL` with the cause logged, not sent.
Examples of what the client receives:

```
400  {"code":"BAD_REQUEST","message":"validation failed: email: must be a valid email address; password: must be at least 8 characters",
      "fields":{"email":["must be a valid email address"],"password":["must be at least 8 characters"]}}
404  {"code":"NOT_FOUND","message":"record not found"}          // also for a malformed id (with Mongo)
409  {"code":"CONFLICT","message":"an account with this email already exists"}
500  {"code":"INTERNAL","message":"an internal server error occurred"}
```

Test: mount the **real** `Endpoints(h)` over a fake service in a gin engine, so a
test covers decoding, validation, the handler, error rendering and JSON encoding
together — and also reflects over every DTO to prove each field has a camelCase
`json` tag.

**`internal/handler/user/handler_test.go`**

```go
package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/api"
	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/middleware"
	"github.com/hritul2/goboiler/internal/models"
	usersvc "github.com/hritul2/goboiler/internal/services/user"
	"github.com/hritul2/goboiler/internal/validation"
)

// Compile-time proof the real service satisfies the narrow interface, so the
// fake below can't drift from what production wiring passes in.
var _ userService = (*usersvc.UserService)(nil)

// fakeService records what the handler passed it and returns canned results.
type fakeService struct {
	user *models.User
	err  error

	gotID    models.ID
	gotPage  models.Pagination
	gotName  string
	gotEmail string
	deleted  models.ID
	list     []models.User
}

func (f *fakeService) Create(_ context.Context, in validation.UserCreateInput) (*models.User, error) {
	f.gotEmail = in.Email
	return f.user, f.err
}

func (f *fakeService) GetByID(_ context.Context, id models.ID) (*models.User, error) {
	f.gotID = id
	return f.user, f.err
}

func (f *fakeService) Update(_ context.Context, id models.ID, in validation.UserUpdateInput) (*models.User, error) {
	f.gotID, f.gotName = id, in.Name
	return f.user, f.err
}

func (f *fakeService) Delete(_ context.Context, id models.ID) error {
	f.deleted = id
	return f.err
}

func (f *fakeService) List(_ context.Context, p models.Pagination) ([]models.User, error) {
	f.gotPage = p
	return f.list, f.err
}

func newTestUser() *models.User {
	u := models.NewUser("ada@example.com", "Ada", "$2a$10$SUPERSECRETHASH")
	u.ID = "user-1" // any string: the handler is ID-format agnostic
	u.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	u.UpdatedAt = u.CreatedAt
	return u
}

// newEngine mounts the real Endpoints(h) over the fake service, so these
// tests cover DTO decoding, validation, handler logic and JSON encoding.
func newEngine(svc userService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	api.Mount(engine, api.Version{
		Name:      "v1",
		Prefix:    "/api/v1/rpc",
		Endpoints: Endpoints(NewHandler(svc, zerolog.Nop())),
	})
	return engine
}

func call(engine *gin.Engine, fn, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/rpc/"+fn, strings.NewReader(body)))
	return w
}

func TestEndpoints_Registered(t *testing.T) {
	eps := Endpoints(NewHandler(&fakeService{}, zerolog.Nop()))

	kinds := map[string]api.Kind{}
	for _, e := range eps {
		kinds[e.Name] = e.Kind
	}
	assert.Equal(t, map[string]api.Kind{
		"createUser": api.KindMutation,
		"updateUser": api.KindMutation,
		"deleteUser": api.KindMutation,
		"getUser":    api.KindQuery,
		"listUsers":  api.KindQuery,
	}, kinds)

	v := api.Version{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: eps}
	assert.NoError(t, v.Validate())
}

func TestDTOs_EveryFieldHasCamelCaseJSONTag(t *testing.T) {
	// Reflect over every registered Req/Res: a missing or PascalCase tag
	// would leak a Go field name into the generated TypeScript.
	for _, e := range Endpoints(NewHandler(&fakeService{}, zerolog.Nop())) {
		for _, typ := range []reflect.Type{e.Req, e.Res} {
			for i := range typ.NumField() {
				f := typ.Field(i)
				name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				assert.Regexp(t, `^[a-z][a-zA-Z0-9]*$`, name, "%s.%s needs a camelCase json tag", typ.Name(), f.Name)
			}
		}
	}
}

func TestCreateUser_OK_NoPasswordHashInResponse(t *testing.T) {
	svc := &fakeService{user: newTestUser()}
	engine := newEngine(svc)

	w := call(engine, "createUser", `{"email":"ada@example.com","name":"Ada","password":"correct-horse"}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "ada@example.com", svc.gotEmail)

	body := w.Body.String()
	assert.NotContains(t, strings.ToLower(body), "password")
	assert.NotContains(t, body, "SUPERSECRETHASH")

	var res map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.ElementsMatch(t, []string{"id", "email", "name", "createdAt", "updatedAt"}, keys(res))
	assert.Equal(t, "ada@example.com", res["email"])
	assert.Equal(t, svc.user.ID.String(), res["id"])
	assert.Equal(t, "2026-01-02T03:04:05Z", res["createdAt"])
}

func TestCreateUser_ValidationFailure_400(t *testing.T) {
	w := call(newEngine(&fakeService{}), "createUser", `{"email":"nope","name":"A","password":"short"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var body errs.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, errs.CodeBadRequest, body.Code)
	// Keyed by the DTO's JSON names, so a form can map them to inputs.
	assert.Equal(t, map[string][]string{
		"email":    {"must be a valid email address"},
		"name":     {"must be at least 2 characters"},
		"password": {"must be at least 8 characters"},
	}, body.Fields)
}

func TestCreateUser_ServiceConflict_409(t *testing.T) {
	svc := &fakeService{err: errs.Conflict("an account with this email already exists")}

	w := call(newEngine(svc), "createUser", `{"email":"ada@example.com","name":"Ada","password":"correct-horse"}`)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestGetUser_InvalidUserID_400(t *testing.T) {
	svc := &fakeService{user: newTestUser()}
	engine := newEngine(svc)

	// Only what the DTO can judge without knowing the database: presence and
	// size (`required,max=64`).
	for name, body := range map[string]string{
		"missing":      `{}`,
		"empty string": `{"userId":""}`,
		"too long":     `{"userId":"` + strings.Repeat("a", 65) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := call(engine, "getUser", body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), errs.CodeBadRequest)
			assert.Contains(t, w.Body.String(), `"userId"`, "error is keyed by the JSON field name")
		})
	}
	assert.True(t, svc.gotID.IsZero(), "service must not be reached with a bad id")
}

// TestGetUser_UnparseableID_PassedThrough documents the DB-agnostic contract:
// the handler no longer judges ID FORMAT. An ID the store cannot parse (not
// an ObjectID, not a UUID, ...) is handed to the service and comes back as
// the store's NOT FOUND.
func TestGetUser_UnparseableID_PassedThrough(t *testing.T) {
	svc := &fakeService{err: errs.ErrRecordNotFound}

	w := call(newEngine(svc), "getUser", `{"userId":"not-an-object-id"}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, models.ID("not-an-object-id"), svc.gotID)
}

func TestGetUser_OK_And_NotFound(t *testing.T) {
	u := newTestUser()
	svc := &fakeService{user: u}
	engine := newEngine(svc)

	w := call(engine, "getUser", `{"userId":"`+u.ID.String()+`"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, u.ID, svc.gotID)
	assert.NotContains(t, w.Body.String(), "SUPERSECRETHASH")

	svc.err = errs.NotFound("user not found")
	w = call(engine, "getUser", `{"userId":"`+u.ID.String()+`"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateUser_OK(t *testing.T) {
	u := newTestUser()
	svc := &fakeService{user: u}

	w := call(newEngine(svc), "updateUser", `{"userId":"`+u.ID.String()+`","name":"Ada L"}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Ada L", svc.gotName)
	assert.Equal(t, u.ID, svc.gotID)
}

func TestDeleteUser_OK_ReturnsEmptyObject(t *testing.T) {
	id := models.ID("user-2")
	svc := &fakeService{}

	w := call(newEngine(svc), "deleteUser", `{"userId":"`+string(id)+`"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{}`, w.Body.String())
	assert.Equal(t, id, svc.deleted)
}

func TestDeleteUser_MissingID_400(t *testing.T) {
	svc := &fakeService{}

	w := call(newEngine(svc), "deleteUser", `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.True(t, svc.deleted.IsZero())
}

func TestListUsers_DefaultsAndItemsNeverNull(t *testing.T) {
	svc := &fakeService{list: nil}

	w := call(newEngine(svc), "listUsers", ``)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"items":[],"page":1,"limit":20}`, w.Body.String())
	assert.Equal(t, models.Pagination{Page: 1, Limit: 20}, svc.gotPage)
}

func TestListUsers_PassesPagingAndMapsItems(t *testing.T) {
	a, b := newTestUser(), newTestUser()
	svc := &fakeService{list: []models.User{*a, *b}}

	w := call(newEngine(svc), "listUsers", `{"page":2,"limit":50}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, models.Pagination{Page: 2, Limit: 50}, svc.gotPage)

	var res ListUsersRes
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Items, 2)
	assert.Equal(t, 2, res.Page)
	assert.Equal(t, 50, res.Limit)
	assert.NotContains(t, w.Body.String(), "SUPERSECRETHASH")
}

func TestListUsers_LimitOverMax_400(t *testing.T) {
	w := call(newEngine(&fakeService{}), "listUsers", `{"limit":1000}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

Add the handler to the aggregate, built from the service:

**`internal/handler/handlers.go (with User added)`**

```go
package handler

import (
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/handler/user"
	"github.com/hritul2/goboiler/internal/services"
)

// Handlers aggregates every HTTP handler. Mirrors services.Services's
// aggregation shape.
type Handlers struct {
	Health *HealthHandler
	User   *user.Handler
}

// New constructs every handler. db/redis are Pingers (db.Database and
// *redis.Redis both satisfy this structurally) — passed through from
// internal/services/server, which already holds both via db.Get()/redis.Get().
// svcs supplies the domain services the RPC handlers wrap.
//
// New does not call anything on its arguments, so cmd/gen can pass nil
// Pingers and an empty &services.Services{} to obtain the endpoint list
// (types only) without a database or Redis.
func New(db, redis Pinger, svcs *services.Services, log zerolog.Logger) *Handlers {
	return &Handlers{
		Health: NewHealthHandler(db, redis, log),
		User:   user.NewHandler(svcs.User, log),
	}
}
```

---

## Step 8 — Register the feature in an API version

*Plugs in at:* the `// ADD YOUR DOMAIN` comment on `Endpoints` in
`internal/router/versions.go`.

`APIVersions` is the single list both the router and the TypeScript generator
read, so the served API and the generated client cannot drift. `api.Mount`
validates the version at boot and **panics** on a malformed declaration (bad
name, duplicate name, non-struct request type), which is better than serving a
half-registered API.

**`internal/router/versions.go (with User added)`**

```go
package router

import (
	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/handler"
	"github.com/hritul2/goboiler/internal/handler/user"
)

// APIVersions is the single registry of RPC API versions. It feeds BOTH the
// router (router.New mounts each entry as its own gin group) and the
// TypeScript generator (cmd/gen writes each entry to gen/ts/<Name>/), so the
// served API and the generated client cannot drift.
//
// To add a function: append its endpoint here (usually via a domain's
// Endpoints(h) helper). To add /api/v2: append a second api.Version with
// Name "v2" and Prefix "/api/v2/rpc". Versions are independent — each gets
// its own route group, its own middleware and its own generated folder — so
// v2 can reuse v1's handlers, wrap them, or diverge entirely without
// touching v1.
//
// Nothing here knows about webhooks: those are plain gin routes under
// /webhook/<provider> (see router.New) and are never generated.
func APIVersions(h *handler.Handlers) []api.Version {
	return []api.Version{
		{
			Name:      "v1",
			Prefix:    "/api/v1/rpc",
			Endpoints: user.Endpoints(h.User),
		},
	}
}
```

The router tests mount the real versions, so a bad declaration fails
`go test ./internal/router` before it can fail a deploy.

---

## Step 9 — Generate the client and use it from a frontend

```sh
make gen                        # fetch client  -> gen/ts/v1/{schemas,client,hooks,index}.ts
make gen-axios                  # axios client  -> same files; only client.ts differs
```

Commit `gen/ts` (CI's `make gen-check` fails if it is stale) or send it to your
frontend however you decide — see `HOW_TO_USE_IT.md` §3. The generated code
needs `zod@^4`, `@tanstack/react-query@^5` if you import `hooks.ts`, and `axios@^1`
for the axios client.

What was generated for this feature. The zod schemas mirror the Go DTOs and their
`validate` tags (the server stays authoritative):

**`gen/ts/v1/schemas.ts (excerpt)`**

```ts
export const CreateUserReqSchema = z.object({
  email: z.string().min(1).check(z.email()),
  name: z.string().min(1).refine((val) => [...val].length >= 2, 'String must contain at least 2 character(s)').refine((val) => [...val].length <= 100, 'String must contain at most 100 character(s)'),
  password: z.string().min(1).refine((val) => [...val].length >= 8, 'String must contain at least 8 character(s)').refine((val) => [...val].length <= 72, 'String must contain at most 72 character(s)'),
})
export type CreateUserReq = z.infer<typeof CreateUserReqSchema>

export const UserResSchema = z.object({
  id: z.string(),
  email: z.string(),
  name: z.string(),
  createdAt: z.coerce.date(),
  updatedAt: z.coerce.date(),
})
export type UserRes = z.infer<typeof UserResSchema>
```

One typed function per endpoint (fetch client shown; the axios client has the same
function signatures):

**`gen/ts/v1/client.ts (excerpt)`**

```ts
    createUser: (input: CreateUserReq): Promise<UserRes> =>
      call("/api/v1/rpc/createUser", input, UserResSchema),
```

And a hook per endpoint, `useQuery` or `useMutation` according to step 7:

**`gen/ts/v1/hooks.ts (excerpt)`**

```ts
    useCreateUser: (options?: Omit<UseMutationOptions<UserRes, ApiError, CreateUserReq>, "mutationFn">) =>
      useMutation({ mutationFn: (input: CreateUserReq) => client.createUser(input), ...options }),
```

### Using the fetch client

**`src/api.ts (fetch client)`**

```ts
import { createClient, ApiError } from "./gen/ts/v1"

export const client = createClient({
  baseUrl: "https://api.example.com",
  headers: () => ({ Authorization: `Bearer ${localStorage.getItem("token") ?? ""}` }), // optional
})

export async function signUp(email: string, name: string, password: string) {
  try {
    const user = await client.createUser({ email, name, password }) // typed in, zod-parsed out
    return user.createdAt // a real Date
  } catch (e) {
    if (e instanceof ApiError && e.code === "CONFLICT") {
      return "that email is already registered" // e.status === 409
    }
    throw e
  }
}
```

### Using the axios client (`make gen-axios`)

Pass your own axios instance so its interceptors (auth tokens, retries) apply, or
just a `baseURL`. HTTP errors become an `ApiError` from the response body;
"no response" (offline, DNS, CORS, timeout) becomes `ApiError` with `status` 0 and
`code` `"NETWORK_ERROR"`.

**`src/api.ts (axios client)`**

```ts
import axios from "axios"
import { createClient, ApiError } from "./gen/ts/v1"

// Your own instance: interceptors (auth, retries, logging) keep working.
const http = axios.create({ baseURL: "https://api.example.com" })
http.interceptors.request.use((cfg) => {
  cfg.headers.set("Authorization", `Bearer ${localStorage.getItem("token") ?? ""}`)
  return cfg
})

export const client = createClient({ axios: http })
// or, without an instance of your own: createClient({ baseURL: "https://api.example.com" })

export async function signUp(email: string, name: string, password: string) {
  try {
    const user = await client.createUser({ email, name, password })
    return user.createdAt
  } catch (e) {
    if (e instanceof ApiError && e.status === 0) {
      return "network error, try again" // code "NETWORK_ERROR": no response at all
    }
    if (e instanceof ApiError && e.code === "CONFLICT") {
      return "that email is already registered"
    }
    throw e
  }
}
```

### React: hooks and showing `fields` errors on a form

`hooks.ts` is identical for both clients. A validation failure arrives as an
`ApiError` whose `fields` maps each JSON field name to its messages, so a form can
put every message next to the right input:

**`src/SignupForm.tsx`**

```tsx
import { useState } from "react"
import { createHooks } from "./gen/ts/v1"
import { client } from "./api" // the createClient(...) from the fetch or axios snippet above

// The same component works with either generated client: hooks.ts is identical.
export const { useCreateUser } = createHooks(client)

const fields = ["email", "name", "password"] as const

export function SignupForm() {
  const [values, setValues] = useState({ email: "", name: "", password: "" })
  // Server-side validation messages, keyed by JSON field name: { email: ["must be a valid email address"] }
  const [errors, setErrors] = useState<Record<string, string[]>>({})

  const createUser = useCreateUser({
    onSuccess: () => setErrors({}),
    onError: (e) => setErrors(e.fields ?? {}), // e is an ApiError; `fields` only exists on validation failures
  })

  return (
    <form
      onSubmit={(ev) => {
        ev.preventDefault()
        createUser.mutate(values)
      }}
    >
      {fields.map((field) => (
        <label key={field}>
          {field}
          <input
            name={field}
            type={field === "password" ? "password" : "text"}
            value={values[field]}
            onChange={(ev) => setValues({ ...values, [field]: ev.target.value })}
          />
          {errors[field]?.map((message) => (
            <p key={message} role="alert">
              {message}
            </p>
          ))}
        </label>
      ))}
      {/* Errors that are not about a field (CONFLICT, NETWORK_ERROR, 500, ...) */}
      {createUser.error && !createUser.error.fields && <p role="alert">{createUser.error.message}</p>}
      <button disabled={createUser.isPending}>Sign up</button>
    </form>
  )
}
```

Both clients and this form were type-checked (strict, `exactOptionalPropertyTypes`,
`noUnusedLocals`) against the output generated from the code in this page. The
same `CreateUserReqSchema` can also pre-validate on the client
(`CreateUserReqSchema.safeParse(values)`) before any request is made.

---

## Step 10 — A Clerk webhook that syncs users

*Plugs in at:* the `// ADD YOUR DOMAIN` comment on the `/webhook` group in
`internal/router/router.go`.

**Sketch.** This is not part of the repository; it was compiled and its tests were
run once in a scratch copy when this page was written. It is a webhook example —
it does not add authentication.

A webhook is not an RPC function and is not generated, because: the provider
(Clerk, delivering through Svix) calls *you*; the signature is computed over the
**raw request body**, so the adapter's decode-first path cannot be used; and the
payload is Clerk's, not yours. So it is a plain `gin.HandlerFunc` in the reserved
`/webhook` group. What it must do:

1. **Read the raw body** with `c.GetRawData()` (an oversized body surfaces as a
   `*http.MaxBytesError`, which `ErrorHandler` turns into 413).
2. **Verify the Svix signature** before parsing: HMAC-SHA256 over
   `"<svix-id>.<svix-timestamp>.<body>"` with the base64-decoded `whsec_...`
   secret, compared in constant time against each `v1,<base64>` entry in the
   `svix-signature` header (there can be several during secret rotation).
3. **Reject replays**: refuse a `svix-timestamp` more than a few minutes from
   your clock.
4. **Be idempotent.** Svix delivers at least once, so the sync must be an upsert
   keyed by the Clerk user ID (for this, add an `ExternalID` field to the model, a
   unique index and a repository `UpsertByExternalID`). If you also need to
   suppress duplicate side effects, remember `svix-id` for a while (for example a
   Redis `SET NX` with a TTL).
5. **Answer fast and with the right status**: 2xx when handled (and for event types
   you ignore, so they are not retried forever), non-2xx when you want Svix to
   retry. Do slow work by enqueuing a job as in step 5.

**`internal/webhook/clerk/handler.go (sketch)`**

```go
// Package clerk receives Clerk webhooks (delivered by Svix) and syncs users
// into our own store. Webhooks are NOT RPC functions: Clerk calls us, the
// signature is computed over the raw body, and the payload is Clerk's.
package clerk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	errs "github.com/hritul2/goboiler/internal/errors"
)

// maxSkew is how far the Svix timestamp may be from our clock. It bounds how
// long a captured request stays replayable.
const maxSkew = 5 * time.Minute

var errBadSignature = errors.New("clerk webhook: invalid signature")

// userSyncer is the narrow service interface the handler needs. Make the
// implementation idempotent (an upsert keyed by the Clerk user ID), because
// Svix delivers at least once: a retry or duplicate must not create a second
// user or fail.
type userSyncer interface {
	SyncFromClerk(ctx context.Context, clerkID, email, name string) error
}

// Handler serves POST /webhook/clerk.
type Handler struct {
	key []byte // decoded signing secret
	svc userSyncer
	now func() time.Time
}

// NewHandler takes the endpoint's signing secret as shown in the Clerk
// dashboard ("whsec_..."), which should come from config, not source.
func NewHandler(signingSecret string, svc userSyncer) (*Handler, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(signingSecret, "whsec_"))
	if err != nil || len(key) == 0 {
		return nil, fmt.Errorf("clerk webhook: signing secret is not a valid whsec_ value")
	}
	return &Handler{key: key, svc: svc, now: time.Now}, nil
}

// event is the envelope; Data is decoded per type.
type event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type userCreated struct {
	ID                    string `json:"id"`
	FirstName             string `json:"first_name"`
	LastName              string `json:"last_name"`
	PrimaryEmailAddressID string `json:"primary_email_address_id"`
	EmailAddresses        []struct {
		ID           string `json:"id"`
		EmailAddress string `json:"email_address"`
	} `json:"email_addresses"`
}

// Handle verifies the signature over the RAW body, then dispatches on the
// event type. Any non-2xx makes Svix retry; unknown event types are
// acknowledged with 200 so they are not retried forever.
func (h *Handler) Handle(c *gin.Context) {
	raw, err := c.GetRawData() // body exactly as sent; oversized => *http.MaxBytesError => 413
	if err != nil {
		_ = c.Error(err)
		return
	}
	if err := h.verify(raw, c.Request.Header); err != nil {
		_ = c.Error(errs.Unauthorized("invalid signature"))
		return
	}

	var evt event
	if err := json.Unmarshal(raw, &evt); err != nil {
		_ = c.Error(errs.BadRequest("invalid event"))
		return
	}

	switch evt.Type {
	case "user.created":
		var u userCreated
		if err := json.Unmarshal(evt.Data, &u); err != nil || u.ID == "" {
			_ = c.Error(errs.BadRequest("invalid user.created payload"))
			return
		}
		email := ""
		for _, e := range u.EmailAddresses {
			if e.ID == u.PrimaryEmailAddressID {
				email = e.EmailAddress
			}
		}
		name := strings.TrimSpace(u.FirstName + " " + u.LastName)
		if err := h.svc.SyncFromClerk(c.Request.Context(), u.ID, email, name); err != nil {
			_ = c.Error(err) // 5xx => Svix retries later
			return
		}
	default:
		// user.updated, user.deleted, ... : add a case when you need it.
	}
	c.Status(http.StatusOK)
}

// verify implements Svix's scheme: HMAC-SHA256 over "<svix-id>.<svix-timestamp>.<body>"
// with the decoded secret, compared in constant time against every "v1,<base64>"
// entry in the space-separated svix-signature header (several can be present
// during secret rotation). The timestamp check rejects replays.
func (h *Handler) verify(body []byte, hdr http.Header) error {
	id, ts, sigs := hdr.Get("svix-id"), hdr.Get("svix-timestamp"), hdr.Get("svix-signature")
	if id == "" || ts == "" || sigs == "" {
		return errBadSignature
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errBadSignature
	}
	if skew := h.now().Sub(time.Unix(sec, 0)); skew > maxSkew || skew < -maxSkew {
		return errBadSignature
	}

	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)

	for _, part := range strings.Fields(sigs) {
		version, sig, ok := strings.Cut(part, ",")
		if !ok || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			continue
		}
		if hmac.Equal(got, want) {
			return nil
		}
	}
	return errBadSignature
}
```

**`internal/webhook/clerk/handler_test.go (sketch)`**

```go
package clerk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/middleware"
)

const testSecret = "whsec_" + "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw" // base64 text; any valid base64 works

type fakeSyncer struct {
	calls []string
	err   error
}

func (f *fakeSyncer) SyncFromClerk(_ context.Context, id, email, name string) error {
	f.calls = append(f.calls, id+"|"+email+"|"+name)
	return f.err
}

// sign produces a valid svix-signature header value for body.
func sign(t *testing.T, id string, ts time.Time, body string) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(testSecret, "whsec_"))
	require.NoError(t, err)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(ts.Unix(), 10) + "." + body))
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func newEngine(t *testing.T, svc userSyncer, now time.Time) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h, err := NewHandler(testSecret, svc)
	require.NoError(t, err)
	h.now = func() time.Time { return now }

	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Group("/webhook").POST("/clerk", middleware.MaxReqResSizeOverride(1<<20), h.Handle)
	return engine
}

func deliver(engine *gin.Engine, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhook/clerk", strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

const userCreatedBody = `{"type":"user.created","data":{"id":"user_2abc","first_name":"Ada","last_name":"Lovelace",` +
	`"primary_email_address_id":"idn_2","email_addresses":[{"id":"idn_1","email_address":"old@example.com"},` +
	`{"id":"idn_2","email_address":"ada@example.com"}]}}`

func TestUserCreated_SyncsUser(t *testing.T) {
	now := time.Now()
	svc := &fakeSyncer{}
	w := deliver(newEngine(t, svc, now), userCreatedBody, map[string]string{
		"svix-id":        "msg_1",
		"svix-timestamp": strconv.FormatInt(now.Unix(), 10),
		"svix-signature": sign(t, "msg_1", now, userCreatedBody),
	})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"user_2abc|ada@example.com|Ada Lovelace"}, svc.calls)
}

func TestSignatureRejections(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	good := sign(t, "msg_1", now, userCreatedBody)

	tests := map[string]struct {
		body string
		hdr  map[string]string
	}{
		"missing headers":    {userCreatedBody, nil},
		"tampered body":      {strings.Replace(userCreatedBody, "Ada", "Eve", 1), map[string]string{"svix-id": "msg_1", "svix-timestamp": ts, "svix-signature": good}},
		"wrong id":           {userCreatedBody, map[string]string{"svix-id": "msg_2", "svix-timestamp": ts, "svix-signature": good}},
		"garbage signature":  {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": ts, "svix-signature": "v1,!!!"}},
		"unknown version":    {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": ts, "svix-signature": strings.Replace(good, "v1,", "v2,", 1)}},
		"replay (too old)":   {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10), "svix-signature": sign(t, "msg_1", now.Add(-10*time.Minute), userCreatedBody)}},
		"from the future":    {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": strconv.FormatInt(now.Add(10*time.Minute).Unix(), 10), "svix-signature": sign(t, "msg_1", now.Add(10*time.Minute), userCreatedBody)}},
		"bad timestamp text": {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": "yesterday", "svix-signature": good}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			svc := &fakeSyncer{}
			w := deliver(newEngine(t, svc, now), tt.body, tt.hdr)
			assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			assert.Empty(t, svc.calls, "an unverified event must never reach the service")
		})
	}
}

func TestRotatedSecret_AnyMatchingSignatureWins(t *testing.T) {
	now := time.Now()
	hdr := map[string]string{
		"svix-id":        "msg_1",
		"svix-timestamp": strconv.FormatInt(now.Unix(), 10),
		"svix-signature": "v1,AAAA " + sign(t, "msg_1", now, userCreatedBody), // old key first, current key second
	}
	w := deliver(newEngine(t, &fakeSyncer{}, now), userCreatedBody, hdr)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestUnknownEventAcknowledged_ServiceErrorRetried(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	other := `{"type":"session.created","data":{}}`

	svc := &fakeSyncer{}
	w := deliver(newEngine(t, svc, now), other, map[string]string{"svix-id": "m", "svix-timestamp": ts, "svix-signature": sign(t, "m", now, other)})
	assert.Equal(t, http.StatusOK, w.Code, "unknown types are 200 so Svix does not retry them")
	assert.Empty(t, svc.calls)

	failing := &fakeSyncer{err: assert.AnError}
	w = deliver(newEngine(t, failing, now), userCreatedBody, map[string]string{"svix-id": "m", "svix-timestamp": ts, "svix-signature": sign(t, "m", now, userCreatedBody)})
	assert.Equal(t, http.StatusInternalServerError, w.Code, "a failed sync is non-2xx so Svix retries")
}

func TestNewHandler_RejectsBadSecret(t *testing.T) {
	_, err := NewHandler("whsec_***not-base64***", &fakeSyncer{})
	assert.Error(t, err)
}
```

Register it on the existing group in `router.New` — optionally raising the body
limit for this route only, since the global limit applies to every route (Clerk
`user.created` events are small, so you can omit the override):

```go
// internal/router/router.go — sketch
webhooks := engine.Group("/webhook")
webhooks.POST("/clerk", middleware.MaxReqResSizeOverride(1<<20), h.Clerk.Handle)
```

`h.Clerk` is a `*clerk.Handler` you add to `handler.Handlers` (construct it with
the signing secret from config — add a section for it in
`internal/utils/config`, e.g. `CLERK_WEBHOOK_SECRET` in `.env`).

---

## Step 11 — Add `/api/v2` for this feature

*Plugs in at:* the `// ADD YOUR DOMAIN` comment on `Endpoints` in `versions.go`.

Versions are independent: each is its own route group, middleware stack and
generated folder (`gen/ts/v1`, `gen/ts/v2`). A frontend migrates by changing one
import path, and both can coexist. v2 can reuse v1's handlers unchanged, wrap
them, or diverge. Here v2 keeps every v1 function and replaces only `getUser`,
whose response renames `name` to `displayName`. It delegates to the v1 handler and
reshapes, and gets a new DTO name (type names must be unique within a version).

**`internal/handler/userv2/endpoints.go`**

```go
// Package userv2 is the /api/v2 surface of the User feature. It reuses the
// v1 handler for everything that did not change and replaces only getUser,
// whose response gains a different shape.
package userv2

import (
	"context"
	"slices"
	"time"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/handler/user"
)

// GetUserRes is v2's getUser response: "name" is renamed "displayName".
// A new type name (not UserRes) keeps the generated v2 schemas unambiguous.
type GetUserRes struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Endpoints is v2's function list: v2's getUser first, then every other v1
// function unchanged (same DTOs, same handlers).
func Endpoints(v1 *user.Handler) []api.Endpoint {
	getUser := func(ctx context.Context, req user.GetUserReq) (GetUserRes, error) {
		u, err := v1.GetUser(ctx, req) // delegate, then reshape
		if err != nil {
			return GetUserRes{}, err
		}
		return GetUserRes{ID: u.ID, Email: u.Email, DisplayName: u.Name, CreatedAt: u.CreatedAt}, nil
	}

	return slices.Concat(
		[]api.Endpoint{
			api.Query("getUser", getUser,
				api.WithDescription("Fetches one user by id (v2: name is returned as displayName).")),
		},
		slices.DeleteFunc(user.Endpoints(v1), func(e api.Endpoint) bool { return e.Name == "getUser" }),
	)
}
```

Add the version next to v1 in `versions.go`:

```go
return []api.Version{
	{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: user.Endpoints(h.User)},
	{Name: "v2", Prefix: "/api/v2/rpc", Endpoints: userv2.Endpoints(h.User)},
}
```

Test: both versions validate, v2 has the new function first and the rest of v1
unchanged, the generator writes a separate `v2` folder containing the new schema
(and `v1` does not), and the new route is served.

**`internal/router/userv2_test.go`**

```go
package router

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/gen"
	"github.com/hritul2/goboiler/internal/handler/user"
	"github.com/hritul2/goboiler/internal/handler/userv2"
	"github.com/hritul2/goboiler/internal/models"
	"github.com/hritul2/goboiler/internal/validation"
)

// stub satisfies user's (unexported) service interface structurally.
type stub struct{}

func (stub) Create(context.Context, validation.UserCreateInput) (*models.User, error) {
	return nil, nil
}
func (stub) GetByID(_ context.Context, id models.ID) (*models.User, error) {
	u := models.NewUser("ada@example.com", "Ada", "h")
	u.ID = id
	return u, nil
}
func (stub) Update(context.Context, models.ID, validation.UserUpdateInput) (*models.User, error) {
	return nil, nil
}
func (stub) Delete(context.Context, models.ID) error                        { return nil }
func (stub) List(context.Context, models.Pagination) ([]models.User, error) { return nil, nil }

func TestV2_ReusesV1AndReplacesGetUser(t *testing.T) {
	h1 := user.NewHandler(stub{}, zerolog.Nop())
	versions := []api.Version{
		{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: user.Endpoints(h1)},
		{Name: "v2", Prefix: "/api/v2/rpc", Endpoints: userv2.Endpoints(h1)},
	}
	for _, v := range versions {
		require.NoError(t, v.Validate())
	}

	var names []string
	for _, e := range versions[1].Endpoints {
		names = append(names, e.Name)
	}
	assert.True(t, slices.Equal([]string{"getUser", "createUser", "listUsers", "updateUser", "deleteUser"}, names), names)

	// Each version generates to its own folder; the new type only exists in v2.
	files, err := gen.Generate(versions, gen.Options{})
	require.NoError(t, err)
	assert.Contains(t, string(files["v2/schemas.ts"]), "GetUserResSchema")
	assert.Contains(t, string(files["v2/client.ts"]), "/api/v2/rpc/getUser")
	assert.NotContains(t, string(files["v1/schemas.ts"]), "GetUserResSchema")

	// And it is served.
	engine, _ := newTestEngine()
	api.Mount(engine, versions[1])
	w := do(engine, http.MethodPost, "/api/v2/rpc/getUser", `{"userId":"abc"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"displayName":"Ada"`)
}
```

Then `make gen` writes `gen/ts/v2/` next to `gen/ts/v1/`. When v1 is retired,
delete its entry (and its folder).

---

## Testing: the style per layer

| Layer | Style | Needs |
|---|---|---|
| Model, validation | plain unit / table tests on the struct | nothing |
| Repository (`internal/repo/mongo`) | test the pure logic (conversion, ID parsing, index specs, find options) and that malformed IDs never reach the driver (nil collection); verify the CRUD glue against a real Mongo, not a mock | nothing for unit tests; compose for the glue |
| Service | in-memory fake of the repo **interface** plus a fake `Enqueuer`; assert error codes, `fields`, and that the repo/queue were or were not called | nothing |
| Job (email) | producer is pure; call the consumer with a fake `Sender`; metrics asserted against one in-memory reader | nothing |
| Handler | real `Endpoints(h)` over a fake service in a gin engine with `middleware.ErrorHandler`; assert status and body | nothing |
| Router | mount the real `APIVersions`; assert routes and that bad declarations fail | nothing |
| Architecture | `internal/archtest`: only `internal/db` and `internal/repo/mongo` may import the Mongo driver | `go` on PATH |
| Generator | golden files for a fixture API (`go test ./internal/gen -update` after an intended change) | nothing |
| Webhook | build a correctly signed request with the test secret, then tamper with body, id, timestamp | nothing |

A few of the shapes, taken from the tests above:

**`service test: fake repo + fake enqueuer`**

```go
func newTestService() (*UserService, *fakeUserRepo, *fakeEnqueuer) {
	repo := newFakeUserRepo()
	enq := &fakeEnqueuer{}
	return NewUserService(repo, enq, zerolog.Nop()), repo, enq
}

func TestUserService_Create_DuplicateEmail(t *testing.T) {
	svc, repo, _ := newTestService()
	repo.createErr = errs.ErrDuplicateKey

	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.Error(t, err)
	th.AssertErrorCode(t, err, errs.CodeConflict)
}
```

**`handler test: real endpoints, fake service`**

```go
// newEngine mounts the real Endpoints(h) over the fake service, so these
// tests cover DTO decoding, validation, handler logic and JSON encoding.
func newEngine(svc userService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	api.Mount(engine, api.Version{
		Name:      "v1",
		Prefix:    "/api/v1/rpc",
		Endpoints: Endpoints(NewHandler(svc, zerolog.Nop())),
	})
	return engine
}

func call(engine *gin.Engine, fn, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/rpc/"+fn, strings.NewReader(body)))
	return w
}

func TestCreateUser_ValidationFailure_400(t *testing.T) {
	w := call(newEngine(&fakeService{}), "createUser", `{"email":"nope","name":"A","password":"short"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var body errs.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, errs.CodeBadRequest, body.Code)
	// Keyed by the DTO's JSON names, so a form can map them to inputs.
	assert.Equal(t, map[string][]string{
		"email":    {"must be a valid email address"},
		"name":     {"must be at least 2 characters"},
		"password": {"must be at least 8 characters"},
	}, body.Fields)
}
```

**`repository test: no database`**

```go
// TestInvalidIDs_NeverReachTheDriver proves the "malformed ID => not found"
// rule without a live Mongo: the repo has a nil collection, so any path that
// touched it would panic.
func TestInvalidIDs_NeverReachTheDriver(t *testing.T) {
	r := &UserRepository{} // nil collection on purpose
	ctx := context.Background()

	_, err := r.GetByID(ctx, "not-an-object-id")
	assert.ErrorIs(t, err, errs.ErrRecordNotFound)

	assert.ErrorIs(t, r.Delete(ctx, "not-an-object-id"), errs.ErrRecordNotFound)
	assert.ErrorIs(t, r.Delete(ctx, ""), errs.ErrRecordNotFound)

	u := models.NewUser("a@b.c", "A", "h")
	u.ID = "not-an-object-id"
	assert.ErrorIs(t, r.Update(ctx, u), errs.ErrRecordNotFound)
	u.ID = ""
	assert.ErrorIs(t, r.Update(ctx, u), errs.ErrRecordNotFound, "never-persisted user cannot be updated")

	u.ID = "not-an-object-id"
	err = r.Create(ctx, u)
	var appErr *errs.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 400, appErr.StatusCode, "supplying a malformed ID on Create is a caller bug")
}
```

Run everything with `make test` (`go test ./internal/...`) and the full CI gate
with `make check`.
