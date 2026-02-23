# Graceful Restart & In‑Flight Request Draining

This document audits all long‑running/interruptible paths in the codebase and provides a concrete implementation plan to add graceful restart so the server only restarts after a request is fully processed — including finishing streaming, billing, logging, and request‑cost reconciliation.

- Why this matters: Most channels/adapters use Server‑Sent Events (SSE), so a single request/response can last minutes. A Docker rolling upgrade can terminate the process mid‑stream, causing broken client responses and unbilled requests.

## 1) Inventory: Where a Restart Can Interrupt Work

Below is a comprehensive list of logic that can be interrupted by a process restart. The list is grouped by request lifecycle stages and by cross‑cutting concerns.

### 1.1 Entry points and routing

- File `router/relay.go`
  - All relay endpoints under `/v1/*` are long‑running candidates. Notable ones:
    - `POST /v1/chat/completions` → OpenAI ChatCompletion
    - `POST /v1/responses` → OpenAI Response API
    - `POST /v1/messages` → Claude Messages API
    - `POST /v1/images/*`, `POST /v1/audio/*` → can also be long‑running
    - `GET /v1/realtime` → realtime proxying
- File `controller/relay.go`
  - `Relay(c *gin.Context)` is the single entry point that selects a `relayMode` and dispatches to the concrete helper (text, response API, messages, audio/image, etc.).
  - It contains retry loops across channels and starts background error processing via `go processChannelRelayError(...)` (interruptible if the process dies).

### 1.2 SSE and long‑running response writers

SSE responses are written incrementally inside the handler goroutine in most adapters/handlers. Shutdown while streaming will terminate the TCP connection; we must prevent shutting down until these handlers exit.

- File `relay/controller/text.go`
  - `RelayTextHelper()`
    - Calls `adaptor.DoRequest()` then `adaptor.DoResponse()`.
    - `DoResponse()` often performs SSE streaming (writes via `c.Render(-1, common.CustomEvent{Data: ...})`).
    - After streaming, it triggers post‑billing in a background goroutine (see 1.3).
- File `relay/controller/response.go`
  - `ResponseAPIHandler()` (invoked through `Relay()` path when mode is Response API)
    - Same pattern: request → possibly stream response → post‑billing in background goroutine.
- File `relay/controller/claude_messages.go`
  - Claude Messages streaming and conversion paths:
    - For Claude native channels: direct SSE passthrough.
    - For OpenAI‑compatible upstreams: uses SSE conversion (accumulates text/tool args, computes usage), then writes Claude‑native SSE events.
    - At the end, usage is fed into billing.
- File `relay/adaptor/*/main.go` and friends
  - Many provider‑specific adapters stream SSE events:
    - AWS: `relay/adaptor/aws/*/main.go` (OpenAI, Claude, Cohere, Mistral, Llama3, Writer, etc.).
    - Ali, Baidu, Zhipu, Cloudflare, Xunfei, Tencent, VertexAI, etc.
  - They write with `text/event-stream` headers and `c.Render(-1, common.CustomEvent{Data: ...})` in a loop.

A shutdown during these writes will interrupt client delivery. Graceful restart must wait for these handlers to finish.

### 1.3 Billing, logging, and request‑cost reconciliation

These phases run after the streaming is done and are sometimes spawned in background goroutines to avoid blocking the HTTP handler. All of them are critical and must complete before process exit to avoid:

- Unbilled requests (quota not consumed)
- Inconsistent user/channel aggregates
- Stale provisional per‑request cost entries

Key locations:

- File `relay/controller/text.go`
  - After `DoResponse()`, billing is triggered in a goroutine:
    - `postConsumeQuota(ctx, usage, meta, ...)` → calculates final quota
    - `model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota)` → reconcile provisional cost
    - Timeout monitoring with `config.BillingTimeoutSec`
  - Also calls `billing.ReturnPreConsumedQuota(...)` on early error paths.
- File `relay/controller/response.go`
  - Mirrored logic for the Response API:
    - Pre‑consume estimate and provisional cost record
    - On success, `postConsumeResponseAPIQuota(...)/billing.PostConsumeQuotaDetailed(...)`
    - Reconcile per‑request cost with `UpdateUserRequestCostQuotaByRequestID`
    - `billing.ReturnPreConsumedQuota(...)` on errors
