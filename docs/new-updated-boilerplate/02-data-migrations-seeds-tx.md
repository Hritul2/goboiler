# Phase 2 — Migrations, seeds, transactions, cursor pagination (MongoDB)

**Goal:** schema and data changes are versioned and repeatable; multi-document
writes can be atomic; large lists page efficiently.

## 2.1 Migrations
- A migration is a Go value registered in code: `ID` (sortable, e.g. `20261112_120000_create_users_indexes`), `Up(ctx, db)`, optional `Down(ctx, db)`.
- Applied migrations are recorded in a `schema_migrations` collection with ID, applied-at, duration and checksum.
- A distributed lock (a document with a lease, or Redis `SET NX` with TTL) stops two instances migrating at the same time.
- Commands (D3/D4): `app migrate up | down [N] | status`, exposed as `make migrate`, `make migrate-down`, `make migrate-status`.
- Index creation moves from repo constructors (`EnsureIndexes` today) into migrations, so production deploys control when indexes are built.
- Policy documented: `serve` never auto-migrates by default; a config flag `db.auto_migrate` (default **false**, true in local config) allows it for development.

## 2.2 Seeds
- Seeds are registered like migrations but are **idempotent** (upserts) and environment-scoped (`dev`, `test`).
- `app seed [--env dev]` → `make seed`. Refuses to run when `app.env` is `prod` unless `--force`.

## 2.3 Transactions
- `repo.WithTx(ctx, func(ctx context.Context) error)` in the repo layer (driver-agnostic interface). The Mongo implementation uses a session plus `WithTransaction`, retries transient errors, and puts the session in the context so repo methods join it automatically.
- Services call `WithTx` without knowing about Mongo sessions; archtest keeps that rule.
- Requires a replica set: local and test Mongo run as a **single-node replica set** (compose and testcontainers both set this up).

## 2.4 Cursor pagination
- Add a `models.Cursor` type and a `Page[T]` result (`items`, `nextCursor`) alongside today's offset `Pagination`.
- The Mongo helper builds cursors from `(sortField, _id)` and encodes them as opaque base64 strings.
- The generator maps `Page[T]` to concrete named types (generics are not allowed on the wire today; the plan is a small named wrapper per domain, produced by the Phase 6 generator).

## Observability (D7)
Every migration and seed run gets a span plus a log line with ID and duration; metrics `db.migration.applied` and `db.migration.duration_ms`; transaction spans with retry counts.

## Done when
- `make migrate-status` lists migrations on a fresh database; `up`/`down` work; the lock is tested with two concurrent runs.
- A service test proves a failing `WithTx` rolls back both writes (integration test in Phase 8).
- Docs: `HOW_TO_USE_IT.md` (decision entries), `CODING_SEQUENCE.md`, the example.
