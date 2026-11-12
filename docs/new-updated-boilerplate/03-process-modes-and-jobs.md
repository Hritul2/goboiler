# Phase 3 — One binary with modes; worker and scheduler on Redis

**Goal (D3, D5):** one build artifact runs as API server, job worker, job
scheduler, or a one-off command. The queue is Redis (asynq) by default.

## Modes
```
app serve       # HTTP API only; enqueues jobs
app worker      # processes jobs (asynq server)
app scheduler   # enqueues periodic jobs (asynq scheduler); run exactly one
app all         # serve + worker + scheduler in one process (local dev default)
app migrate ... # Phase 2
app seed ...    # Phase 2
```
- Parsing uses the standard library (`flag` + a small subcommand switch); no CLI framework.
- Every mode shares the same boot sequence (config → logger → otel → db → redis) and only starts what it needs. `BOOT_SEQUENCE.md` gets a table: which steps each mode runs.
- Makefile: `make dev` (all), `make dev-serve`, `make dev-worker`, `make dev-scheduler`.

## Worker
- Concurrency, queue names and priorities come from config (`jobs.concurrency`, `jobs.queues: {critical: 6, default: 3, low: 1}`).
- Retries with exponential backoff and jitter; max retry per task type; tasks that run out of retries go to asynq's archive (the dead-letter queue). Tooling: `make jobs-dlq` lists archived tasks; retrying or deleting them uses the existing inspector (Phase 7 wires it behind an admin-only HTTP route once a project adds auth).
- Graceful drain: on SIGTERM stop taking new tasks, wait up to `jobs.shutdown_timeout` for running ones, then exit.
- Unique / idempotent tasks: an optional `jobs.Unique(ttl)` option (asynq `Unique`) and a documented idempotency-key pattern.

## Scheduler
- Periodic jobs registered in code: `scheduler.Every("@every 5m", task)` or cron syntax, with time zone from config.
- Only **one** scheduler may run: asynq's scheduler plus a Redis leader lock so an accidental second instance stays idle. Documented in deploy docs.

## Tracing across processes (D7)
- The enqueue span's trace context is injected into task headers/payload metadata and extracted in the worker, so a request → job chain shows up as one trace.
- Metrics: queue depth per queue (polled from the inspector), task latency (enqueued → started), processing duration, retries, archived count.

## Health
- `worker` and `scheduler` expose a tiny health server on a separate port (`/live`, `/ready`) so orchestrators can probe them (Phase 7 defines the checks).

## Done when
- `app serve` + `app worker` as two processes complete an enqueued job end to end (integration test, Phase 8).
- SIGTERM during a long job drains it without loss (test).
- Two schedulers started at once enqueue each periodic job only once (test).