- File `relay/controller/claude_messages.go`
  - Same patterns: provisional cost, usage extraction, `billing.PostConsumeQuotaDetailed(...)` → finalization.
- File `relay/billing/billing.go`
  - `PostConsumeQuotaDetailed(detail)` and `PostConsumeQuotaWithLog(ctx, ...)`:
    - DB operations:
      - `model.PostConsumeTokenQuota(tokenId, quotaDelta)`
      - `model.UpdateUserUsedQuotaAndRequestCount(userId, totalQuota)`
      - `model.UpdateChannelUsedQuota(channelId, totalQuota)`
      - `model.RecordConsumeLog(ctx, logEntry)`
      - `model.CacheUpdateUserQuota(ctx, userId)`
  - `ReturnPreConsumedQuota(ctx, preConsumedQuota, tokenId)` uses a background goroutine.

All bullets above can be interrupted if the process exits before goroutines finish or before the handler completes.

### 1.4 Retry/suspension side‑effects

- File `controller/relay.go`
  - Long retry loop over channels. On first failure it spawns `go processChannelRelayError(...)` to perform suspension/disable logic and logging.
  - If interrupted mid‑execution, channel suspension states or alerts may not be recorded consistently.

### 1.5 Background processes started in `main.go`

While not strictly part of a single request, coordinated shutdown should also cancel and join the following to avoid resource leaks during restart:

- Option & channel cache sync loops: `model.SyncOptions`, `model.SyncChannelCache`
- Channel auto‑tests: `controller.AutomaticallyTestChannels`
- Batch updater: `model.InitBatchUpdater` → stopped via `model.StopBatchUpdater(ctx)` during shutdown
- Prometheus DB/Redis monitoring initialization

The batch updater is particularly critical as it holds uncommitted quota changes in memory. During graceful shutdown, `StopBatchUpdater` signals the updater to perform a final flush before exiting. This is registered as a critical task via `graceful.GoCritical` to ensure the drain process waits for completion.

## 2) Observed gaps in current startup/shutdown

- File `main.go`
  - Uses `server.Run(":"+port)`. This starts an internal `http.Server` but offers no way to call `Shutdown(ctx)` on signals.
  - No signal handling; Docker `SIGTERM` will kill the process without waiting for handlers/goroutines.
  - No in‑flight request accounting. SSE + post‑billing goroutines are not tracked.

Conclusion: As of now, restarts can interrupt streaming, billing, and logging, causing broken client responses and unbilled or partially billed requests.

## 3) Requirements for Graceful Restart

- Stop accepting new requests when a shutdown signal is received.
- Allow all in‑flight requests to finish streaming and finalize billing/logging.
- Bound the maximum wait (configurable; recommend minutes) to avoid hanging forever (e.g., upstream stuck streams).
- Prefer not to cancel billing goroutines; instead, wait for them to complete.
- Optionally switch readiness/health to “not ready” immediately to drain from load balancers.

## 4) Design: Draining Mode + In‑Flight Tracking + Server Shutdown

### 4.1 Components

- HTTP server with explicit lifecycle:
  - Replace `server.Run(...)` with `http.Server{Handler: server}` and `srv.ListenAndServe()` in a goroutine.
  - Capture `SIGTERM`/`SIGINT`, then:
    - Set “draining mode” (optional middleware to refuse new traffic or rely on `Server.Shutdown`).
    - Call `srv.Shutdown(ctx)` to stop accepting new connections and wait for handlers to return.
- Global Lifecycle Manager (singleton):
  - `InFlight` counter + `sync.WaitGroup` for critical sections.
  - `BeginRequest()`/`EndRequest()` in a middleware around each relay request.
  - `GoCritical(fn)` helper wraps background goroutines (billing, error processing, pre‑consume refund) and ties them to the WaitGroup.
  - `Drain(ctx)` waits for `InFlight == 0` and WaitGroup to reach zero, then returns.
- Tie places that spawn goroutines to the lifecycle manager:
  - `billing.ReturnPreConsumedQuota(...)`
  - Billing goroutines in `text.go`, `response.go`, `claude_messages.go`
  - `processChannelRelayError(...)`

With this, `Server.Shutdown` waits for the request handler goroutine(s) to return (which includes SSE streaming). Our `LifecycleManager.Drain` additionally waits for post‑handler critical goroutines to finish before allowing the process to exit.

