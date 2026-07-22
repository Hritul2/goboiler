// Package repo is the data-access layer: one repository per domain entity,
// interface + Mongo implementation, all driver errors normalised through
// db.MapDBError before they leave this package.
//
// Testing note: *mongo.Collection is a concrete type and cannot be faked
// without an abstraction layer worse than the problem. Unit tests therefore
// cover the pure logic factored out of driver calls (index specs, find
// options); the thin CRUD glue is verified against a live Mongo via the
// local docker compose, and error normalisation by db.MapDBError's own
// test suite.
package repo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/hritul2/goboiler/internal/db"
	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/models"
)

const usersCollection = "users"

// UserRepository is the data-access contract for User entities.
type UserRepository interface {
	Create(ctx context.Context, u *models.User) error
	GetByID(ctx context.Context, id bson.ObjectID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, u *models.User) error
	Delete(ctx context.Context, id bson.ObjectID) error
	List(ctx context.Context, p models.Pagination) ([]models.User, error)
	// EnsureIndexes idempotently creates the collection's indexes. Called
	// once at wiring time by NewRepositories.
	EnsureIndexes(ctx context.Context) error
}

type mongoUserRepo struct {
	col *mongo.Collection
}

// NewUserRepository returns the Mongo-backed UserRepository. Requires
// db.Init to have run (boot order).
func NewUserRepository() UserRepository {
	return &mongoUserRepo{col: db.GetMongo().Collection(usersCollection)}
}

// userIndexModels declares the users collection's indexes (pure, unit-tested).
func userIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
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

func (r *mongoUserRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := r.col.Indexes().CreateMany(ctx, userIndexModels()); err != nil {
		return db.MapDBError(err)
	}
	return nil
}

func (r *mongoUserRepo) Create(ctx context.Context, u *models.User) error {
	if u.ID.IsZero() {
		u.Base = models.NewBase()
	}
	if _, err := r.col.InsertOne(ctx, u); err != nil {
		// Duplicate email surfaces as errs.ErrDuplicateKey via the mapper.
		return db.MapDBError(err)
	}
	return nil
}

func (r *mongoUserRepo) GetByID(ctx context.Context, id bson.ObjectID) (*models.User, error) {
	return r.findOne(ctx, bson.M{"_id": id})
}

func (r *mongoUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	return r.findOne(ctx, bson.M{"email": email})
}

func (r *mongoUserRepo) findOne(ctx context.Context, filter bson.M) (*models.User, error) {
	var u models.User
	if err := r.col.FindOne(ctx, filter).Decode(&u); err != nil {
		return nil, db.MapDBError(err)
	}
	return &u, nil
}

func (r *mongoUserRepo) Update(ctx context.Context, u *models.User) error {
	u.Touch()
	res, err := r.col.ReplaceOne(ctx, bson.M{"_id": u.ID}, u)
	if err != nil {
		return db.MapDBError(err)
	}
	if res.MatchedCount == 0 {
		return errs.ErrRecordNotFound
	}
	return nil
}

func (r *mongoUserRepo) Delete(ctx context.Context, id bson.ObjectID) error {
	res, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return db.MapDBError(err)
	}
	if res.DeletedCount == 0 {
		return errs.ErrRecordNotFound
	}
	return nil
}

func (r *mongoUserRepo) List(ctx context.Context, p models.Pagination) ([]models.User, error) {
	p.Normalize()

	cursor, err := r.col.Find(ctx, bson.M{}, listFindOptions(p))
	if err != nil {
		return nil, db.MapDBError(err)
	}

	users := make([]models.User, 0, p.Limit)
	if err := cursor.All(ctx, &users); err != nil {
		return nil, db.MapDBError(err)
	}
	return users, nil
}
