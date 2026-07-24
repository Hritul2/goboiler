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
//
// Observability: no manual tracing or logging is added here — otelmongo's
// command monitor (wired in internal/db) already traces every query, and a
// debug log per query would be volume with no context beyond what the
// service layer already logs. Metrics ARE added (via instrumentOperation),
// because unlike tracing, no automatic per-operation Mongo metrics exist.
package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/hritul2/goboiler/internal/db"
	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/models"
)

const usersCollection = "users"

// meter is the package-level instrumentation handle, following
// internal/observability's documented convention (one per package, keyed by
// import path).
var (
	meter = otel.Meter("github.com/hritul2/goboiler/internal/repo")

	repoOperationCounter, _ = meter.Int64Counter(
		"repo.operation",
		metric.WithDescription("Total repository operations, by operation and status"),
	)
	repoOperationDuration, _ = meter.Float64Histogram(
		"repo.operation.duration_ms",
		metric.WithDescription("Repository operation duration"),
		metric.WithUnit("ms"),
	)
)

// instrumentOperation records repo.operation/repo.operation.duration_ms
// around fn and forwards fn's return value unchanged. This is the one
// shared place every CRUD method routes through for metrics — kept
// independent of *mongo.Collection so it's unit-testable without a live
// Mongo connection.
func instrumentOperation(ctx context.Context, operation string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := float64(time.Since(start).Milliseconds())

	status := "ok"
	if err != nil {
		status = "error"
	}

	repoOperationCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("status", status),
	))
	repoOperationDuration.Record(ctx, duration, metric.WithAttributes(
		attribute.String("operation", operation),
	))

	return err
}

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
	return instrumentOperation(ctx, "ensure_indexes", func() error {
		if _, err := r.col.Indexes().CreateMany(ctx, userIndexModels()); err != nil {
			return db.MapDBError(err)
		}
		return nil
	})
}

func (r *mongoUserRepo) Create(ctx context.Context, u *models.User) error {
	return instrumentOperation(ctx, "create", func() error {
		if u.ID.IsZero() {
			u.Base = models.NewBase()
		}
		if _, err := r.col.InsertOne(ctx, u); err != nil {
			// Duplicate email surfaces as errs.ErrDuplicateKey via the mapper.
			return db.MapDBError(err)
		}
		return nil
	})
}

func (r *mongoUserRepo) GetByID(ctx context.Context, id bson.ObjectID) (*models.User, error) {
	return r.findOne(ctx, "get_by_id", bson.M{"_id": id})
}

func (r *mongoUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	return r.findOne(ctx, "get_by_email", bson.M{"email": email})
}

func (r *mongoUserRepo) findOne(ctx context.Context, operation string, filter bson.M) (*models.User, error) {
	var u models.User
	err := instrumentOperation(ctx, operation, func() error {
		if err := r.col.FindOne(ctx, filter).Decode(&u); err != nil {
			return db.MapDBError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *mongoUserRepo) Update(ctx context.Context, u *models.User) error {
	return instrumentOperation(ctx, "update", func() error {
		u.Touch()
		res, err := r.col.ReplaceOne(ctx, bson.M{"_id": u.ID}, u)
		if err != nil {
			return db.MapDBError(err)
		}
		if res.MatchedCount == 0 {
			return errs.ErrRecordNotFound
		}
		return nil
	})
}

func (r *mongoUserRepo) Delete(ctx context.Context, id bson.ObjectID) error {
	return instrumentOperation(ctx, "delete", func() error {
		res, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
		if err != nil {
			return db.MapDBError(err)
		}
		if res.DeletedCount == 0 {
			return errs.ErrRecordNotFound
		}
		return nil
	})
}

func (r *mongoUserRepo) List(ctx context.Context, p models.Pagination) ([]models.User, error) {
	var users []models.User
	err := instrumentOperation(ctx, "list", func() error {
		p.Normalize()

		cursor, err := r.col.Find(ctx, bson.M{}, listFindOptions(p))
		if err != nil {
			return db.MapDBError(err)
		}

		users = make([]models.User, 0, p.Limit)
		if err := cursor.All(ctx, &users); err != nil {
			return db.MapDBError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}