### 4.2 Shutdown ordering

1. Receive signal → mark draining → optionally flip readiness/health.
2. `srv.Shutdown(ctx)` stops new connections and waits for handlers to return.
3. `LifecycleManager.Drain(ctx)` waits for post‑handler critical goroutines (billing/refund/logging/error‑processing) to finish.
4. Close DB/Redis only after Step 3 so in‑flight billing DB ops complete.

### 4.3 Timeouts

- Suggested envs:
  - `SHUTDOWN_TIMEOUT_SEC` (default: 360s ~ 6 min)
  - `BILLING_TIMEOUT_SEC` already exists; keep per‑request guard, but don’t cancel billing early due to shutdown.
- If the global shutdown timeout elapses, log a critical error including remaining in‑flight counts; then exit.

## 5) Implementation Guide (Step‑by‑Step)

This section provides concrete edits and touch points. Keep error handling with `github.com/Laisky/errors/v2`, use request‑scoped logger via `gmw.GetLogger(c)`, and UTC time per project style.

### 5.1 Add a lifecycle manager

Create `common/graceful/graceful.go`:

- Expose:
  - `func BeginRequest() func()` — increments in‑flight; returns a `done()` to defer in the handler.
  - `func GoCritical(ctx context.Context, name string, fn func(context.Context))` — increments WG and runs `fn`; decrements on finish; logs start/end/errors.
  - `func Drain(ctx context.Context) error` — waits for WG to zero; logs remaining tasks on timeout.
  - `func SetDraining()` / `func IsDraining() bool` (optional; to proactively reject new requests with 503).

- Export a metric/gauge for `in_flight_requests` and `in_flight_critical_tasks` if desired.

### 5.2 Wrap relay requests

- Add a middleware in `router/relay.go` (before `middleware.Distribute()`):
  - `graceful.BeginRequest()` at entry; defer the returned `done()` at the end of the handler stack.
  - If `IsDraining()` is true, you may immediately 503 new relay requests; alternately rely solely on `Server.Shutdown` to refuse new connections.

### 5.3 Wrap critical goroutines

- In `controller/relay.go` where `go processChannelRelayError(...)` is launched, replace with:
  - `graceful.GoCritical(gmw.BackgroundCtx(c), "processChannelRelayError", func(ctx context.Context) { processChannelRelayError(ctx, ...) })`

- In `relay/controller/text.go` and `relay/controller/response.go`:
  - Replace the bare `go func(){ ... }()` post‑billing goroutines with `graceful.GoCritical(...)` wrappers.
  - For `billing.ReturnPreConsumedQuota(...)`, either:
    - Change it to accept a `graceful.GoCritical` callback; or
    - Move the `go` inside callers and invoke `ReturnPreConsumedQuota` synchronously in the spawned critical goroutine.

- In `relay/billing/billing.go`: update `ReturnPreConsumedQuota` to not spawn a goroutine itself. Let callers launch it under lifecycle control to avoid hidden background work.

### 5.4 Replace `server.Run` with managed `http.Server`

- In `main.go`:
  - Build `srv := &http.Server{Addr: ":"+port, Handler: server}`
  - Start `go func(){ _ = srv.ListenAndServe() }()`
  - Capture `SIGTERM|SIGINT` in a signal.Notify channel.
  - On signal:
    - Log “draining start” with counts.
    - Optionally set draining flag.
    - Call `srv.Shutdown(ctx)` with `SHUTDOWN_TIMEOUT_SEC`.
    - Then call `graceful.Drain(ctx)` (same or slightly smaller timeout).
    - After successful drain, close DB (`model.CloseDB()`) and other resources.

### 5.5 Readiness/Health integration

- Health: `/api/status` is used in Docker `HEALTHCHECK`. Optionally make it return non‑200 when `IsDraining()` to speed up load balancer drain.
- Readiness probe (Kubernetes): add a `/ready` endpoint or reuse `/api/status` to reflect draining state.

### 5.6 Configuration & Docker/K8S notes

