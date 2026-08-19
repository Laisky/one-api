# Proposal: Systematic Remediation of Async/Sync Races in the Relay & Billing Path

- Status: Implemented (P1–P5 landed; see §10)
- Author: @Laisky
- Created: 2026-06-08
- Owners: backend
- Scope: `controller/*`, `relay/controller/*`, `common/graceful`, `common/relayctx`, billing path

> This started as a **plan** for a systematic, repo-wide remediation: it defines the
> bug class, the target-state design, the guardrails, and a phased work breakdown.
> The remediation has since been implemented — see **§10 Implementation Status** for
> what landed and where. The plan sections below are preserved as the design rationale.

## 1. Problem Statement

The relay/billing subsystem launches background work (`graceful.GoCritical` and
raw `go func`) that closes over the request's `*gin.Context` `c` and continues to
read/write request-scoped state **after the HTTP handler has returned**. Two
hazards follow from this, both observed in production or proven in tests:

1. **Sync-reader vs async-writer race.** A synchronous reader (a deferred function,
   or code right after the spawn) observes state that is only written inside the
   spawned goroutine. Example: the deferred `billingAuditSafetyNet(c)` reads
   `ctxkey.BillingReconciled` while the refund goroutine sets it → false-positive
   "CRITICAL BILLING AUDIT … manual reconciliation required" alarm, and a symmetric
   double-refund window.

2. **`*gin.Context` use-after-return.** gin `v1.12.0` recycles `*gin.Context` via
   `sync.Pool` in `ServeHTTP`; the next request gets the same `c` and calls
   `c.reset()` (clearing `c.Keys`) and `c.Set(...)`. A background goroutine still
   calling `c.GetInt(...)`/`c.Set(...)`/`gmw.GetLogger(c)` then races that map —
   which can **bill the wrong user/token** or crash the process with
   `fatal error: concurrent map read and map write`.

Point fixes do not close the class: the pattern is pervasive and the most common
form is **indirect** — a goroutine that never names `c.Get` but calls
`gmw.BackgroundCtx(c)` / `gmw.GetLogger(c)`, both of which read `c` internally.

### 1.1 Environment facts (verified, load-bearing for this plan)

- gin `v1.12.0` pools and `reset()`s `*gin.Context` after `ServeHTTP` returns.
- **No `c.Copy()` exists anywhere** in non-test code.
- `gmw.BackgroundCtx(c)` detaches cancellation **but stores `c` itself** via
  `context.WithValue(ctx, CtxKeyGin, c)`; `gmw.GetLogger(ctx)` / `gmw.TraceID(ctx)`
  therefore still dereference `c`. So "use `BackgroundCtx` instead of `Ctx`" fixes
  cancellation (refund-loss) but **not** the use-after-return on `c.Keys`.
- The lifecycle tracker `graceful.GoCritical` waits for goroutines only at
  shutdown drain; it establishes **no** happens-before with gin's per-request pool
  recycle.

### 1.2 Why now / impact

- Correctness: silent mis-billing (wrong user/token), lost refunds, and
  false-positive CRITICAL alarms that erode trust in the audit signal.
- Stability: a realistic path to a process-fatal concurrent-map panic under load.
- Drift: without a guardrail, every new relay handler re-introduces the pattern by
  copy-paste (the canonical `postBilling` closures already model the unsafe shape).

## 2. Goals / Non-goals

### Goals

- Establish **one safe, ergonomic way** to run request-scoped background work, such
  that touching a live `*gin.Context` from a goroutine is impossible-by-construction
  (or at least lint-blocked).
- Produce an **exhaustive inventory** of every goroutine that captures `c`, and
  remediate all of them.
- Add a **regression guardrail** (static check + CI `-race`) so the class cannot
  return.
- Hold the line on the project rule: every behavioral fix ships with a
  **deterministic test that fails before and passes after** (reproduce-first, to
  avoid false positives).

### Non-goals

- Changing billing math, pricing, or the conservative no-underbilling policy.
- Rewriting the realtime websocket transport beyond its goroutine/`c` usage.
- Upgrading gin or replacing it; the fix must work with the pooled-context model.
- Performance work, except to keep the safe pattern allocation-cheap.

