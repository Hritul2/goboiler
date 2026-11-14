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