- Docker Compose: set a sufficiently long `stop_grace_period` (e.g., 6–10 minutes) to cover long SSE.
- Dockerfile: current `HEALTHCHECK` hits `/api/status`. Keep it; during drain it can be flipped to failure to stop routing.
- Kubernetes: configure `terminationGracePeriodSeconds` (e.g., 600), preStop hook (sleep or `curl /drain` to flip readiness), and readiness probe using `/api/status`.

## 6) Edge Cases & Correctness Notes

- Client‑cancelled streams: Billing paths already use `gmw.BackgroundCtx(c)` to avoid being cancelled by client abort. With lifecycle tracking, we still wait for billing completion during shutdown.
- Billing timeouts: Keep per‑request `BillingTimeoutSec`. On shutdown, do not forcibly cancel these; allow them to finish or timeout naturally.
- Retry loop mid‑flight: If shutdown occurs while `Relay` is retrying channels, `Server.Shutdown` will let the current handler return; we must still run `processChannelRelayError` and billing cleanup under `GoCritical`.
- Pre‑consume refunds: Must be tracked as critical tasks; otherwise refunds may be lost.
- DB closing: Close DB after `Drain()` so final writes succeed.
- Idempotency: `UpdateUserRequestCostQuotaByRequestID` is idempotent per `(user_id, request_id)` and safe to retry. This helps correctness even if we re‑attempt after restarts (future improvement: a dead‑letter/retry queue for billing failures is noted as TODO in code).

## 7) Validation Plan

- Unit: add tests to simulate long billing and ensure `Drain()` waits.
- Integration: start server, initiate a streaming request that lasts N seconds, send SIGTERM, verify client receives full stream and billing/log rows are persisted before process exits.
- Observability: log at INFO level when draining begins/ends; export in‑flight gauges; alert on forced shutdown with pending tasks.

## 8) Mapping: Code Touch Points Summary

- Request tracking middleware (new):
  - `router/relay.go` — wrap relay routes with Begin/End.
- Replace background goroutines with lifecycle‑managed ones:
  - `controller/relay.go` — `processChannelRelayError` launcher
  - `relay/controller/text.go` — post‑billing goroutine, refund goroutine
  - `relay/controller/response.go` — post‑billing goroutine, refund goroutine
  - `relay/controller/claude_messages.go` — post‑billing goroutine
  - `relay/billing/billing.go` — make `ReturnPreConsumedQuota` synchronous; move `go` to callers
- Graceful server shutdown:
  - `main.go` — explicit `http.Server`, signal handling, `Shutdown`, `graceful.Drain`, then close DB/Redis.

## 9) Rollout Strategy

- Phase 1: Land lifecycle manager + wrappers; keep functional behavior otherwise.
- Phase 2: Replace `server.Run` with managed `http.Server` and add signal handling.
- Phase 3: Add readiness flip during draining and container grace periods.
- Phase 4: Add tests and dashboards for in‑flight tracking and forced shutdown metrics.

## 10) Future Enhancements

- Dead‑letter queue for billing/logging failures (already hinted by TODOs) to guarantee eventual consistency across restarts even if DB hiccups occur during shutdown.
- Persist in‑flight request metadata in Redis to survive node termination (optional).
- Per‑request max execution time guard for pathological upstream streams.

---

Appendix A — Quick Code Snippets (for reference only; implementation belongs in code, not in docs)

- Server scaffold:

```go
srv := &http.Server{Addr: ":"+port, Handler: server}
go func() {
    if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        logger.Logger.Fatal("listen failed", zap.Error(err))
    }
}()
// on signal:
_ = srv.Shutdown(ctx)
_ = graceful.Drain(ctx)
```

- Lifecycle wrappers:

```go
end := graceful.BeginRequest()
defer end()

// instead of: go processChannelRelayError(...)
graceful.GoCritical(gmw.BackgroundCtx(c), "processChannelRelayError", func(ctx context.Context) {
    processChannelRelayError(ctx, userId, channelId, channelName, group, model, err)
})

// instead of: go func(){ postConsume...; UpdateUserRequestCostQuota... }()
graceful.GoCritical(gmw.BackgroundCtx(c), "postBilling", func(ctx context.Context) {
    quota := postConsumeQuota(ctx, usage, meta, req, ratio, preConsume, modelRatio, groupRatio, sysReset, completionRatio)
    if quota != 0 { _ = model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota) }
})
```

This plan ensures the process restarts only after all critical work per request is finished.
