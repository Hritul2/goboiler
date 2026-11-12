// Package mongo is the MongoDB implementation of the repository interfaces
// declared in internal/repo. It is the ONLY place (besides internal/db) that
// may import the Mongo driver: everything above it — models, services,
// handlers — sees models.ID strings and domain model values, never ObjectIDs
// or bson. internal/archtest fails the build if that rule is broken.
//
// Storage mapping: each entity has a private document struct carrying the
// bson tags (e.g. userDoc) plus explicit to/from model conversion. The model's
// opaque models.ID is the hex form of the document's ObjectID; an ID that
// is not a valid ObjectID cannot exist in this store, so lookups with one
// return errs.ErrRecordNotFound (404), not a validation error.
//
// Testing note: *mongo.Collection is a concrete type and cannot be faked
// without an abstraction layer worse than the problem. Unit tests therefore
// cover the pure logic factored out of driver calls (index specs, find
// options, document conversion, ID parsing); the thin CRUD glue is verified
// against a live Mongo via the local docker compose, and error normalisation
// by db.MapDBError's own test suite.
//
// Observability: no manual tracing or logging is added here — otelmongo's
// command monitor (wired in internal/db) already traces every query, and a
// debug log per query would be volume with no context beyond what the
// service layer already logs. Metrics ARE added (via instrumentOperation),
// because unlike tracing, no automatic per-operation Mongo metrics exist.
//
// This package ships with no repository, only the shared metrics helper
// (instrument.go). ADD YOUR DOMAIN: one file per entity here, with a private
// document struct, conversion, ID parsing and every CRUD method routed
// through instrumentOperation and db.MapDBError — see docs/EXAMPLE.md step 2.
package mongo
