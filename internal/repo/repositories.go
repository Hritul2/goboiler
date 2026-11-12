// Package repo is the data-access layer's DRIVER-AGNOSTIC surface: the
// repository interfaces (one file per entity, declared here) and the
// Repositories aggregate with its wiring (this file). It imports no database
// driver. Each database has its own implementation package underneath:
//
//	internal/repo/mongo      MongoDB (the only one provided)
//	internal/repo/postgres   not implemented — see below
//
// The repo layer (plus internal/db, the connection/error-mapping layer) is
// the ONLY database-specific code in the project. models, services and
// handlers see models.ID strings and the interfaces here; internal/archtest
// fails the test run if any other package imports the Mongo driver.
//
// This skeleton declares no repositories yet. docs/EXAMPLE.md (step 2) builds
// a complete one: the interface, the Mongo implementation, and a sketch of a
// Postgres one.
//
// # Adding a Postgres or SQLite implementation
//
//  1. Create internal/repo/<driver> (e.g. internal/repo/postgres) with a
//     type implementing each repository interface. Give it a private
//     row/document struct (db tags live there, never on models) and explicit
//     to/from model conversion — docs/EXAMPLE.md step 2 shows the shape.
//  2. Own the ID format in that package: generate it in Create, parse it in
//     GetByID/Update/Delete, and return errs.ErrRecordNotFound for an ID that
//     is malformed for that store.
//  3. Add the driver and its connection (internal/db/postgres.go is sketched)
//     and map its errors onto the errs sentinels next to db.MapDBError.
//  4. In NewRepositories below, construct the new implementation instead of
//     Mongo's, and add a `var _ XRepository = (*postgres.XRepository)(nil)`
//     assertion. Services, handlers and models do not change.
//  5. Extend the allow-list in internal/archtest for the new driver's import
//     path so the same isolation rule protects it.
package repo

import "context"

// Repositories aggregates every domain repository for injection into the
// service layer (CODING_SEQUENCE steps 15–16).
//
// ADD YOUR DOMAIN: one field per repository interface (e.g. `User UserRepository`),
// declared in its own file in this package — see docs/EXAMPLE.md step 2.
type Repositories struct{}

// NewRepositories picks the database implementation, constructs every
// repository and runs its EnsureIndexes — fail-fast at wiring time rather than
// on first insert. Requires db.Init to have run once there is a repository to
// build. This function is the single place that names a concrete driver
// package.
//
// ADD YOUR DOMAIN: construct each repository here (e.g.
// `users := mongorepo.NewUserRepository()`, then `users.EnsureIndexes(ctx)`)
// and set it on the returned struct — see docs/EXAMPLE.md step 2.
func NewRepositories(ctx context.Context) (*Repositories, error) {
	_ = ctx // used once you construct repositories and call EnsureIndexes
	return &Repositories{}, nil
}
