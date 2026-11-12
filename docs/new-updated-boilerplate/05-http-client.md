# Phase 5 — Outbound HTTP client

**Goal:** every call to a third-party API goes through one client that is safe
by default: bounded, retried correctly, traced and logged.

## Behaviour
- Built on `net/http`; `httpclient.New(name, opts...)` returns a configured `*http.Client` plus small JSON helpers (`DoJSON(ctx, req, &out)`).
- **Timeouts:** overall request timeout, plus dial, TLS handshake and response-header timeouts. No client without a timeout.
- **Retries:** only for idempotent methods (or when the caller passes an idempotency key), on network errors, 429 and 5xx. Exponential backoff with jitter; respects `Retry-After`; max attempts and max total time from options. Request bodies are buffered or re-creatable so a retry resends them.
- **Limits:** maximum response body size; connection pool sizes per host.
- **Circuit breaker (optional per client):** opens after N consecutive failures and fails fast for a cool-down period. Default off.
- **Errors:** non-2xx responses become a typed error with status, a truncated body and the upstream name; mapped to `errs.ServiceUnavailable` when retries are exhausted.

## Observability (D7)
- Tracing via `otelhttp` transport (new module dependency, approval at phase start); span names `HTTP <method> <client name>`.
- Metrics per client name: requests, duration, retries, breaker state.
- Logs: one line per final outcome with client name, method, host, status, attempts, duration and trace ID. Headers such as `Authorization` are redacted.

## Done when
- Unit tests against `httptest` servers cover timeout, retry on 503, no retry on POST without an idempotency key, `Retry-After`, body limit, breaker.
- The Clerk webhook / an example outbound call in the docs use it.
