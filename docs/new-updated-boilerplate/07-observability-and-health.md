# Phase 7 — Observability audit, liveness and readiness

**Goal (D7):** every layer emits traces, metrics and correlated logs, and the
rule is enforced by tests, not just reviewed by hand.

## 7.1 Audit (first deliverable: a checklist in this folder)
For each layer, record what exists today and what's missing:

| Layer | Traces | Metrics | Logs | Correlation (trace ID / request ID in logs) |
|---|---|---|---|---|
| HTTP server (otelgin) | | | | |
| RPC adapter (per function name) | | | | |
| Mongo (otelmongo) | | | | |
| Redis (redisotel) | | | | |
| Jobs: enqueue → worker (Phase 3) | | | | |
| Scheduler (Phase 3) | | | | |
| Cache / storage / mail (Phase 4) | | | | |
| HTTP client (Phase 5) | | | | |
| Migrations / seeds (Phase 2) | | | | |
| Boot and shutdown | | | | |

## 7.2 Fixes expected from the audit
- RPC spans and metrics named by **function** (`rpc.v1.getUser`), not only by HTTP route, with outcome and error code as attributes.
- Errors: 5xx responses record the error on the span and log it once at error level, including the internal cause; 4xx are logged at info/debug without stack noise.
- Log ↔ trace correlation everywhere: every log line inside a request or job carries `trace_id`, `span_id`, `request_id` / `task_id`.
- Sensitive-data rule: no request bodies, tokens or emails in logs or span attributes by default; a redaction helper.
- Resource attributes: service name, version, environment, mode (`serve` / `worker` / `scheduler`), instance ID.
- Sampling configurable (`otel.sampler: parentbased_traceidratio`, ratio from config).

## 7.3 Enforcement
- Test helpers in `kit/observability/obstest`: an in-memory span exporter, a manual metric reader and a log capture, so tests assert "this call produced span X with attribute Y and metric Z".
- Each adapter and the RPC adapter get one such test. An archtest checks that adapter constructors return the instrumented wrapper.

## 7.4 Health endpoints
- `GET /live`: process is up; no dependency checks (fast, never fails because Redis is slow).
- `GET /ready`: checks Mongo, Redis and, per mode, the queue/scheduler lock; returns 503 with per-check detail while starting or draining.
- `/ping` stays as an alias of `/ready` for compatibility; docs say to probe `/live` and `/ready`.
- During graceful shutdown `/ready` flips to 503 first, then the server drains.

## 7.5 Local stack
`docker-compose.local.yaml` (Phase 9) runs the collector plus a UI. Recommended: `grafana/otel-lgtm` (Grafana, Tempo, Loki, Prometheus in one image). `OBSERVABILITY.md` explains where to look for a trace, a metric and a log line.

## Done when
- The audit table is filled with "✅" for every row, each backed by a test.
- A request that enqueues a job shows API span → job span as one trace in the local UI.
