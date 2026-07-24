# Async Jobs — How the Queue Works

This is a practical guide to the background job system (`internal/services/asyncprocessor`, on top of `hibiken/asynq` over Redis). It explains the moving parts in plain terms and walks through how to add a new job. For the "why it's built this way" architectural reasoning, see `CODING_SEQUENCE.md` step 13; this document is the "how do I actually use it" companion.

---

## The mental model

Think of it as a post office, not a function call:

- **Producer** — code that wants something done later, without waiting for it. It writes a task ("send this email to this address") and drops it in the queue. It does not do the work itself, and it doesn't wait around.
- **Queue (Redis)** — holds tasks until a worker is free to pick them up.
- **Consumer / worker** — a background process that continuously pulls tasks off the queue and runs them.
- **Router** — the sorting desk. When a task comes off the queue, the router looks at its *type* (a string, like `"email:welcome"`) and hands it to whichever function was registered for that type. If nothing was registered for a type, the task fails with a "not found" error.

`internal/services/asyncprocessor` only provides the queue, the worker, and the router. It has no idea what any task actually does — that's deliberate, so this package never has to change when you add a new kind of job.

---

## The four things you need to know

### 1. A task type

A task type is just a string constant that identifies what kind of job this is — e.g. `"email:welcome"`. It's how the router knows which handler to call. Pick something namespaced (`domain:action`) so types from different parts of the app don't collide.

### 2. A producer — "how to enqueue"

A producer is a small function that builds a task and hands it to the queue. It has two halves:

**Half A — build the task.** Decide what data the job needs (the *payload*), and write a function that packages it up:

```go
const TaskWelcomeEmail = "email:welcome"

type WelcomeEmailPayload struct {
    To   string `json:"to"`
    Name string `json:"name"`
}

func NewWelcomeEmailTask(to, name string) (*asynq.Task, error) {
    payload, err := json.Marshal(WelcomeEmailPayload{To: to, Name: name})
    if err != nil {
        return nil, err
    }
    return asynq.NewTask(TaskWelcomeEmail, payload, welcomeEmailTaskOptions()...), nil
}
```

This lives next to whatever domain owns the job — the welcome-email task lives in `internal/lib/email/tasks.go`, not in the queue package. If you're adding a "send invoice" job, it'd live wherever invoices are handled.

**Half B — actually enqueue it.** Anywhere in the app that needs to trigger the job calls the producer function, then hands the result to something that can enqueue:

```go
task, err := email.NewWelcomeEmailTask(user.Email, user.Name)
if err != nil {
    // building the task failed — handle/log
}
if _, err := enqueuer.EnqueueContext(ctx, task); err != nil {
    // the queue itself is unreachable — handle/log
}
```

"Enqueuing" just means: serialize the task, push it onto the right Redis list, and return immediately. Nothing runs synchronously.

**Where does `enqueuer` come from?** Services accept a narrow interface — just the one method they need:

```go
type Enqueuer interface {
    EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
```

The real thing you pass in is `asyncprocessor.Get().Client`. Services depend on the interface, not on `asyncprocessor` directly — this is what lets you test a service's enqueue behavior with a fake, without ever touching Redis.

### 3. A handler — "how to create a handler"

A handler is the function that actually does the work when a task comes off the queue. Its signature is always the same shape:

```go
func(ctx context.Context, task *asynq.Task) error
```

Write it as a **factory** — a function that takes whatever dependencies the handler needs (a sender, a repository, whatever) and *returns* the actual handler function. This is what lets the handler use real dependencies without the queue package needing to know about them:

```go
func NewWelcomeEmailHandler(sender Sender, log zerolog.Logger) func(ctx context.Context, task *asynq.Task) error {
    return func(ctx context.Context, t *asynq.Task) error {
        var p WelcomeEmailPayload
        if err := json.Unmarshal(t.Payload(), &p); err != nil {
            return fmt.Errorf("email: unmarshal welcome email payload: %w", err)
        }
        if err := SendWelcome(ctx, sender, p.To, p.Name); err != nil {
            log.Error().Err(err).Str("to", p.To).Msg("failed to send welcome email")
            return err
        }
        log.Info().Str("to", p.To).Msg("sent welcome email")
        return nil
    }
}
```