## 3. Target-State Design

The plan standardizes on a layered rule: **a goroutine must never hold a live
`*gin.Context`.** It receives (a) a *detached* context that carries only copied
logger/trace values, and (b) explicitly snapshotted scalars.

### 3.1 Two safe primitives (to be built in Phase 2)

1. **Detached context constructor that does NOT retain `c`.** Today
   `gmw.BackgroundCtx(c)` retains `c`. We will introduce a project-side wrapper that
   snapshots the logger and trace id into a fresh `context.Background()` *without*
   storing the gin context, and use it at every spawn boundary. Sketch:

   ```go
   // common/relayctx (new): a background context safe to hand to goroutines —
   // it carries the request's logger and trace id by value and never references *gin.Context.
   func Detach(c *gin.Context) context.Context {
       ctx := context.Background()
       ctx = gmwlog.SetLogger(ctx, gmw.GetLogger(c)) // logger snapshot, not c
       if tid, err := gmw.TraceID(c); err == nil {
           ctx = context.WithValue(ctx, gutils.TracingKey, tid)
       }
       return ctx
   }
   ```

   (If upstream `gin-middlewares` can be changed instead, prefer fixing
   `BackgroundCtx` not to store `c`; decision recorded in Phase 2.)

2. **A spawn helper that forces snapshotting.** Make the safe path the path of
   least resistance:

   ```go
   // graceful.GoRequestScoped runs fn in a tracked goroutine with a detached ctx.
   // fn receives ONLY the detached context; capturing *gin.Context in fn is a lint error.
   func GoRequestScoped(c *gin.Context, name string, fn func(ctx context.Context)) {
       ctx := relayctx.Detach(c)
       GoCritical(ctx, name, fn)
   }
   ```

   Billing helpers that currently read `c` inside the goroutine
   (`returnPreConsumedQuotaConservative`, `postConsume*`) are refactored to accept
   their inputs as **value params captured on the request goroutine** (token id,
   user id, provisional-log id, forwarded flag), removing all in-goroutine `c`
   access.

### 3.2 The canonical safe shape (target for every site)

```go
// On the request goroutine: snapshot everything the background work needs.
snap := billingSnapshot{
    userID:    c.GetInt(ctxkey.Id),
    tokenID:   c.GetInt(ctxkey.TokenId),
    provLogID: c.GetInt(ctxkey.ProvisionalLogId),
    forwarded: shouldSkipPreConsumedRefund(c),
    requestID: c.GetString(ctxkey.RequestId),
}
markBillingReconciled(c)                 // sync state the deferred reader depends on
graceful.GoRequestScoped(c, "postBilling", func(ctx context.Context) {
    doBilling(ctx, snap)                 // no c, no gmw.*(c), no closure over c
})
```

Invariants the design enforces:

- All state a synchronous/deferred reader depends on is written **before** the spawn.
- The goroutine closes over **no** `*gin.Context`.
- Post-return work uses a **non-cancelled, c-free** context.

## 4. Scope & Inventory

Current rough scope (to be made exhaustive in Phase 1):

| Surface | Count (non-test) | Notes |
|---------|------------------|-------|
| `graceful.GoCritical` call sites | ~20 | across 14 files in `relay/controller`, `controller`, `model` |
| raw `go func` goroutines | ~36 | includes nested goroutines inside `GoCritical` closures |
| `gmw.{Ctx,BackgroundCtx,GetLogger}(c)` references in relay path | ~163 | a subset are *inside* goroutines (the indirect form) |
| Billing helpers reading `c` internally | `returnPreConsumedQuotaConservative`, `ResetPerAttemptBillingForRetry`, `postConsume*` | called from refund/post-billing goroutines |

Known hotspot files to classify first: `relay/controller/{response,response_fallback,response_ws,claude_messages,text,ocr,rerank,audio,video}.go`,
`relay/controller/billing_safety.go`, `controller/relay.go`,
`controller/realtime.go`, `controller/realtime_billing.go`, `model/utils.go`.

**Phase 1 deliverable:** a complete table — one row per goroutine — with columns
`{file:line, spawn kind, captures c?, c reads/writes inside, sync/deferred reader,
classification (real-race | refund-loss | safe), proposed fix}`. Build it with a
multi-lens parallel scan + adversarial verification (same harness used for the
precedent audit), so "safe" rows are positively justified, not merely unflagged.

