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

// example:region repo-test
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

// example:endregion repo-test

func mapKeys(m bson.M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
