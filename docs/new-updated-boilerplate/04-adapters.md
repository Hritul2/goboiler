# Phase 4 — Adapters: cache (Redis), storage (S3), mail (SMTP)

**Goal (D6):** each common backend need has one small interface in `kit`, one
production implementation, and a local stand-in that runs in docker. Services
depend on the interface only.

## Shared rules
- Interface in `kit/<adapter>`, implementation in `kit/<adapter>/<driver>`.
- Construction is explicit in `main` / the mode wiring, chosen by config (`cache.driver: redis`, `storage.driver: s3`, `mail.driver: smtp`). Only one driver of each ships now; the config key exists so a project can add another.
- Every implementation is wrapped with tracing, metrics and structured logs (D7) and returns `errs` types for "not found", "conflict", "unavailable".
- A **fake** in-memory implementation lives in `kit/<adapter>/fake` for unit tests only, never selectable in config.

## 4.1 Cache → Redis
- Interface: `Get`, `Set(ttl)`, `Delete`, `GetOrLoad(ctx, key, ttl, loader)` with single-flight so concurrent misses run the loader once.
- Typed JSON helpers via generics (`cache.GetJSON[T]`).
- Key prefix from `app.name` + environment, so projects sharing a Redis don't collide.
- Uses the existing Redis client (one connection pool); metrics `cache.hit`, `cache.miss`, `cache.duration_ms`.

## 4.2 Storage → S3
- Interface: `Put(ctx, key, reader, opts)`, `Get` (stream), `Delete`, `Head`, `List(prefix)`, `PresignGet` / `PresignPut(ttl)`.
- Implementation with `aws-sdk-go-v2` (new module dependency — needs approval at phase start). Supports custom endpoint + path-style for MinIO.
- Local: MinIO in `docker-compose.local.yaml`, with a bucket created at startup.
- Uploads through the API use presigned PUT URLs by default (the API never proxies large files), documented with an example.

## 4.3 Mail → SMTP
- Already partly there (`internal/lib/email`: client + SMTP). Formalise the `Sender` interface, add templating (HTML + text, Go `html/template`, embedded files), and a "send via job" helper so mail is sent by the worker with retries, not inside a request.
- Local: Mailpit (already in compose).

## Done when
- Each adapter has unit tests with the fake and integration tests against Redis / MinIO / Mailpit (Phase 8).
- `HOW_TO_USE_IT.md` explains how to add a second driver (e.g. Resend for mail, GCS for storage).