## 5. Guardrails (regression prevention)

1. **Static check.** An `ast-grep` rule (repo-preferred) that flags any
   `c.Get*` / `c.Set` / `gmw.*(c)` reference that is lexically inside a `go` or
   `GoCritical` closure capturing `c`. Run in CI and as a pre-commit hook. Seed
   pattern (to be refined):

   ```
   ast-grep --lang go -p 'graceful.GoCritical($CTX, $NAME, func($P){ $$$; c.$M($$$); $$$ })'
   ```

   Escalate to a small `golang.org/x/tools/go/analysis` analyzer if the lexical rule
   proves too coarse (it can track that the closure-captured identifier is the
   handler's `*gin.Context`).

2. **CI `-race`.** Gate `go test -race ./controller/... ./relay/controller/...` in CI
   (already passing locally) so any reintroduced race fails the build.

3. **Convention doc + review checklist.** Add the "goroutines never hold `*gin.Context`"
   rule to the contributor docs and PR checklist; point new code at
   `graceful.GoRequestScoped`.

## 6. Phased Work Breakdown

Each phase is independently shippable and ends with build + vet + `-race` green.
Delegation: fan out read-only inventory/verification to sub-agents; run migrations
as sequential per-file reproduce→fix→verify tasks (one file per agent) to avoid
shared-tree build collisions.

- **P0 — Precedent (DONE, see Appendix A).** Validates the snapshot/`BackgroundCtx`
  patterns and the deterministic gate-based test harness. No further action.
- **P1 — Exhaustive inventory.** Produce the §4 table; adversarially verify every
  "safe" classification. Output: prioritized backlog (real-race > refund-loss >
  defensive).
- **P2 — Safe primitives.** Build `relayctx.Detach` (or fix `gmw.BackgroundCtx`) and
  `graceful.GoRequestScoped`; refactor billing helpers to take value-param
  snapshots instead of reading `c`. Unit-test the primitives (incl. "detached ctx
  does not retain `c`" and "survives request-ctx cancellation").
- **P3 — Migration waves.** Convert all inventoried sites to the canonical shape,
  one file per task, each with a reproduce-first test where a real race/refund-loss
  exists (reuse the gate+observe harness). Order: billing-critical paths first
  (`response*`, `claude_messages`, `text`, `ocr`, `rerank`, `audio`, `video`,
  `realtime*`), then non-billing goroutines.
- **P4 — Guardrail.** Land the `ast-grep`/analyzer rule + CI `-race` gate + docs.
  This must come after P3 so the rule starts green.
- **P5 — Cleanup & hardening.** Remove now-redundant in-goroutine `c` reads; add a
  concurrency stress integration test (many concurrent requests with `-race`) that
  exercises pool recycling; consider async-refund failure auditing (so marking
  reconciled before the async refund cannot hide a later DB failure).

## 7. Test Strategy

- **Reusable deterministic harness.** Generalize the gate + observation seam used in
  P0 (`*ForTest` package vars: a `chan struct{}` to park the goroutine, a
  `chan`/`func` to record the value/context it actually used). Each migration
  asserts on the **observed value/context** (deterministic), never on the `-race`
  detector firing — releasing a gate creates a happens-before edge that would hide a
  real race.
- **Per-finding tests** (reproduce-first): parks the goroutine, mutates `c` /
  cancels the request ctx / rewrites the shared field, then asserts the goroutine
  used the spawn-time snapshot.
- **Stress test** (P5): a `-race` test issuing many concurrent requests against a
  test router so the gin pool actually recycles contexts under a live goroutine.
- **Guardrail test**: a fixture that violates the rule and asserts the static check
  flags it.

## 8. Acceptance Criteria (program-level)

1. Inventory (P1) is complete and every goroutine is classified real-race /
   refund-loss / safe, with each "safe" positively justified.
2. `relayctx.Detach`/`GoRequestScoped` exist, are unit-tested, and **no production
   goroutine closes over `*gin.Context`** nor calls `gmw.*(c)`/`c.*` inside a
   goroutine (verified by the guardrail).
3. Every real-race / refund-loss site has a reproduce-first test that failed before
   its fix and passes after, under `-race`.
4. `go build ./...`, `go vet ./...`, and `go test -race ./controller/... ./relay/controller/...`
   are green in CI.
5. The static guardrail is enabled in CI and pre-commit and starts green.
6. No change to billing math or the conservative no-underbilling policy; the
   reconcile flag is set no later than any refund spawn on every path.

## 9. Risks, Sequencing & Ownership

| Risk | Mitigation |
|------|------------|
| Large surface (~56 goroutines) → long migration | Phase by blast radius; billing-critical first; one file per task; reuse harness. |
| `gmw.BackgroundCtx` is upstream code | P2 decides: wrap locally (`relayctx.Detach`) vs upstream patch; default to local wrapper to stay self-contained. |
| Lexical `ast-grep` rule yields false positives/negatives | Start lexical for fast coverage; escalate to a typed `go/analysis` analyzer if needed. |
| Marking reconciled before async refund can mask async failures | P5 adds async-refund failure auditing; matches existing success-path semantics in the interim. |
| Behavioral drift during refactor | Reproduce-first tests + `-race` per phase; no phase merges red. |

Sequencing: **P1 → P2 → P3 → P4 → P5** (P4 strictly after P3). Suggested ownership:
backend, with the inventory/verification fanned out to sub-agent workflows and the
primitives/guardrail owned by one maintainer for consistency.

## Appendix A — Precedent (already merged; pattern reference only)

The following point fixes already demonstrate the patterns this plan generalizes
(each shipped with a deterministic reproduce-first `-race` test):

- `scheduleConservativeRefund` — marks `BillingReconciled` synchronously before the
  refund goroutine (fixes the original false-positive CRITICAL alarm); adopted by 12
  refund call sites.
- `controller/relay.go` `goProcessChannelRelayError` — snapshots `*bizErr` before
  spawn (fixes the `bizErr.Error.Message` data race).
- `response_ws.go` — captures `tokenID`/`quotaId` before spawn (fixes two
  `c.GetInt(...)`-in-goroutine reads).
- `video.go` / `audio.go` — rollback refund moved to `gmw.BackgroundCtx(c)` (fixes
  request-ctx-cancellation refund-loss).

These remain examples, not the goal. The **indirect** form (`gmw.BackgroundCtx(c)` /
`gmw.GetLogger(c)` inside goroutines, and billing helpers reading `c`) is unaddressed
and is precisely what Phases 1–5 exist to eliminate.

## Appendix B — Definitions

- **Async-writer vs sync-reader race**: shared state written only inside a spawned
  goroutine and read synchronously (deferred or post-spawn) without happens-before.
- **Use-after-return**: any access to `*gin.Context` (directly or via
  `gmw.*(c)`) after the handler returns and gin may have pooled/reset it.
- **Snapshot**: copying needed values onto the request goroutine before spawning, so
  the goroutine depends on no live request object.

## 10. Implementation Status

All phases landed. Summary of what shipped and where:

### P2 — Safe primitives (`common/relayctx`)

- `relayctx.Detach(c) context.Context` — a **non-cancelled, c-free** background context
  that carries the request logger and trace id **by value**. Unlike
  `gmw.BackgroundCtx(c)` (which stores `c` under `CtxKeyGin`), `Detach` never retains
  `c`, so `gmw.GetGinCtxFromStdCtx(Detach(c))` returns `(nil, false)` and no goroutine
  can dereference the recycled context through it.
- `relayctx.GoRequestScoped(c, name, fn)` — spawns a tracked critical goroutine with a
  `Detach`ed context; `fn` receives only that context.
- Unit-tested in `common/relayctx/relayctx_test.go`: does-not-retain-`c`, logger
  snapshot, survives request-ctx cancellation, carries trace id.

### P3 — Migration of every billing goroutine

- **Refund path** (`relay/controller/billing_safety.go`): introduced
  `conservativeRefundSnapshot` (skipRefund/userID/tokenID/provisionalLogID/requestID/
  reason/quota), captured on the request goroutine. `scheduleConservativeRefund` now
  marks reconciled synchronously, snapshots, and spawns on `relayctx.Detach(c)` (fixes
  the refund-loss from the old `gmw.Ctx(c)` and the WS regression). The synchronous
  `returnPreConsumedQuotaConservative` (≈30 call sites) is unchanged at the signature
  level but reuses the snapshot internally (DRY). The async refund now uses
  `model.PostConsumeTokenQuota` directly and emits a **CRITICAL audit** on DB failure,
  so marking reconciled up front can no longer hide a lost async refund.
- **Post-billing path**: `detachForBilling(c)` (`relay/controller/billing_ctx.go`)
  returns `relayctx.Detach(c)` plus a `billingIdentity` snapshot (request id /
  provisional log id / trace id / tool summary). `postConsume*` resolve identifiers via
  `billingIdentityFromContext(ctx)` (snapshot first, embedded-gin fallback for sync
  callers) instead of `gmw.GetGinCtxFromStdCtx(ctx)` inside the goroutine. Migrated:
  `response.go`, `response_fallback.go` (×2), `text.go` (×2), `claude_messages.go`,
  `ocr.go`, `rerank.go`, `response_ws.go`, plus `audio.go`/`video.go` post-billing and
  `controller/realtime.go` + `controller/realtime_billing.go`.
- **Shared rollback helper**: `goRollbackPreConsumed` de-duplicates the audio/video
  rollback refund; both wrappers pass their own test seams.
- **Streaming WS bridge** (`relay/adaptor/openai/response_api_ws_transport.go`):
  captures `c.Request.Context()` before the bridge goroutine instead of reading it
  inside.
- **Proxy** (`relay/controller/proxy.go`): the previously **untracked** `go func()` that
  re-derived `gmw.BackgroundCtx(c)` inside the closure is now
  `relayctx.GoRequestScoped`.

### P4 — Guardrail (regression prevention)

- Two complementary `ast-grep` rules in `.ast-grep/rules/`:
  - `no-gin-context-in-goroutine.yml` flags any reference to the identifier `c` inside a
    goroutine body (`go func`, `graceful.GoCritical`, `relayctx.GoRequestScoped`,
    `runPostBillingWithTimeout`, `goDetachedBillingWork`). Spawn arguments
    (`relayctx.Detach(c)`/`detachForBilling(c)`) are outside the closure and not flagged.
  - `no-gin-context-as-spawn-arg.yml` flags `gmw.Ctx(c)`/`gmw.BackgroundCtx(c)` passed
    DIRECTLY as a spawn helper's context argument (the gap the first rule cannot see).
  Both start **green** on the current tree.
- **Known lexical limitation** (documented in both rules): the *aliased* form
  `ctx := gmw.Ctx(c); graceful.GoCritical(ctx, ...)` is invisible to `ast-grep` (it needs
  data-flow). This is intentionally backstopped by the CI `-race` gate (which catches the
  runtime race regardless of how it is introduced) plus the "always pass
  `relayctx.Detach(c)`/`detachForBilling(c)` at the spawn site" convention + review.
- Self-tested via `.ast-grep/rule-tests/` (`ast-grep test --skip-snapshot-tests`).
- Wired into `make lint` (`lint-goroutine-guard`, skips if ast-grep absent) and a CI job
  in `.github/workflows/lint.yml`. CI installs ast-grep from a **checksum-pinned prebuilt
  binary** (no npm — project policy), not `npm i -g`. The repo's existing
  `go test -race ./...` CI job is the `-race` gate.

### P5 — Hardening

- Async-refund failure auditing (above) closes the "marking reconciled hides a later DB
  failure" gap.

### P6 — Post-review hardening (timeout-bounded + lifecycle-tracked background billing)

A review of the P1–P5 landing surfaced two correctness gaps in the *lifecycle* of the
detached billing goroutines (not the c-safety, which P2–P3 closed). Both are now fixed:

- **Post-billing timeout no longer orphans the worker.** `runPostBillingWithTimeout`
  (`relay/controller/post_billing.go`) previously returned from the tracked
  `graceful.GoCritical` closure the instant the timeout branch ran, leaving the inner
  `work` goroutine running **untracked** — graceful shutdown could exit out from under an
  in-flight billing DB write. The closure now **joins** the worker (`<-done`) on BOTH
  select branches, so it stays tracked by `graceful.Drain`. The misleading "billing must
  still complete" comment is corrected: the timeout **cancels the billing attempt** (work
  runs on the timeout-bounded ctx); post-consume only reconciles an already-charged
  estimate, so an aborted attempt is an un-reconciled estimate (visible via the CRITICAL
  BILLING TIMEOUT audit + the DLQ TODO), never an un-charged request.
- **Refund/rollback goroutines are now timeout-bounded.** A new shared primitive
  `goDetachedBillingWork(spawnCtx, name, fn)` runs `fn` directly in a tracked
  `graceful.GoCritical` closure bounded by `config.BillingTimeoutSec`, so a stuck billing
  DB can neither leak a goroutine/connection nor extend graceful drain past the deadline.
  `scheduleConservativeRefund`, `goRollbackPreConsumed`, `ResetPerAttemptBillingForRetry`,
  and `billingAuditSafetyNet`'s emergency refund all route through it. The primitive is
  registered in `no-gin-context-in-goroutine.yml`.
- **Synchronous deferred billing paths audited & documented.** `relay/controller/image.go`
  retains `gmw.BackgroundCtx(c)` for its image post-billing/refund, but that runs in a
  SYNCHRONOUS defer (on the request goroutine, before `ServeHTTP` returns and gin recycles
  `c`) — it is NOT the async race class. A NOTE there documents this and warns against
  copying the pattern into a goroutine.

### Reproduce-first tests (fail-before / pass-after, under `-race`)

- `billing_safety_race_test.go`: `…_MarksReconciledSynchronously` (existing) and the new
  `…_DetachedSnapshot` (verified to fail with `gmw.Ctx(c)`: `context canceled`).
- `billing_ctx_test.go`: snapshot survives `c` mutation; sync-fallback path.
- `relayctx_test.go`: the primitive's invariants.
- Existing audio/video rollback + WS post-billing + bizErr tests continue to pass.

P6 reproduce-first / behavioral additions (all under `-race`):

- `post_billing_test.go`: `…_DrainWaitsForInnerGoroutineAfterTimeout` — verified to FAIL
  against the pre-fix orphaning path (Drain returns while work is in flight) and PASS with
  the join.
- `billing_timeout_bound_test.go`: `…_RefundBoundedByBillingTimeout` — verified to FAIL
  against the pre-fix unbounded spawn (refund ctx error stays nil) and PASS with the bound
  (ctx hits `DeadlineExceeded`); plus `…_DrainWaitsForParkedRefund` (lifecycle).
- `rollback_accounting_test.go`: audio/video rollback **DB accounting** (refunds exactly
  once; survives client disconnect; concurrent mix nets to start).
- `billing_audit_accounting_test.go`: safety-net emergency refund **DB accounting**
  (not-forwarded refunds; forwarded keeps; reconciled is a no-op).
- `billing_crosspath_stress_test.go`: multi-round cross-path concurrency (success
  post-billing + conservative refund + forwarded skip + rollback) through a real
  `gin.Engine` with per-request cancellation, asserting the **exact** quota balance every
  round and a clean `graceful.Drain`.

## 11. Convention (contributor rule)

**A goroutine must never hold or read the request `*gin.Context`.** gin recycles `c` via
`sync.Pool` the instant the handler returns; `c.Get*`/`c.Set`/`gmw.*(c)` from a goroutine
race the next request and can mis-bill or panic.

Do this instead:

```go
// 1. Snapshot everything the goroutine needs, on the request goroutine.
id := c.GetInt(ctxkey.Id)
// 2. Write any state a deferred/sync reader depends on BEFORE the spawn.
markBillingReconciled(c)
// 3. Spawn with a detached, c-free context; the closure uses ctx, never c.
relayctx.GoRequestScoped(c, "work", func(ctx context.Context) {
    doWork(ctx, id)               // gmw.GetLogger(ctx), not gmw.GetLogger(c)
})
// For billing post-consume, use detachForBilling(c) so request id / provisional log id /
// trace id / tool summary travel with the context.
```

The `ast-grep` guardrail enforces this in CI; run it locally with `make lint-goroutine-guard`.
