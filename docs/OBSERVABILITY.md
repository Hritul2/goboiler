# Observability

`internal/observability` wires up all three OpenTelemetry signals — traces,
metrics, and logs — behind a single `Provider`. This doc covers how to use
each signal from application code. For where this fits in the boot
sequence, see `BOOT_SEQUENCE.md` step 2.

## Setup (already wired, for reference)

`observability.New(ctx, cfg, log)` runs once in `cmd/main.go`'s `start()`,
right after the logger is built and before `db.Init`:

```go
obs, err := observability.New(initCtx, cfg, a.log)
...
a.log = a.log.Hook(obs.ZerologHook())
```

Boot order matters: `db.Init` runs *after* this so `otelmongo`'s command
monitor picks up the global tracer provider (see `internal/db/mongo.go`).
Redis follows the same rule. New services with their own OTel-instrumented
clients should init after `observability.New` too.

`obs.Shutdown(ctx)` is called on process exit (`cmd/main.go`'s `stop()`) to
flush all three providers.

### Enabling/disabling

Controlled by `otel` config (`internal/utils/config/otel.go`), sourced from
env in `configs/*.json`:

| Env var            | Config field | Notes                                   |
|---------------------|--------------|------------------------------------------|
| `OTEL_ENABLED`       | `Enabled`    | `false` by default locally               |
| `OTEL_ENDPOINT`      | `Endpoint`   | OTLP/HTTP collector URL, e.g. `http://localhost:4318` |
| `OTEL_SAMPLE_RATE`   | `SampleRate` | Trace sampling ratio; defaults to `1.0` if unset |

When `Enabled` is `false`, `observability.New` returns a `Provider` backed
by no-op providers — every `otel.Tracer(...)`, `otel.Meter(...)`, and the
zerolog hook still work, they just don't ship anything anywhere. This means
call sites never need to branch on whether OTel is enabled.

## Logging

Nothing changes about how you log — the existing `zerolog.Logger` (from
context or injected via constructor) is exactly the same one used
everywhere else. `ZerologHook()` bridges every log record to the OTel Logs
signal automatically once wired in `main.go`; no per-call-site changes.

```go
log.Info().Str("user_id", uid).Msg("user signed in")
log.Error().Err(err).Msg("payment failed")
```

To correlate a log line with the active trace/span, pull the IDs off the
span in context:

```go
span := trace.SpanFromContext(ctx)
log.Info().
    Str("trace_id", span.SpanContext().TraceID().String()).
    Str("span_id", span.SpanContext().SpanID().String()).
    Msg("processing order")
```

## Tracing

Get a `tracer` once per package as a package-level `var`, keyed by the
package's full import path (this is the convention used everywhere in this
repo — see `internal/services/user/user.go`, `internal/services/asyncprocessor/router.go`):

```go
var tracer = otel.Tracer("github.com/hritul2/goboiler/internal/services/order")
```

Start a span at the top of the method you want traced, and always `defer
span.End()`:

```go
func (s *OrderService) Create(ctx context.Context, req CreateOrderRequest) error {
    ctx, span := tracer.Start(ctx, "OrderService.Create")
    defer span.End()

    span.SetAttributes(
        attribute.String("order.user_id", req.UserID),
        attribute.Int("order.item_count", len(req.Items)),
    )

    if err := s.repo.Insert(ctx, req); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return err
    }
    return nil
}
```

Pass the reassigned `ctx` (not the original) down to anything called
afterward — that's what threads the span into child calls and into Mongo
queries (`otelmongo`'s command monitor picks up spans from context
automatically, no manual instrumentation needed at the repo layer — see the
comment atop `internal/repo/user.go`).

**Don't double-instrument Mongo calls.** `internal/db` already wires
`otelmongo` as a command monitor, so every query gets a span for free. Add
manual spans at the service layer (business operations), not around
individual repo calls.

## Metrics

Same package-level convention as tracing — declare a `meter` and its
instruments once per package, keyed by import path:

```go
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
```

Record against the instruments with attributes for what varies (operation
name, status, etc.) — see `internal/repo/user.go`'s `instrumentOperation`
helper for the canonical pattern: wrap the operation in a closure, time it,
and record both a counter and a duration histogram tagged with
`status=ok|error`:

```go
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
```

Existing examples to copy from:
- `internal/repo/user.go` — per-operation counter + duration histogram for
  every Mongo CRUD call
- `internal/lib/email/tasks.go` — sent/failed counters for outbound email
- `internal/services/asyncprocessor/router.go` — task-processed counter +
  duration histogram, plus a span per job (combines tracing and metrics in
  one place)

## When to add what

- **Tracing**: business-level operations you'd want to see as a span in a
  trace waterfall (service methods, async job handlers). Skip it for calls
  already covered by driver-level instrumentation (Mongo via `otelmongo`).
- **Metrics**: anything you'd want to alert on or chart — counts, error
  rates, durations. Add a counter + histogram pair per package doing
  meaningful work, following the `repo.operation` pattern above.
- **Logging**: unchanged — log what you'd always log. The OTel bridge is
  transparent; don't add extra logging just because OTel exists.
