# Phase 8 — Integration tests with testcontainers

**Goal (D8):** code that talks to real infrastructure (repos, transactions,
migrations, jobs, adapters) is tested against the real thing, in CI and locally.

## Setup
- `testcontainers-go` (new module dependency, approval at phase start), with modules for MongoDB (single-node replica set), Redis, MinIO and Mailpit.
- Build tag `integration`, so `make test` stays fast and dependency-free; `make test-integration` runs them. Docker must be available (Docker Desktop locally, the CI runner in GitHub Actions).
- One shared container set per package (`TestMain`), each test isolated by a unique database name / key prefix / bucket, so tests can run in parallel.
- Never touches a developer's local Mongo or the compose stack (separate random ports).

## What is covered
- Repo CRUD and indexes for the example domain and generated domains (Phase 6).
- `WithTx` rollback and retry (Phase 2).
- Migrations up/down/status and the migration lock.
- Jobs: enqueue in one goroutine-run `serve`, process in a `worker`, retries, archive, graceful drain (Phase 3).
- Cache, storage (including presigned URLs), mail delivery checked through Mailpit's API (Phase 4).
- Trace propagation from enqueue to worker (Phase 7), using the in-memory exporter.

## CI
- New `integration` job in `.github/workflows/ci.yml`, run on every PR; container images cached where possible.

## Done when
- `make test-integration` passes locally and in CI from a clean machine.
- `HOW_TO_USE_IT.md` documents when to write a unit test vs an integration test.
