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