Rules of thumb for writing a handler:
- **Unmarshal the payload first**, and return an error immediately if it's malformed — don't guess at zero values.
- **Return an error to signal failure.** Returning `nil` means "done, don't retry." Returning any other error means "retry me later" (see Retries below).
- **Log inside the handler** for both success and failure — the worker doesn't log task contents for you.
- Handlers should be **idempotent where possible** — a task can be retried and, more rarely, redelivered, so running it twice shouldn't corrupt anything.

This lives next to the producer, in the same domain file (`internal/lib/email/tasks.go` for the welcome-email example).

### 4. A router — "how to create a router, and how dequeuing works"

The router is what turns "a task came off the queue" into "call the right handler." You build one, register every handler you have against it, then start the queue with it:

```go
router := asyncprocessor.NewRouter()
router.Handle(email.TaskWelcomeEmail, email.NewWelcomeEmailHandler(sender, log))
// router.Handle(invoice.TaskSendInvoice, invoice.NewSendInvoiceHandler(...))
// ...one line per task type you support...
```

This registration step — building every handler's dependencies and calling `router.Handle` for each task type — lives in its own function, `services.NewJobRouter`, in `internal/services/registerasynctasks.go`. That's deliberately **not** in `cmd/main.go` and **not** in `internal/services/asyncprocessor`: `cmd/main.go` should stay a thin "start this, then that" sequence, and asyncprocessor must never know about email or any other domain. `internal/services` is the composition point that's already allowed to know about every domain (it's where `services.go` aggregates domain services too), so this is where "which job kinds exist" belongs.

`cmd/main.go` just calls it and passes the result on:

```go
router, err := services.NewJobRouter(cfg, a.log)
if err != nil {
    return a, err
}
job, err := asyncprocessor.Init(ctx, cfg, a.log, router)
```

`asyncprocessor.Init` takes the router, starts the background worker, and from that point on:

1. The worker continuously pulls tasks off Redis (this is "dequeuing" — you never call a "dequeue" function yourself, it happens automatically in the background as soon as `Init` runs).
2. For each task, the router looks at its type string and calls the matching handler.
3. If the handler returns `nil`, the task is done and removed.
4. If the handler returns an error, asynq retries it later (see below).

You never call `router.mux` or touch asynq's internals directly — `Handle` is the only method you need.

---

## Putting it together: adding a brand-new job, step by step

Say you want a "cleanup expired invites" job.

1. **Decide where it lives.** Pick the package that owns this behavior — e.g. `internal/services/user` if it's User-domain, or a new package if it's genuinely cross-cutting.
2. **Define the task type and payload** in that package (a new file, e.g. `tasks.go`, mirroring `internal/lib/email/tasks.go`):
   ```go
   const TaskCleanupInvites = "invite:cleanup"
   type CleanupInvitesPayload struct { OlderThanDays int }
   ```
3. **Write the producer:**
   ```go
   func NewCleanupInvitesTask(olderThanDays int) (*asynq.Task, error) {
       payload, err := json.Marshal(CleanupInvitesPayload{OlderThanDays: olderThanDays})
       if err != nil { return nil, err }
       return asynq.NewTask(TaskCleanupInvites, payload,
           asynq.MaxRetry(2), asynq.Queue("low"), asynq.Timeout(time.Minute)), nil
   }
   ```
4. **Write the handler factory**, taking whatever the cleanup logic needs (a repo, typically):
   ```go
   func NewCleanupInvitesHandler(repo InviteRepository, log zerolog.Logger) func(ctx context.Context, t *asynq.Task) error {
       return func(ctx context.Context, t *asynq.Task) error {
           var p CleanupInvitesPayload
           if err := json.Unmarshal(t.Payload(), &p); err != nil { return err }
           return repo.DeleteExpired(ctx, p.OlderThanDays)
       }
   }
   ```
5. **Register it in `internal/services/registerasynctasks.go`** (`NewJobRouter`), next to the existing `router.Handle` call — not in `cmd/main.go`:
   ```go
   router.Handle(user.TaskCleanupInvites, user.NewCleanupInvitesHandler(inviteRepo, log))
   ```
6. **Enqueue it from wherever should trigger it** — a scheduled trigger, an admin action, whatever — via the same `Enqueuer` interface pattern services already use.

Notice `internal/services/asyncprocessor` was never opened. That's the whole design.

---

## Queues and priority

There are three lanes, defined once in `asyncprocessor`'s `queues()`:

| Queue | Worker share |
|---|---|
| `critical` | 6 |
| `default` | 3 |
| `low` | 1 |

The numbers are relative weights, not hard limits — with concurrency 10, the worker spends roughly 60% of its capacity on `critical` tasks, 30% on `default`, 10% on `low`, whenever all three have work waiting. Pick a queue per task via `asynq.Queue("critical"|"default"|"low")` when building it in the producer. If you don't specify one, it goes to `default`.

## Retries, timeouts, and other task options

These are set when the **producer** builds the task, as extra arguments to `asynq.NewTask`:

- `asynq.MaxRetry(n)` — how many times to retry a failing task before giving up (it then moves to asynq's "archive").
- `asynq.Timeout(d)` — how long the handler is allowed to run before it's considered failed and retried.
- `asynq.Queue(name)` — which priority lane it goes in.
- `asynq.ProcessIn(d)` / `asynq.ProcessAt(t)` — delay a task instead of running it immediately (e.g. "send this reminder in 24h").
- `asynq.Unique(ttl)` — prevent duplicate enqueues of an identical task within a time window.

If a handler panics or returns a non-nil error and retries are exhausted, the task is archived (kept, not silently dropped) so it can be inspected later — it will not be retried again automatically.

## Reading the DLQ (archived tasks)

asynq's "archive" is the dead-letter-queue equivalent — tasks that exhausted their retries land there instead of disappearing. To look at what's archived:

```go
tasks, err := asyncprocessor.Get().ListArchivedTasks("default")
for _, t := range tasks {
    fmt.Println(t.Type, string(t.Payload), t.LastErr)
}
```

**This is a Go-API-only capability — there is no HTTP route for it, deliberately.** Archived task payloads can contain PII (e.g. the welcome-email task's recipient address), and this codebase has no auth yet. Reading the DLQ today means writing a small script, a debug command, or a REPL-style call — not a `curl`. Once real auth exists, an admin-gated HTTP route can wrap this same method.

## Testing a handler or a producer

Neither needs Redis:

- **Testing a producer**: call it directly, unmarshal the resulting task's payload, assert on the fields — same as any pure function.
- **Testing a handler**: call the factory with a fake dependency (a fake `Sender`, a fake repo), then call the returned function directly with a hand-built `*asynq.Task`, and assert on what the fake recorded.
- **Testing the router itself**: register a fake handler, build a task, run it through the router — see `internal/services/asyncprocessor/router_test.go` for the pattern.

The only thing that genuinely needs a live Redis to verify is the full path — enqueue for real, let the worker actually pick it up — and that's done manually via `docker compose up -d redis` plus a throwaway smoke test, not as part of the regular test suite.

## Shutdown

You don't need to do anything special when adding a job — shutdown is handled once, centrally. When the app stops, the queue's `Close` drains any in-flight tasks (gives them time to finish, up to a configured timeout) before the process exits. It happens automatically as part of the existing boot/shutdown sequence in `cmd/main.go`.

## Quick reference: where things live

| Concern | Location |
|---|---|
| The queue/worker singleton (`Init`/`Get`/`Close`) | `internal/services/asyncprocessor/job.go` |
| The router (`Router`, `Handle`) | `internal/services/asyncprocessor/router.go` |
| Welcome-email task type, payload, producer, handler | `internal/lib/email/tasks.go` |
| Which job kinds exist — building handler deps + `router.Handle` calls | `internal/services/registerasynctasks.go` (`NewJobRouter`) |
| Starting the queue with the built router | `cmd/main.go`, inside `start()` |
| Example of a service enqueuing a task | `internal/services/user/user.go` (`enqueueWelcomeEmail`) |
| Reading the DLQ (`ListArchivedTasks`) | `internal/services/asyncprocessor/inspector.go` |
