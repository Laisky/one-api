# Observability Data Tiering for High-Volume Deployments

- Status: Phases 0 and 1 implemented, verified, and benchmarked (2026-09-05); Phases 2, 3, 4 are
  design-only and are the main subject of this review
- Document revision: 2026-09-06, self-contained edition prepared for an external review panel.
  Compared with the 2026-09-05 edition it adds Part I (system context), §12 (known gaps and
  review questions), and Appendices A–D, and it corrects the internal contradictions listed in
  §0.5.
- Audience: reviewers who have **no access to the repository**. Every claim that cites a code
  location is also explained in words; the citation is an anchor for anyone who later obtains
  access, never a prerequisite for understanding.
- Measured results: companion document `docs/benchmarks/20260905_observability-phase0-phase1.md`
  (submitted together with this document; summarised in Appendix D)
- Baseline tree (before any of this work): commit `397781e1`
- Measured tree (after Phases 0 and 1): working-tree snapshot `11663356cc79` (uncommitted at the
  time of writing; all `path:line` anchors in this document refer to that tree)
- Owners: Backend, database operations, SRE
- Area: Tracing storage, usage-log storage, dashboard aggregation, application log retention
- Target scale: ~1,000,000 registered users, sustained 10,000 relay requests/second
- Related internal documents (not required reading; their relevant content is folded into
  Part I): Request Tracing System Architecture (`docs/arch/tracing_system.md`), Dashboard PRD
  (`docs/arch/dashboard.md`), Logger Architecture (`docs/arch/logger.md`), Compact UUID Storage
  proposal (`docs/proposals/archive/20260715_compact-uuid-storage.md`)

---

## 0. How to Read This Document

### 0.1 Structure

| Part | Sections | What it contains | Who needs it |
| --- | --- | --- | --- |
| **I — Context** | §1 | What the product is, how a request flows through it, what the three observability signals are and where they are stored today, how configuration works, and the engineering rules the design must respect. Written for readers who have never seen the code. | Every external reviewer. Internal readers may skim. |
| **II — The proposal** | §2–§14 | The contract, the measured problem, the design, the work items with their delivery records, the compatibility contract, the configuration surface, expected impact, testing, risks, known gaps, sequencing, references. | Everyone. |
| **III — Appendices** | A–D | Schema reference, implemented interfaces quoted verbatim, glossary, and a summary of the measured results. | Reference while reading Part II. |

### 0.2 Implementation status at a glance

| Phase | Content | Status | Evidence |
| --- | --- | --- | --- |
| 0 | Trace path exclusion, chunked retention sweeps, log-file retention/size/free-disk guards, log sampling and compact per-request line, dashboard caching and range cap | **Implemented, tested, benchmarked** | §5 Phase 0 delivery record, §10.2, Appendix D |
| 1 | In-memory trace recorder, `TraceSink` abstraction, batched SQL writer, OTLP sink, request-end sampling, per-timestamp columns | **Implemented, tested, benchmarked** | §5 Phase 1 delivery record, §10.1, Appendix D |
| 2 | Dashboard rollup tables and incremental aggregator, keyset pagination, index diet, optional usage-log retention | **Design only** | §5 Phase 2 |
| 3 | OTLP application logs, dashboard-grade metrics, reference collector configuration (the OTLP trace sink itself shipped in Phase 1) | **Design only** | §5 Phase 3 |
| 4 | `UsageStore` interface and optional ClickHouse mirror | **Design only** | §5 Phase 4 |

Everything implemented so far is **opt-in**. A deployment that upgrades without changing its
configuration keeps the pre-proposal behavior exactly (§6). The optimizations are enabled
together by setting one environment variable, `OBSERVABILITY_PROFILE=scaled`.

### 0.3 Conventions used in this document

- **Code anchors.** `path/to/file.go:123` names a file and line in the measured tree. The
  surrounding sentence always states what the code does, so the anchor can be ignored.
- **Names in backticks** are, depending on context, environment variables (`TRACE_SINK`),
  database tables and columns (`logs`, `created_at`), Go identifiers (`RecordConsumeLog`), or
  metric names (`oneapi_trace_records_total`). Appendix C resolves every project-specific term.
- **Time units differ between tables, and this matters for every query in this document.**
  `traces.created_at` and all six trace lifecycle timestamps are Unix **milliseconds**.
  `logs.created_at` is Unix **seconds**. All server, database, and API time handling is UTC.
  Date ranges are half-open: `[start, end)` where `end` is 00:00 UTC of the day after the last
  requested day.
- **"Statement"** means one SQL statement sent to the database, i.e. one network round trip on
  PostgreSQL and MySQL. "RPS" means relay requests per second, i.e. requests that reach an
  upstream model provider; management API calls and static asset requests are not counted.
- **"Engine"** means one of the three supported database engines: SQLite, MySQL, PostgreSQL.
  The product runs one code path against all three; engine-specific SQL is called out wherever
  it exists.
- **The three profiles** `standalone`, `scaled`, and `external` are sets of configuration
  defaults, not code paths. §4 defines them; §7 lists every value.

### 0.4 What the panel is asked to evaluate

1. **The framing.** Is the separation into three data planes (account state, usage ledger,
   peripheral telemetry — §2) and three deployment profiles (§4) the right decomposition for a
   product whose default installation must remain a single binary with an embedded database?
2. **Phases 0 and 1 as shipped.** Given the measured results (Appendix D, companion document),
   are the design choices, the opt-in defaults (§6), and the durability trade-offs of the
   batched trace writer (§5 W1.3, §12) acceptable? Are there failure modes the tests and
   benchmarks do not cover?
3. **Phase 2 design.** Are portable rollup tables with a watermark-driven incremental aggregator
   (§5 W2.1–W2.3) the right way to make the dashboard survivable on all three engines, versus
   engine-native features (partitioning, TimescaleDB continuous aggregates) that would break the
   zero-dependency contract? How should late-arriving and reconciled rows (§1.5.2) be handled?
   Are keyset pagination (W2.4) and the index diet (W2.5) safe for the existing UI contracts?
4. **Phases 3 and 4 design.** Is the OTLP-first, OLAP-optional posture (§5 Phase 3, Phase 4)
   correct, and is the ClickHouse mirror with SQL as the billing source of truth (W4.4) the
   right consistency model?
5. **The gaps in §12.** Which of them must be closed before Phase 2 starts, and which are
   acceptable to carry?
6. **The acceptance gates in §10.** Are the 10k RPS thresholds meaningful, and how should they
   be validated given that no automated load rig exists yet?

### 0.5 Corrections made in this revision

The 2026-09-05 edition was written partly before implementation and partly after, and the
compatibility audit of 2026-09-06 changed several defaults after some sections were written.
The following statements in that edition were wrong and are corrected here; reviewers who
have seen the earlier edition should discard the earlier text on these points.

| Earlier statement | Correct statement (verified against the code) |
| --- | --- |
| `LOG_RETENTION_DAYS` default "changed from 0 to 7"; "the one intentional behavior change" | The default is **`0` (disabled) under `standalone`**, `3` under `scaled`, `1` under `external`. No default under `standalone` deletes anything. The cleaner logs one warning at startup when all three file guards are off. |
| `TRACE_EXCLUDED_PATH_PREFIXES` default `/api/status,/metrics,/health,/static,/assets,/favicon` | That list is the `scaled`/`external` default. Under `standalone` the list is **empty** (every path is traced, as before). |
| "`model.DeleteOldLog` now takes a `context.Context`" | `DeleteOldLog(targetTimestamp)` keeps its old signature; the new variant is `DeleteOldLogContext(ctx, targetTimestamp)`, which is what the admin purge endpoint calls. |
| The `logs` table has "15 secondary indexes" | The struct declares **17** (14 single-column, 3 composite); a migrated database carries **18** because a separate migration adds a unique index on `uuid`. |
| Chunked delete uses "`ORDER BY id LIMIT n`" on MySQL and SQLite | MySQL uses a bare `DELETE … LIMIT n`; SQLite uses `DELETE … WHERE rowid IN (SELECT rowid … LIMIT n)`; PostgreSQL uses `ctid IN (SELECT ctid … LIMIT n)`. None orders by `id`. |
| Dropped traces are counted in `one_api_trace_records_dropped_total{reason="queue_full"}` | The counter is **`oneapi_trace_records_total{outcome="dropped_queue_full"}`**, with sibling gauges `oneapi_trace_queue_depth` and `oneapi_trace_queue_capacity`. Same names in Prometheus and OpenTelemetry. |
| The batched writer is "registered through `graceful.GoCritical` so shutdown flushes" | It is **not** registered with the graceful package. `main` calls `tracing.Shutdown` explicitly after the HTTP server stops and before the database handle closes (§5 W1.3). |
| "A debug header forces capture" of a trace | No client-controllable input affects sampling. `tracing.ForceTraceSample` is a server-side hook and currently has no production caller. |
| Rollup aggregation "leader-elected on the existing `IsMasterNode` machinery so multi-node deployments compute once" (implying existing sweeps are master-only) | The existing retention sweepers run on **every node**; nothing in the trace pipeline is master-gated. Leader election is a requirement Phase 2 must add (§12). |
| `TracingMiddleware` "registered at `main.go:194`" | Now `main.go:206`. |
| The load gate "≤ 2 MB/s at `LOG_REQUEST_SAMPLE_RATE=0.1`" | `LOG_REQUEST_SAMPLE_RATE` does not exist; sampling is `LOG_SAMPLE_INITIAL`/`LOG_SAMPLE_THEREAFTER`/`LOG_SAMPLE_TICK_MS` (§5 W0.4). The gate is restated in §10. |
| The "additive, rollback-tolerant storage change" rule is "the compatibility rule this project applies to every additive storage change" | That rule is **defined by this proposal** (§6). The project's written conventions (§1.8) do not state it; the compact-UUID proposal follows the same principle but it has not been promoted to a project-wide rule. |

---

# Part I — System Context

## 1. The System This Proposal Changes

### 1.1 What one-api is

one-api is a self-hosted **LLM API gateway**. A client sends an OpenAI-, Anthropic-, or
Gemini-style request (`/v1/chat/completions`, `/v1/responses`, `/v1/messages`, plus
embeddings, images, audio, video, rerank, OCR, realtime, and MCP endpoints) with an API key the
gateway issued. The gateway authenticates the key, picks an upstream provider account
("channel") that can serve the requested model, converts the request into that provider's wire
format, forwards it, converts the response back (streaming or not), bills the caller's quota,
and records what happened. Around that relay path sits a multi-tenant control plane with a
React admin and user interface: users, groups, API tokens with their own quotas and model
allow-lists, channel management with health testing and weighted routing, pricing per model
and per group, redemption codes, and the usage pages and dashboard this proposal is about.

The product is one Go binary with the built web frontends embedded in it. Roughly 55 upstream
provider adaptors live under `relay/adaptor/`. The published Docker image starts with no
configuration at all on an embedded SQLite database.

Core entities, one line each (Appendix C has the full glossary):

| Entity | Meaning | Storage |
| --- | --- | --- |
| **User** | An account with a role (`guest`, `common`, `admin`, `root`), a status, a granted quota balance and a used-quota counter, and a **group** name. | `users` table |
| **Group** | A named pricing/routing tier carried on users and channels; a per-group ratio multiplies every price. | string column on `users` and `channels` |
| **Token** (API key) | A user-owned credential (`sk-…`) with its own remaining/used quota, expiry, optional model allow-list, and optional IP restriction. Every relay request is made with a token. | `tokens` table |
| **Channel** | One configured upstream provider endpoint: provider type, credential, base URL, supported models, per-channel pricing, priority, weight, rate limit, health status. | `channels` table |
| **Ability** | A denormalized routing row per `(group, model, channel)` used to filter candidate channels. | `abilities` table |
| **Quota** | The internal billing unit. A request pre-consumes an estimate and post-consumes the true cost; the difference is refunded. | columns on `users`, `tokens`, `channels` |
| **Usage log** (a row in `logs`) | The billing audit record for one billed request, also used for usage analytics and the dashboard. | `logs` table (§1.5.2) |
| **Trace** (a row in `traces`) | The per-request latency record: URL, method, body size, status, six lifecycle timestamps, external tool calls. | `traces` table (§1.5.1) |
| **Application log** | The process's structured log lines (files and/or stdout). Not a database object. | log files (§1.5.3) |
| **Option** | A key/value row of runtime configuration editable from the admin UI, mirrored into memory and re-read periodically. | `options` table |

### 1.2 Deployment shapes

**One code path, three database engines.** The primary database is chosen from `SQL_DSN`: a
DSN beginning with `postgres://` selects PostgreSQL, any other non-empty DSN selects MySQL, an
empty DSN selects SQLite at `SQLITE_PATH` (default `one-api.db`), opened in WAL mode with
`synchronous=NORMAL` (`model/main.go:117-176`). The SQLite driver is the CGO binding to the C
library (`mattn/go-sqlite3`), so the binary is not CGO-free. The ORM is GORM 1.31; all three
engines are exercised by the test suite. Connection pool policy: 200 idle, 2000 open,
5-minute lifetime (`model/main.go:540-566`), with a 30-second monitor that logs when the pool
is more than 80% used.

**Optional second database for usage logs.** `LOG_SQL_DSN`, when set, opens a second handle
(`LOG_DB`) and moves **exactly two tables** to it: `logs` and `data_migrations`
(`model/main.go:477-538`). Everything else, **including `traces`**, stays on the primary
database. This is the existing seam for offloading Plane B (the usage ledger) and it is why
the proposal treats `logs` and `traces` differently: `traces` cannot be moved off the primary
database by configuration today.

**Optional Redis.** `REDIS_CONN_STRING` enables a shared cache used for token/user/channel
lookups, rate limiting, and (after Phase 0) the dashboard aggregate cache. Redis is disabled
when the variable is empty **or** when `SYNC_FREQUENCY=0`. A startup ping failure is fatal. A
subtlety that bit Phase 0: the in-process "Redis enabled" flag defaults to `true` until the
client is initialized, so any cache gated on the flag alone must also check the client handle.

**Multi-node.** Several gateway processes may share one database. `NODE_TYPE=slave` marks a
process as a non-master; anything else is a master (`common/config/config.go:89`). Being a
master gates exactly three things: running schema migration (`AutoMigrate`) on both database
handles, running the data-migration workers, and ignoring `FRONTEND_BASE_URL`. It does **not**
gate the API routers, the dashboard, or any background sweeper: every node serves every route
and every node runs the retention workers (§12). Nodes see each other's configuration changes
by **polling**: every `SYNC_FREQUENCY` seconds (default 120) each node re-reads the `options`
and channel tables; there is no pub/sub invalidation.

**Optional telemetry.** `OTEL_ENABLED=true` (default `false`, requires
`OTEL_EXPORTER_OTLP_ENDPOINT`) installs an OpenTelemetry tracer provider and meter provider
exporting over OTLP/HTTP with gzip, a `traceparent`/`tracestate` propagator, an `otelgin`
server span per request, and the GORM OpenTelemetry plugin (`common/telemetry/telemetry.go`).
`ENABLE_PROMETHEUS_METRICS` (default `true`) exposes `/metrics`, protected by `METRICS_TOKEN`.

**Graceful shutdown** (`main.go:269-317`, `common/graceful`). On SIGINT/SIGTERM the process
stops accepting requests, waits for in-flight handlers, flushes the trace writer, flushes the
quota batch updater, waits for "critical" background goroutines (billing, refunds, error
processing, registered through `graceful.GoCritical`), shuts down the OpenTelemetry
providers, and closes the database. The whole sequence shares one deadline,
`SHUTDOWN_TIMEOUT` (default 360 s).

**Toolchain.** Go 1.27.1, gin 1.12, GORM 1.31.2, OpenTelemetry SDK 1.44, a fork of zap for
logging, `github.com/Laisky/errors/v2` for error wrapping, testify for tests.

### 1.3 Anatomy of a relay request

Middleware registered on the whole HTTP engine, in order (`main.go:188-239`):

| # | Middleware | Role |
| --- | --- | --- |
| 1 | `gin.Recovery()` | Panic recovery for the whole engine. Registered first so that it recovers **after** the tracing middleware's deferred cleanup has run. |
| 2 | `otelgin.Middleware` | OpenTelemetry HTTP server span; only when `OTEL_ENABLED`. |
| 3 | `gmw.NewLoggerMiddleware` | Attaches the request-scoped structured logger that every handler retrieves with `gmw.GetLogger(c)`, mints the per-request trace id, and emits the **access-log line** when the request completes (§1.5.3). |
| 4 | `middleware.RequestId()` | Generates the request id echoed as `X-Oneapi-Request-Id` and stored on every usage-log row. |
| 5 | `middleware.TracingMiddleware()` | Starts the request trace, wraps the response writer to capture the first byte sent to the client, and defers the trace end (§1.5.1). Registered globally, so **every** HTTP request is traced unless excluded, including static assets and health probes. |
| 6–7 | Prometheus request and rate-limit metrics | Only when `ENABLE_PROMETHEUS_METRICS`. |
| 8 | Cookie session store | Browser dashboard sessions. |

Middleware on the relay routes only (`router/relay.go:54-63`): an in-flight request counter
for graceful drain; a relay-shaped panic recovery; **token authentication** (validates the
API key, expiry, IP restriction, model allow-list, quota; binds the user/token identity onto
the request logger); async-task channel pinning; **channel selection** (`Distribute`: resolves
the caller's group and the requested model, then picks a channel by ability, priority, and
weight); and three rate limiters (global, low-balance, per-channel).

The controller then dispatches by endpoint into a relay helper (`relay/controller/text.go`
is the canonical one), which: validates and maps the model name; resolves pricing;
**pre-consumes** an estimated quota and writes a **provisional** usage-log row; hands the
converted request to the provider adaptor's `DoRequest`; hands the response to `DoResponse`,
which converts and streams it back to the client and returns the token usage; and
**post-consumes** the true cost. Post-consume runs in a `graceful.GoCritical` goroutine with
a detached context so that a client disconnect cannot abort billing, and it either
**reconciles** the provisional row into a final consume row or inserts a consume row directly
(`relay/billing/billing.go:71-160`). Quota counters on `users`, `tokens`, and `channels` are
updated per request, or accumulated in memory and flushed every `BATCH_UPDATE_INTERVAL`
seconds when `BATCH_UPDATE_ENABLED=true` (default `false`).

Where each observability write happens on that path, in the **pre-proposal** code:

```
client ──▶ Recovery ─▶ [otelgin] ─▶ gmw logger ─▶ RequestId ─▶ TracingMiddleware
                                                                 │ INSERT traces (request_received)
           relay: auth ─▶ Distribute ─▶ rate limits ─▶ controller.Relay ─▶ RelayTextHelper
                                                                 │ INSERT logs (provisional)
           adaptor.DoRequest ─────────────────────────────────── │ SELECT+UPDATE traces (request_forwarded)
                    upstream HTTP ──────────────────────────────│ SELECT+UPDATE traces (first_upstream_response)
           adaptor.DoResponse ─▶ first byte to client ─────────│ SELECT+UPDATE traces (first_client_response)
                    stream ends ────────────────────────────────│ SELECT+UPDATE traces (upstream_completed)
           post-consume (GoCritical) ───────────────────────────│ UPDATE logs (reconcile) or INSERT logs; quota UPDATEs
client ◀── response complete; TracingMiddleware defer ─────────│ SELECT+UPDATE traces (request_completed); UPDATE traces (status)
           gmw logger ─────────────────────────────────────────│ access-log line to file/stdout
```

Twelve of those statements are trace bookkeeping; after Phase 1 they collapse into one
multi-row `INSERT` per batch issued off the request goroutine (§3.1, §5 Phase 1).

### 1.4 Two different things are both called "logs"

This document, like the code, uses the word for two unrelated artifacts, and the panel should
keep them apart:

- **Usage logs** are rows in the database table `logs`. One row per billed request (plus
  top-up, admin, system, and test events). They are the **billing audit trail** and the
  source of every usage page and dashboard chart. They are Plane B in §2.
- **Application logs** are the process's structured text lines, written to rotating files
  under the `--log-dir` directory and/or to stdout. They are for operators. They are part of
  Plane C in §2.

`LOG_SQL_DSN`, `LOG_DB`, `logs`, `RecordConsumeLog`, and "usage ledger" all refer to the
table. `LOG_RETENTION_DAYS`, `LOG_MAX_TOTAL_SIZE_MB`, `LOG_MIN_FREE_DISK_MB`, `LOG_SAMPLE_*`,
`APP_LOG_SINK`, and "log files" all refer to the process logs.

### 1.5 The three observability signals and where they live today

#### 1.5.1 Request traces (`traces` table)

**What a trace is.** One row per HTTP request handled by the gateway, keyed by a per-request
`trace_id` (a span-scoped id minted by the logger middleware, unique even when several
requests share one distributed OpenTelemetry trace). The row records the sanitized URL
(capped at 4096 bytes), method, request body size, final HTTP status, and a JSON document of
lifecycle timestamps. Full DDL and the JSON shape are in Appendix A.1.

**The six lifecycle timestamps**, all Unix milliseconds, in the order they occur in a request
(`model/trace.go:76-83`):

| Key | Marked where | Meaning |
| --- | --- | --- |
| `request_received` | when the trace is created in `TracingMiddleware` | gateway received the request |
| `request_forwarded` | `relay/adaptor/common.go:176`, immediately before the upstream HTTP call (also the WebSocket transport for the Responses API) | request left the gateway |
| `first_upstream_response` | `relay/adaptor/common.go:218`, immediately after the upstream call returns headers | first byte back from the provider |
| `first_client_response` | first `Write`/`WriteHeader`/`WriteString` on the wrapped response writer (`middleware/tracing.go:42-70`) | first byte sent to the client |
| `upstream_completed` | streaming handlers in `relay/adaptor/openai/main.go:84` | provider stream finished |
| `request_completed` | `RecordTraceEnd`, from the middleware's `defer` | response finished |

For a streaming relay `first_client_response` precedes `upstream_completed`; for a
non-streaming relay the order is reversed. Each key is a single slot, last write wins; on a
relay retry the forwarded/first-upstream marks are overwritten by the later attempt. The
document also carries `external_calls`, a list of tool invocations (today only MCP tool calls
from `relay/controller/mcp_retry.go:188`) with source, tool, server, start/end, duration, and
an error flag. `status` is the plain HTTP status code; `0` is coerced to `200` at request end.

**Who reads traces.** Two authenticated API endpoints, `GET /api/trace/:trace_id` and
`GET /api/trace/log/:log_id` (`controller/tracing.go`); the second resolves a usage-log row
first, then its trace, and returns server-computed durations (`processing_time`,
`upstream_response_time`, `response_processing_time`, `streaming_time`, `total_time`). The
web UI renders these as the per-request timeline in the log details modal
(`web/modern/src/components/LogDetailsModal.tsx`, plus equivalents in the two legacy themes),
and the CSV export of the logs page fetches one trace per exported row. Users may only read
traces for their own requests; admins may read all.

**Pre-proposal write pattern.** Every lifecycle mark was written immediately, on the request
goroutine: one `INSERT` at trace creation, then for each of five marks a `SELECT` of the whole
row, a JSON unmarshal, a one-field change, a JSON marshal, and an `UPDATE` of the whole text
column, then one final `UPDATE` of the status. That is twelve statements per traced request,
all against a table with a unique index on `trace_id` (§3.1). This path still exists behind
`TRACE_WRITE_MODE=sync` and is the `standalone` default (§6).

**Retention.** `TRACE_RETENTION_DAYS` (default 30, `0` disables) drives a background sweeper.
Before Phase 0 it ran once a day and issued a single unbounded
`DELETE FROM traces WHERE created_at < ?`. It is not master-gated: every node runs it.

**Relation to OpenTelemetry.** When `OTEL_ENABLED`, the same timestamps are also added as
events and attributes on the `otelgin` server span. The `traces` table is therefore a
duplicate of data the OpenTelemetry pipeline can already carry, which is what Phase 3
exploits.

#### 1.5.2 Usage logs (`logs` table)

**What a row is.** The billing audit record of one billed request: user id/uuid/name, token
name/uuid, channel id/uuid, requested and upstream model names, prompt/completion/cached
prompt token counts, quota charged, elapsed time (ms), stream flag, `request_id`, `trace_id`,
a rendered human-readable `content` string, and a JSON `metadata` column. `created_at` is
Unix **seconds**. Full column list in Appendix A.2.

**Row types** (`model/log.go:281-303`): `1` top-up, `2` consume (the billing row), `3`
management, `4` system, `5` channel test, `6` **provisional** (pre-consumed, awaiting
reconciliation), `7` tool invocation (one row per MCP tool call, with the tool id in
`model_name`). Every user-facing query excludes type 6. A provisional row is flipped to type 2
by the post-consume reconciliation, so **a row's type can change after it was inserted**, and
the change can land after the hour or day bucket the row belongs to has closed. This is the
"late write" case Phase 2's rollup design must absorb (§5 W2.2, §11.4).

**Write path.** `model.RecordConsumeLog` / `ReconcileConsumeLogDetailed` from the billing
package (§1.3), one `INSERT` (or one `UPDATE` of the provisional row) per billed request. The
same function also emits one application-log line per row; its size is the subject of Phase 0
W0.4.

**Indexes.** The struct declares 17 secondary indexes: 14 single-column (`user_id`,
`user_uuid`, `token_name`, `token_uuid`, `model_name`, `origin_model_name`, `quota`,
`prompt_tokens`, `completion_tokens`, `channel_id`, `channel_uuid`, `trace_id`,
`elapsed_time`, `cached_prompt_tokens`) and 3 composite (`idx_user_token` on
`(user_id, token_name)`, `idx_created_at_type` on `(created_at, type)`,
`index_username_model_name` on `(model_name, username)` — note the column order despite the
name). A separate migration adds a unique index on `uuid`, so a migrated database carries 18.
Every `INSERT` maintains all of them (§3.3).

**Readers** (all under `/api`, `controller/log.go` and `controller/user.go`): the admin log
list and count, the per-user log list and count, the per-token log list, admin and user
search, admin and user quota-sum statistics, the six dashboard aggregates (§1.6), the
trace-by-log lookup, the client-side CSV export, and the admin purge
(`DELETE /api/log/`). The billing-usage endpoints under `/dashboard/billing/*` read quota
counters on `users` and `tokens`, not this table.

**Retention.** **None is automatic.** The only deletion is the admin-invoked purge endpoint.
Traces and async-task bindings have background sweepers; usage logs do not. This asymmetry
is why Plane B is described as unbounded in §2 and why W2.6 exists.

#### 1.5.3 Application logs

**Stack.** A fork of zap (`github.com/Laisky/zap`) wrapped by `glog` from
`github.com/Laisky/go-utils`. Encoding is **console** (human-readable key=value), always;
there is no JSON output path in the application. One process-wide logger; request handlers
obtain a derived, request-scoped logger via `gmw.GetLogger(c)` that already carries
`request_id`, `trace_id`, and the bound user/token/channel identity. All derived loggers
share one atomic level, so changing the level once changes it for the whole process.

**Where lines go.** `APP_LOG_SINK` = `both` (default), `stdout`, or `file`. The file sink is a
custom rotating sink (`common/logger/rotation.go`) writing `oneapi-YYYYMMDD.log` (daily,
UTC) or `oneapi-YYYYMMDDHH.log` (hourly) under the directory given by the **command-line
flag** `--log-dir` (default `./logs`; there is no `LOG_DIR` environment variable). Rotation
is time-based only; there is no size trigger. `ONLY_ONE_LOG_FILE=true` disables rotation.

**What one relay request emits at INFO**, at minimum: the gin access line (message
`"<status> <method>"`, fields `url`, `remote`, `host`, `trace_id`, `cost`, `request_size`,
`response_size`, plus the bound identity fields), the `record log` line emitted when the
usage-log row is written (fields include `type`, `content`, quota and token counts,
`log_request_id`, `log_trace_id`), and several relay lifecycle lines from the adaptor and
billing code. The measured size of the `record log` line alone is 516–751 bytes depending on
`content` length (Appendix D.4).

**Retention before Phase 0.** `LOG_RETENTION_DAYS` defaulted to `0` (never delete); the
retention cleaner returned immediately in that case; there was no size ceiling and no
free-disk guard; the sweep interval was 24 hours. Two retention mechanisms exist and both key
on `LOG_RETENTION_DAYS`: the rotation sink purges by the **date stamp in the file name** when
it rotates, and the retention cleaner purges by **file modification time** on its own
schedule (§12).

**Level and alerting.** `LOG_LEVEL` or `DEBUG=true` sets the level at startup. `LOG_PUSH_API`
installs a webhook that pushes every `error`-and-above event, rate-limited to about one per
second.

#### 1.5.4 Metrics and OpenTelemetry

`common/metrics.MetricsRecorder` is a 38-method interface with two implementations,
Prometheus (`monitor/prometheus`) and OpenTelemetry (`monitor/otel`), and a no-op fallback.
The interface carries a documented label-cardinality discipline: every label value must come
from a compile-time constant, never from request content, and every label value is sanitized
to valid UTF-8 because both backends reject or panic on invalid strings. A shipped Grafana
dashboard (`docs/grafana-dashboard.json`) reads the Prometheus metrics. Phase 1 added a
small **optional** extension interface for trace-pipeline accounting rather than widening
`MetricsRecorder`, so that out-of-tree recorders keep compiling (Appendix B.3).

### 1.6 The dashboard and the log pages

**Endpoint.** `GET /api/user/dashboard` (`controller/user.go:397-550`), authenticated. Query
parameters: `from_date` and `to_date` (`YYYY-MM-DD`, UTC, both inclusive; converted to a
half-open second range), and `user_id` (a user reference, or the literal `all`). A normal user
may only see their own data and is capped at 7 days. A root user may pass `user_id=all` or
omit it; both mean **site-wide** (internally `targetUserId = 0`), and root is capped at 365
days. When no dates are given the window is the last 7 days including today.

**Queries per page load**, in order (`controller/user_dashboard_cache.go:140-164`,
`model/log.go`): per-day-per-model, per-day-per-user, and per-day-per-token aggregates over
consume rows (each returning request count, quota, prompt/completion/cached tokens, cache-hit
count and cache-hit quota), then the same three breakdowns over tool rows (request count and
quota only), then quota and status: for site-wide, a `SUM`/`COUNT` over the whole `users`
table; for a single user, that user's row. All six aggregates group by a **computed day
expression** — `DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d')` on MySQL,
`TO_CHAR(date_trunc('day', to_timestamp(created_at)), 'YYYY-MM-DD')` on PostgreSQL,
`strftime('%Y-%m-%d', datetime(created_at,'unixepoch'))` on SQLite — so no index can serve
the grouping; only the range predicate on `created_at` is indexable.

**Response.** A JSON envelope with six arrays (`logs`, `user_logs`, `token_logs`,
`tool_logs`, `tool_user_logs`, `tool_token_logs`) plus `total_quota`, `used_quota`, and
`status`. The UI (`web/modern/src/pages/dashboard/`) renders overview cards (totals and
per-day averages), a top-models leaderboard, per-day line charts, stacked bar charts by model,
user, and token with a tokens/requests/expenses toggle, the same for tool usage, and
cache-hit heatmaps. Refresh is manual.

**Log list pages.** `GET /api/log/` (admin) and `GET /api/log/self` (user) paginate with
`LIMIT/OFFSET`, default sort `id DESC`, and after every page fetch issue a second, **unbounded
`COUNT(*)`** with the same filters to populate `total`. A 30-day range cap exists but applies
**only** when the caller requests an explicit sort **and** supplies both timestamps; the
default unsorted query and the count are uncapped and have no statement timeout.

### 1.7 How configuration works

Everything in this proposal is configured through environment variables read once at process
start. Three properties of the reader (`common/env/helper.go`) shape the design:

1. **Empty means unset.** A variable set to the empty string is indistinguishable from an
   absent one and takes the default. To disable a list-valued variable that has a non-empty
   profile default, the value `-` is used (`TRACE_EXCLUDED_PATH_PREFIXES=-`).
2. **Unparseable numbers silently take the default.** `TRACE_SAMPLE_RATE=abc` is `1.0` (or
   the profile's value), not an error. Booleans are `true` only for the exact string `true`.
3. **Profiles are defaults, not modes.** `OBSERVABILITY_PROFILE` selects, for each variable,
   which of three literal defaults is used (`common/config/observability.go`,
   `profileInt`/`profileFloat`/`profileString`). An explicitly set variable always wins. An
   unknown profile name falls back to `standalone`, so a typo can never enable sampling.

Values are **normalized or clamped before validation** (unknown sink names are dropped,
rates are clamped into `[0,1]`, non-positive sizes take the default). A set of fail-fast
validators runs at package initialization and panics on violation, but for every
observability variable the validator runs on the already-clamped value and therefore cannot
fire from the environment; they are defense-in-depth against future code changes (§12). The
one genuine fail-fast in this area is pre-existing: `OTEL_ENABLED=true` without
`OTEL_EXPORTER_OTLP_ENDPOINT` aborts startup.

### 1.8 Engineering conventions the design must respect

From the project's contributor rules (`AGENTS.md`), the ones this proposal cites or depends
on:

- No hand-written code file may exceed **800 lines**; split by responsibility. Documents are
  exempt.
- Errors are wrapped with `github.com/Laisky/errors/v2` (`Wrap`, `Wrapf`, `WithStack`),
  never returned bare, and handled **exactly once** (returned or logged, never both).
- Every function and interface has a comment starting with its name that describes purpose,
  parameters, and return values.
- Tests use `github.com/stretchr/testify/require`; the gate is `go vet ./...`,
  `go test -race ./...`, and a frontend build.
- Reads prefer explicit SQL; writes use GORM. No `Preload`, no `clause` package.
- Request paths call `gmw.GetLogger(c)` once per function and never use the global logger.
- All time handling is UTC; date ranges include the whole final day and end just before
  00:00 of the next day.
- Structured logging only; never log credentials or payloads outside debug mode.
- Target Go 1.27.

The **backward-compatibility rule for storage changes** used throughout this proposal —
additive, nullable columns; the old document still written; a rolled-back binary must read
rows the new binary wrote — is stated by this proposal in §6 and was applied by the earlier
compact-UUID work, but it is not (yet) written into `AGENTS.md`.

---

# Part II — The Proposal

## 2. Purpose and Non-Negotiable Contract

one-api today stores every observability signal — request traces, per-request usage logs,
and dashboard aggregates — in the same relational database that holds the authoritative
account state (users, tokens, channels, quota). That design is the reason the product
installs from a single binary with zero external dependencies, and it must stay the default.

This proposal keeps that default intact while making the system survivable at 1M users /
10k RPS. Three guarantees bound every change below:

1. **Zero-dependency default.** A fresh `docker run` with no environment variables must
   still start with SQLite only, no Redis, no collector, no OLAP store, and must behave
   exactly as it does today. After the compatibility audit of 2026-09-06 there are **no**
   default behavior changes under the `standalone` profile (§6, §11.3). Nothing in this
   proposal makes an external system *required*.
2. **Billing integrity is never traded for throughput.** The `logs` table is the billing
   audit trail. Any change to its write path must be opt-in, must flush on graceful
   shutdown, and must never lose more than the operator explicitly accepted via
   configuration.
3. **Additive configuration.** Every knob introduced here defaults to current behavior or to
   a strictly safer behavior. Operators opt into scale, they do not opt out of breakage.

The core idea is a **separation by data class**, not by feature:

| Plane | Data | Consistency need | Volume at 10k RPS | Home |
| --- | --- | --- | --- | --- |
| **A — Account/OLTP** | users, tokens, channels, quota, redemptions, orders | strong, transactional | low (bounded by user count) | primary SQL DB, always |
| **B — Usage ledger** | `logs` rows (billing audit + usage analytics) | durable, eventually queryable | ~864M rows/day | SQL DB by default; rollups for reads; optional OLAP mirror |
| **C — Peripheral telemetry** | `traces` rows, application log files, metrics | best-effort, sampleable, droppable | ~864M rows/day + ~1.3 TB/day of text | SQL DB by default *after* reduction; OTLP/OLAP when configured |

Plane C is the immediate emergency. Plane B is the dashboard problem. Plane A is already
fine.

---

## 3. Measured Problem Statement

All numbers below are derived from the pre-proposal code (commit `397781e1`), with the
arithmetic shown so reviewers can challenge the assumptions rather than the conclusions.
Where Phases 0 and 1 have since measured a figure, the measured value is noted.

### 3.1 The trace pipeline issued ~12 synchronous SQL statements per request

`middleware.TracingMiddleware` is registered globally (`main.go:206`), before any route
grouping, so it runs for every HTTP request including static SPA assets, `/api/status`, and
health probes.

Per request, on the request goroutine, synchronously (§1.5.1 explains each mark):

| Call site | Operation | Statements |
| --- | --- | --- |
| `TracingMiddleware` → `model.CreateTrace` (`model/trace.go:91`) | INSERT | 1 |
| `relay/adaptor/common.go:176` → `UpdateTraceTimestamp` (`model/trace.go:156`), key `request_forwarded` | SELECT + UPDATE | 2 |
| `relay/adaptor/common.go:218` → `UpdateTraceTimestamp`, key `first_upstream_response` | SELECT + UPDATE | 2 |
| `middleware/tracing.go` first write → `UpdateTraceTimestamp`, key `first_client_response` | SELECT + UPDATE | 2 |
| `relay/adaptor/openai/main.go:84` → `UpdateTraceTimestamp`, key `upstream_completed` | SELECT + UPDATE | 2 |
| `RecordTraceEnd` → `UpdateTraceTimestamp`, key `request_completed` | SELECT + UPDATE | 2 |
| `RecordTraceEnd` → `UpdateTraceStatus` (`model/trace.go:315`) | UPDATE | 1 |
| **Total** | | **12** |

Each `UpdateTraceTimestamp` reads the whole row, JSON-unmarshals `timestamps`, mutates one
field, re-marshals, and writes the entire text column back. This is a read-modify-write
cycle per timestamp, on the hot path, against a table with a unique index on `trace_id`.

Adding the consume-log INSERT (`model/log.go:773`), the floor is **~13 statements per relay
request**:

```
10,000 req/s × 13 stmt/req = 130,000 statements/second
                             of which ~120,000 (92%) are trace bookkeeping
```

No single-writer PostgreSQL or MySQL instance serves that alongside the account workload.
On SQLite it is not survivable at all.

*Measured (Appendix D.1):* the twelve statements are exact and reproducible (12.00 ± 0%);
the batched path issues 0 statements on the request goroutine and 0.002–0.05 per trace in
total depending on arrival rate.

### 3.2 Trace and usage storage growth is unbounded relative to any single node

Assumptions: `traces` row ≈ 600 B including the `trace_id` unique index and the `created_at`
index (URL is capped at 4096 B but averages far lower); `logs` row ≈ 1.2–2 KB including its
secondary indexes. The upper bound matches the industry figure of 1–2 KB per logged gateway
request published in LiteLLM's database sizing guide (§14).

```
864,000,000 rows/day  (10k RPS × 86,400 s)

traces : 864M × 600 B  ≈ 520 GB/day  →  ~15.6 TB at the default 30-day retention
logs   : 864M × 1.5 KB ≈ 1.3 TB/day  →  unbounded (no automatic retention today)
```

`TRACE_RETENTION_DAYS` defaults to 30 (`common/config/config.go:1144`) and the pre-proposal
sweeper (`model/trace_retention.go`) issued a single unbounded
`DELETE FROM traces WHERE created_at < ?` once a day. At this scale that statement deletes
~26 billion rows in one transaction: on PostgreSQL it produces a multi-terabyte WAL burst
and table bloat; on MySQL it holds a gap-locking transaction long enough to stall the
gateway; on SQLite it holds the single writer lock for the duration. The same pattern
existed in the admin-triggered `logs` purge (`model.DeleteOldLog`).

There is **no automatic retention for the `logs` table at all** (§1.5.2).

### 3.3 The `logs` table carries 17 secondary indexes on the hottest insert path

From `model/log.go:26-59` (full list in §1.5.2 and Appendix A.2): indexes exist on
`user_id`, `user_uuid`, `token_name`, `token_uuid`, `model_name`, `origin_model_name`,
`channel_id`, `channel_uuid`, `trace_id`, plus the composites `idx_user_token`,
`idx_created_at_type`, `index_username_model_name`, plus five **sorting-only** indexes on
`quota`, `prompt_tokens`, `completion_tokens`, `elapsed_time`, and `cached_prompt_tokens`.
A migration adds an 18th, the unique index on `uuid`.

Every INSERT maintains all of them. The five sorting-only indexes are low-selectivity integer
columns whose only consumer is the sort-column whitelist behind the log list pages
(`model.GetLogOrderClause`, `model/log.go:544`) — a UI convenience that is already
restricted to a 30-day window when a sort is requested (`controller/log.go:45-52`). At 10k
inserts/second this is pure write amplification.

### 3.4 The dashboard runs six unbounded GROUP BY scans per page load

`controller.GetUserDashboard` (`controller/user.go:397`) issues, sequentially (§1.6):

1. `SearchLogsByDayAndModel` (`model/log.go:1571`)
2. `SearchLogsByDayAndUser` (`model/log.go:1625`)
3. `SearchLogsByDayAndToken` (`model/log.go:1681`)
4. `SearchToolLogsByDayAndTool` (`model/log.go:1428`)
5. `SearchToolLogsByDayAndUser` (`model/log.go:1472`)
6. `SearchToolLogsByDayAndToken` (`model/log.go:1520`)

plus `GetSiteWideQuotaStats` (`model/user.go:675`), a `SUM`/`COUNT` over the whole `users`
table — a full scan of 1M rows.

For a root user the default is **site-wide** (`targetUserId = 0`), and the default window is
7 days:

```
7 days × 864M rows/day = 6.05 billion rows
× 6 aggregate queries, none of which can use a covering index because they
  GROUP BY a computed day expression (dayAggregationSelect, model/log.go:1412)
```

Wrapping the indexed column in a function defeats partition pruning and index-only scans on
all three engines.

A root user may request 365 days (`maxDays = 365`, `controller/user.go:411-414`), i.e. ~315
billion rows per aggregate. The dashboard timeout is not a symptom of a slow query — it is
the expected outcome.

*Measured (Appendix D.5):* the site-wide `users` aggregate alone costs 114 ms at 1M users on
PostgreSQL, on every load.

### 3.5 The log list page runs an unbounded COUNT on every page

`controller.GetAllLogs` calls `model.GetAllLogsCount` (`model/log.go:1153`) after every page
fetch, including page 1 with no filters. `COUNT(*)` over a 100-billion-row table is a full
index scan on every engine that supports it, and the count carries no statement timeout
(§1.6).

### 3.6 Application logs have no retention by default and no size ceiling

`LogRetentionDays` defaults to **0 = disabled** (`common/config/config.go:1130`). The
retention cleaner returned immediately in that case, so out of the box nothing ever deletes
`<log-dir>/oneapi-*.log`. There was no size-based ceiling and no free-disk guard; the sweep
interval was 24 h even when enabled.

Per relay request the process emits, at INFO, at minimum: the gin access line, the
`record log` line (which includes the rendered `content` string), and several relay
lifecycle lines (§1.5.3).

```
~4 lines/req × ~400 B = ~1.6 KB/req
10,000 req/s × 1.6 KB = 16 MB/s ≈ 1.35 TB/day
```

A 500 GB volume fills in under 9 hours. This is the reported disk exhaustion.

*Measured (Appendix D.4):* the `record log` line alone is 516–751 bytes in console
encoding, so the ~400 B/line figure above is conservative.

### 3.7 What is already right, and should be reused

The codebase is not missing infrastructure — it is missing the routing decision:

- OpenTelemetry traces and metrics are already wired (`common/telemetry/telemetry.go`,
  `OTEL_ENABLED`, OTLP/HTTP exporters, GORM tracing plugin, `otelgin` middleware).
  `model.CreateTrace` and `UpdateTraceTimestamp` **already** mirror every timestamp onto the
  active span as span events and attributes. The `traces` table is therefore a duplicate of
  data OpenTelemetry can already carry.
- A metrics abstraction with Prometheus and OpenTelemetry recorders exists
  (`common/metrics/interface.go`, `monitor/prometheus`, `monitor/otel`), with documented
  label-cardinality discipline and a shipped `docs/grafana-dashboard.json`.
- Usage-log traffic can already be pointed at a second database via `LOG_SQL_DSN`
  (`common/config/config.go:267`, `model/main.go:477`) — the `LOG_DB` handle. This is the
  seam the OLAP work plugs into. Note that only `logs` moves; `traces` stays on the primary
  database (§1.2).
- A batched write-behind pattern with graceful-shutdown flush already exists and is
  documented for quota (`model/utils.go:75`, `InitBatchUpdater`, off by default), including
  its consistency trade-offs. New batched writers should copy its shape, not invent one.
- `common/graceful` provides `GoCritical`/`Drain` for shutdown-safe background workers.

---

## 4. Design: Three Deployment Profiles

One preset variable selects a coherent set of defaults. Every individual knob remains
independently overridable; the profile only changes *defaults* (§1.7).

```
OBSERVABILITY_PROFILE = standalone | scaled | external      (default: standalone)
```

### 4.1 `standalone` — the default, unchanged experience

Single SQL database. Traces and usage logs in-database, written exactly as before. Dashboard
reads live aggregates (rollups, once Phase 2 ships, will be cheap even for small installs).
Suitable to roughly the low hundreds of RPS.

### 4.2 `scaled` — one database, made to survive

Still a single SQL database and no external dependency. Adds: in-memory trace accumulation
with a single batched write, trace sampling with always-keep rules for errors and slow
requests, the compact per-request log line and log sampling, path exclusions, dashboard
caching and a site-wide range cap, log-file retention, size ceiling, and free-disk guard
(all Phases 0–1, shipped); and, once Phase 2 ships, batched usage-log writes, the reduced
index set, rollup-only dashboards, and keyset pagination. Target: single-digit thousands of
RPS on a well-sized PostgreSQL/MySQL.

### 4.3 `external` — telemetry leaves the database

Peripheral telemetry (Plane C) is emitted as OTLP and never written to SQL. Usage events
(Plane B) are mirrored to an OLAP store; the SQL `logs` table keeps a short operational
window as the source of truth for billing disputes. The built-in dashboard reads from the
OLAP store, or is put in rollup-only mode and drilldown is delegated to Grafana. Target:
10k+ RPS. Today the profile delivers the OTLP trace sink (Phase 1) and the Phase 0
protections; the OLAP mirror is Phase 4.

### 4.4 Data flow

```
                        ┌───────────────────────────────────────────┐
  relay request ───────▶│ in-process TraceRecorder (no I/O)         │   ┐
                        │  marks timestamps in memory               │   │
                        └───────────────┬───────────────────────────┘   │
                                        │  once, at request end          │ IMPLEMENTED
                                        ▼                                │ (Phase 1)
                              ┌──────────────────┐                       │
                              │  TraceSink       │   sampled + path-filtered
                              └───┬─────┬────┬───┘                       │
                     ┌────────────┘     │    └──────────────┐            │
                     ▼                  ▼                   ▼            │
              sqlSink (batched)    otlpSink (span)      nullSink         │
                     │                  │                                │
                     ▼                  ▼                                ┘
              traces table       OTel Collector ──▶ ClickHouse / Tempo / vendor

  billing ─────▶ UsageStore.Record([]UsageEvent)                         ┐
                     │                                                   │
          ┌──────────┴───────────┐                                       │ PLANNED
          ▼                      ▼                                       │ (Phases 2, 4)
   sqlUsageStore           clickhouseUsageStore   (mirror; SQL stays source of truth)
   (logs table)                   │                                      │
          │                       │                                      │
          ▼                       ▼                                      │
   rollup aggregator ──▶ log_usage_rollup  ◀──── dashboard reads         ┘
```

---

## 5. Work Items

Ordered by return on effort. Phase 0 and Phase 1 together remove ~92% of the write load and
are independently shippable. Each shipped phase carries a **delivery record** stating what
was built, where, how it deviates from the plan, and why.

### Phase 0 — Containment (no schema change, no new dependency)

**W0.1 — Exclude non-relay paths from tracing.**
`TracingMiddleware` gets a prefix skip-list, `TRACE_EXCLUDED_PATH_PREFIXES`. Under the
`scaled` and `external` profiles it defaults to
`/api/status,/metrics,/health,/static,/assets,/favicon`; under `standalone` it is empty so
that existing trace coverage is preserved. Matching is a case-sensitive string prefix on
the URL path (query string ignored); `-` disables the list explicitly. Static SPA asset
requests otherwise each create a trace row.

**W0.2 — Chunked retention sweeps.**
Rewrite the trace sweeper and the `logs` purge as bounded loops: delete at most
`RETENTION_DELETE_BATCH_SIZE` rows per statement (default 5000), pause
`RETENTION_DELETE_PAUSE_MS` between chunks (default 10, chosen by measurement — Appendix
D.3), stop when a chunk removes fewer rows than the batch size, and honor context
cancellation between chunks. The bounded statement differs per engine, because the three
disagree about how to limit a `DELETE`:

| Engine | Form |
| --- | --- |
| MySQL | `DELETE FROM t WHERE <pred> LIMIT n` (native; a subquery naming the target table is forbidden) |
| PostgreSQL | `DELETE FROM t WHERE ctid IN (SELECT ctid FROM t WHERE <pred> LIMIT n)` (no native `DELETE … LIMIT`) |
| SQLite | `DELETE FROM t WHERE rowid IN (SELECT rowid FROM t WHERE <pred> LIMIT n)` (the embedded driver is not built with `SQLITE_ENABLE_UPDATE_DELETE_LIMIT`) |

The dialect is read from the database handle, not from the process-global engine flags,
because a deployment that splits `SQL_DSN` and `LOG_SQL_DSN` across engines would otherwise
emit one engine's syntax against the other. The sweep interval drops from 24 h to
`RETENTION_SWEEP_INTERVAL_MINUTES` (default 60) so each sweep is smaller. Appendix B.4 shows
the implemented function.

**W0.3 — Application-log survival.**
- `LOG_RETENTION_DAYS`: age-based deletion of rotated log files. *Plan as written on
  2026-09-05: change the default from `0` to `7`. Outcome: rejected in the compatibility
  audit (§6), because a default that deletes files the operator kept is a behavior change.
  The default stays `0` under `standalone`; `scaled` uses `3`, `external` uses `1`.* When all
  three file guards are off, the cleaner logs one WARN at startup naming the variables and
  `OBSERVABILITY_PROFILE=scaled`.
- `LOG_MAX_TOTAL_SIZE_MB` (default 0 = unlimited; 20480 scaled, 10240 external): when the
  log directory exceeds the ceiling, delete oldest-first until under it, never deleting the
  newest (open) file. This bounds disk even when a single day exceeds the volume.
- `LOG_MIN_FREE_DISK_MB` (default 0 = off; 1024 scaled and external): when free space on the
  log volume falls below the threshold, log one WARN, purge oldest files, and if still
  below, raise the effective log level to `warn` until recovery. A gateway that dies from a
  full disk is worse than a gateway that stops writing INFO lines. Requires `statfs`; inert
  on non-Unix platforms.
- Only files named `oneapi*.log` are ever candidates, so pointing `--log-dir` at a shared
  directory cannot delete another program's files.
- Run the sweep hourly (`RETENTION_SWEEP_INTERVAL_MINUTES`) instead of daily.

**W0.4 — Per-request log volume.**
- `LOG_RECORD_LINE_FORMAT` = `full` (standalone) or `compact` (scaled/external). The
  `record log` line emitted when a usage-log row is written keeps `type`, `log_request_id`,
  `log_trace_id`, `quota`, `prompt_tokens`, `completion_tokens` in both forms; the compact
  form drops `content` and `created_at`. `content` is already persisted on the row;
  duplicating it into the file multiplies bytes for nothing. The full form is also always
  emitted at DEBUG when the level allows it.
- Application-log sampling via zap's per-`(level, message)` sampler: keep the first
  `LOG_SAMPLE_INITIAL` entries of each distinct message per `LOG_SAMPLE_TICK_MS` window
  (default 1000), then one in every `LOG_SAMPLE_THEREAFTER` (default 100).
  `LOG_SAMPLE_INITIAL=0` (the standalone default) disables sampling; `scaled`/`external`
  use 100. **Levels at `warn` and above are never sampled.** *Plan as written: a uniform
  probability `LOG_REQUEST_SAMPLE_RATE`. Outcome: replaced — see the delivery record.*
- `APP_LOG_SINK=file|stdout|both` (default `both`, current behavior); `stdout` alone is the
  correct choice under Kubernetes, where the platform already handles rotation and shipping,
  and it disables the file retention worker entirely.

**W0.5 — Dashboard guard rails.**
- Cap the root site-wide range at `DASHBOARD_MAX_SITEWIDE_RANGE_DAYS` (365 standalone, i.e.
  the existing limit; 31 scaled/external). Per-user ranges are never capped. Exceeding it
  returns an explanatory error instead of a query that would time out.
- Cache the six dashboard aggregates in Redis when Redis is enabled, keyed by
  `oneapi:dashboard:agg:<targetUserId>:<start>:<endExclusive>`, TTL `DASHBOARD_CACHE_TTL_SEC`
  (0 = off under standalone; 60 under scaled/external). Quota and status stay live. Any
  cache failure is a silent miss, never a request failure.
- Serve the site-wide `users` aggregate from an in-process snapshot with the same TTL,
  recomputed at most once per TTL under a mutex, so it also works with no Redis.

#### Phase 0 delivery record (2026-09-05)

Implemented, with tests, in:

| Work item | Where |
| --- | --- |
| W0.1 path exclusion | `tracing.PathExcluded` / `requestExcluded` in `common/tracing/tracing.go:161-185`, honored by every lifecycle helper, not only trace start |
| W0.2 chunked sweeps | `model.ChunkedDelete` in `model/retention_chunk.go`; rewired `CleanExpiredTracesContext`, `DeleteOldLogContext`, `CleanExpiredAsyncTaskBindingsContext`; sweep interval 24 h → 1 h |
| W0.3 log disk survival | `common/logger/disk_guard.go`, `disk_free_unix.go` / `disk_free_other.go`, rewritten `StartLogRetentionCleaner` in `common/logger/log_retention.go`; `APP_LOG_SINK` in `SetupLogger` |
| W0.4 log volume | `common/logger/sampling.go` (`levelBoundedSampler`), installed in `SetupEnhancedLogger` so the gin access line is covered too; `record log` compact form in `model/log.go:666-693` |
| W0.5 dashboard guard rails | `controller/user_dashboard_cache.go`, `model/user_stats_cache.go`, site-wide range cap in `controller.GetUserDashboard` (`controller/user.go:463-477`) |
| Configuration | `common/config/observability.go`, validators in `common/config/validation.go` |

Deviations from the plan above, and why:

1. **One retention knob set instead of trace-specific ones.** The plan named
   `TRACE_DELETE_BATCH_SIZE` and `TRACE_DELETE_PAUSE_MS`. The same chunked sweeper backs
   traces, usage logs, and async-task bindings, so the knobs are
   `RETENTION_DELETE_BATCH_SIZE`, `RETENTION_DELETE_PAUSE_MS`, and
   `RETENTION_SWEEP_INTERVAL_MINUTES`. One mechanism, one set of knobs.
2. **Log sampling uses zap's semantics, not a probability.** The plan named
   `LOG_REQUEST_SAMPLE_RATE` (a probability). What actually solves the problem is zap's
   per-`(level, message)` sampler: it caps the handful of message strings that dominate
   volume while leaving rare messages untouched, which a uniform probability cannot do.
   Sampling is bounded to levels below WARN, because a repeated warning or error reports the
   scale of an incident and thinning it would misreport how bad things are. Because the
   sampler is installed on the process logger's core, it covers the gin access line too, not
   just lines one-api emits directly.
3. **The site-wide range cap defaults to today's limit.** Capping it by default would break
   a working feature for small deployments, where a 365-day site-wide dashboard is perfectly
   fine. `DASHBOARD_MAX_SITEWIDE_RANGE_DAYS` therefore defaults to 365 under `standalone`
   and to 31 under `scaled` and `external`.
4. **Only the aggregates are cached, not the whole dashboard payload.** Quota and status
   stay live for per-user dashboards. They are the numbers a user checks most often, and
   serving them from a minute-old snapshot would be a surprising regression for a saving the
   six aggregate queries already deliver. The site-wide quota aggregate is cached separately,
   in-process, because it must also work in the zero-dependency deployment where there is no
   Redis; for the site-wide dashboard, therefore, quota and status are up to one TTL stale.
5. **`LOG_RETENTION_DAYS` keeps its `0` default under `standalone`** (rejected in the
   compatibility audit; see W0.3 and §6). The earlier edition of this document described
   the `0 → 7` change as shipped; it was not.

Two robustness defects were found and fixed while implementing this phase:

- `common.IsRedisEnabled()` defaults to **true** before `InitRedisClient` runs, so gating a
  cache on that flag alone makes every request attempt a Redis call against a nil client.
  `dashboardCacheEnabled` checks `common.RDB != nil` too.
- The first version of the disk guard read the package-level `logger.Logger` from the
  sweeper goroutine, which races `SetupEnhancedLogger`'s write to it. The worker's captured
  logger is now threaded through every guard function, and `StartLogRetentionCleaner`
  moved after `SetupEnhancedLogger` in `main`. Level escalation still reaches the whole
  process because glog's derived loggers share one atomic level.

Verification is recorded in §10.2.

### Phase 1 — Trace pipeline rewrite (removes ~92% of write statements)

**W1.1 — In-memory `Recorder`.**
The recorder is created once per traced request and stored in the gin context under
`ctxkey.TraceRecorder`. All `RecordTraceTimestamp` / `RecordTraceExternalCall` /
`RecordTraceStatus` calls mutate it in memory under one small mutex — zero I/O. The mutex is
necessary because a streaming relay marks `first_client_response` from the writer goroutine
while the adaptor marks `upstream_completed` from another. Existing call sites keep their
function signatures; only the bodies in `common/tracing/tracing.go` change. The
OpenTelemetry span-event mirroring stays exactly where it is, so `OTEL_ENABLED` deployments
lose nothing. `Finish` is single-shot and returns a deep-copied snapshot, so a late mark from
a straggling goroutine can never share memory with the sink.

**W1.2 — `TraceSink` abstraction.**
Appendix B.1 quotes the interface verbatim. Implementations select on `TRACE_SINK`
(comma-separated):

| `TRACE_SINK` | Behavior |
| --- | --- |
| `db` (default) | batched multi-row INSERT into `traces` |
| `otlp` | emit one span per request with the timestamps as span events; no SQL at all |
| `none` | drop |
| `db,otlp` | fan-out via `multiSink`; each child is attempted, the first error is returned |

Unknown names are dropped by the parser and an empty result falls back to `db`, so a typo
degrades to today's behavior rather than discarding all traces.

**W1.3 — Batched async writer for `sqlSink`.**
Bounded channel (`TRACE_QUEUE_SIZE`, default 20000; 50000 under `scaled`) drained by
`TRACE_WRITER_COUNT` goroutines (default 2; 4 under `scaled`) that flush on
`TRACE_BATCH_SIZE` (default 500) or `TRACE_FLUSH_INTERVAL_MS` (default 1000), whichever
comes first, using one multi-row INSERT per batch. On a full queue, **drop the newest record**
(the one being submitted; nothing already queued is evicted) and increment
`oneapi_trace_records_total{outcome="dropped_queue_full"}`; drops are logged at WARN at most
once per 30 s with a hint naming the tuning variables. `Submit` never blocks and never
returns a capacity error — traces are best-effort by definition. Queue occupancy is
published as `oneapi_trace_queue_depth` against `oneapi_trace_queue_capacity`. A batch-wide
insert failure is not retried row by row (that would turn 500 doomed rows into 501 doomed
statements); only a duplicate-key failure triggers a row-by-row retry that skips duplicates.

Shutdown: `main` calls `tracing.Shutdown` after the HTTP server has stopped accepting
requests and before the database handle closes, and each writer drains the queue on close.
The sink is **not** registered with `common/graceful`; its ordering is explicit in `main`
(§12 notes the fragility).

```
before: 10,000 req/s × 12 stmt = 120,000 stmt/s
after : 10,000 rec/s ÷ 500 per batch = ~20 INSERT/s   (at saturation; see Appendix D.2 for the low-load curve)
```

**W1.4 — Sampling with always-keep rules.**
The decision is made **at request end**, when status and duration are known — the recorder
is in memory, so this is free. The rule, in order (`common/tracing/sampling.go:40-57`,
quoted in Appendix B.2): a rate of 1.0 keeps everything; a forced trace is kept; a status
≥ 400 is kept when `TRACE_ALWAYS_SAMPLE_ERRORS` (default `true`); a duration
≥ `TRACE_ALWAYS_SAMPLE_SLOW_MS` is kept when that is > 0; a rate of 0 drops; otherwise a
uniform draw from `math/rand/v2` is compared with `TRACE_SAMPLE_RATE`. Forcing is a
server-side hook (`tracing.ForceTraceSample`) and is deliberately **not** wired to any
request header or parameter: a client-settable flag would let any caller defeat sampling and
re-inflate trace volume on demand. This is the head-plus-tail hybrid the OpenTelemetry
community recommends for cost-controlled pipelines, implemented in-process because one-api
already holds the full trace in one address space and needs no `decision_wait` buffer or
load-balancing collector tier. Under `TRACE_WRITE_MODE=sync` the sampler never runs.

Profile defaults: `standalone` 1.0; `scaled` 0.05 with `slow_ms=5000`; `external` 1.0 into
OTLP where the collector applies its own policy.

**W1.5 — Column-per-timestamp schema.**
Add nullable `BIGINT` columns `ts_request_received`, `ts_request_forwarded`,
`ts_first_upstream_response`, `ts_first_client_response`, `ts_upstream_completed`,
`ts_request_completed` to `traces` (additive `AutoMigrate`). The write path fills both the
JSON document and the columns; readers overlay the columns on the parsed document, so old
rows, new rows, and columns-only rows all resolve to the same view, and the API response
shape is unchanged (the columns are not serialized). *Plan as written: keep `timestamps`
"for `external_calls` only". Outcome: dual-write — see the delivery record.*

#### Phase 1 delivery record (2026-09-05)

Implemented, with tests, in:

| Work item | Where |
| --- | --- |
| W1.1 in-memory recorder | `common/tracing/recorder.go`, `ctxkey.TraceRecorder` (`common/ctxkey/key.go:393`), rewritten lifecycle helpers in `common/tracing/tracing.go` |
| W1.2 sink abstraction | `common/tracing/sink.go` (interface, selection, `multiSink`, `nullSink`), `common/tracing/sink_otlp.go` |
| W1.3 batched writer | `common/tracing/sink_sql.go`, `model.InsertTraces` and `model.NewTraceRow` in `model/trace_batch.go` |
| W1.4 sampling | `common/tracing/sampling.go`, applied in `RecordTraceEnd` |
| W1.5 per-timestamp columns | `Trace` struct in `model/trace.go:43-48`, projection and overlay in `model/trace_columns.go` |
| Configuration | `common/config/observability.go`, validators in `common/config/validation.go` |
| Metrics | `common/metrics/trace_pipeline.go`, `monitor/prometheus/recorder_trace_pipeline.go`, `monitor/otel/recorder_trace_pipeline.go` |
| Lifecycle wiring | `tracing.InitSinks` (`main.go:100`) after the database handles exist and before the HTTP server starts; `tracing.Shutdown` (`main.go:292`) |
| Read-after-write | `controller/tracing.go:39-57`: on a miss, flush the sink once with a 500 ms budget and retry, so a completed request is always findable under batched mode |

Four deliberate deviations from the plan above, all in the safer direction:

1. **W1.5 dual-writes rather than replaces.** The implementation writes the JSON document
   *and* the columns, and `GetTraceTimestamps` overlays the columns on top of the parsed
   document. Reason: once the write happens once per request, dropping the document buys
   almost nothing (a 6-field marshal is nanoseconds) while it would break any pre-migration
   binary reading a row a new binary wrote — the compatibility rule stated in §6. The
   columns therefore buy SQL-queryable latency analysis, not a write saving. Dropping the
   document later is a one-line change because the read path already prefers the columns.
2. **`TRACE_WRITE_MODE=sync` preserves the full legacy path.** Rather than deleting the
   per-mutation `SELECT`/`UPDATE` code, it is retained behind the flag and covered by a test
   that asserts it still issues those statements. The rollback escape hatch is therefore
   real rather than nominal. It is also the `standalone` default, because under batched
   mode a trace row does not exist until the request ends, so an in-flight five-minute
   streaming relay is no longer queryable while it runs — a capability today's users have.
3. **`TRACE_RETENTION_DAYS` profile defaults were not changed.** An earlier draft listed 7
   days under `scaled` and 0 under `external`; changing the retention default is only safe
   together with the chunked sweeper, and the variable keeps its existing 30-day default
   under every profile.
4. **Flat files instead of sub-packages.** The recorder, sampler, and sinks live directly in
   `common/tracing/` rather than in `common/tracing/recorder/` and `common/tracing/sink/`.
   The whole pipeline is about 1,000 lines across six files, every one well under the
   800-line limit, and keeping it in one package avoids an import layer that buys nothing.

Two behaviors were improved beyond the plan while touching this code:

- `TracingMiddleware` now calls `RecordTraceEnd` from a `defer`, so a panicking handler
  produces a complete trace instead of the partial row the pre-proposal path left behind.
  `gin.Recovery()` is registered earlier in the chain and still receives the panic.
- Synthetic gin contexts that never passed through the middleware (channel health tests, for
  example) carry no recorder, and under batched mode the lifecycle helpers become no-ops for
  them instead of issuing a `SELECT` that could only miss.

One incidental defect was found and fixed while touching this path: `CreateTrace` classified
duplicate-trace-id inserts with `errors.Is(err, gorm.ErrDuplicatedKey)`, which never matches
because this project does not enable `gorm.Config.TranslateError`. Both the synchronous and
the batched path now use `model.IsDuplicateTraceKeyError`, which also inspects the driver
error text.

Verification is recorded in §10.1.

### Phase 2 — Dashboard rollups (portable, no TimescaleDB) — design only

None of the identifiers in this phase exist in the code yet.

**W2.1 — Rollup tables.**

```sql
CREATE TABLE log_usage_rollup (
    bucket_start        BIGINT  NOT NULL,   -- unix seconds, UTC, hour-aligned
    grain               VARCHAR(8) NOT NULL,-- 'hour' | 'day'
    user_id             INT     NOT NULL DEFAULT 0,  -- 0 = site-wide bucket
    model_name          VARCHAR(191) NOT NULL DEFAULT '',
    token_name          VARCHAR(191) NOT NULL DEFAULT '',
    request_count       BIGINT  NOT NULL DEFAULT 0,
    quota               BIGINT  NOT NULL DEFAULT 0,
    prompt_tokens       BIGINT  NOT NULL DEFAULT 0,
    completion_tokens   BIGINT  NOT NULL DEFAULT 0,
    cached_prompt_tokens BIGINT NOT NULL DEFAULT 0,
    cache_hit_count     BIGINT  NOT NULL DEFAULT 0,
    cache_hit_quota     BIGINT  NOT NULL DEFAULT 0,
    elapsed_time_sum    BIGINT  NOT NULL DEFAULT 0,
    error_count         BIGINT  NOT NULL DEFAULT 0,
    updated_at          BIGINT  NOT NULL,
    PRIMARY KEY (grain, bucket_start, user_id, model_name, token_name)
);
CREATE INDEX idx_rollup_scan ON log_usage_rollup (grain, user_id, bucket_start);
```

A parallel `tool_usage_rollup` serves the three tool-statistics endpoints. Both are plain
relational tables and work identically on SQLite, MySQL, and PostgreSQL — no TimescaleDB
continuous aggregate and no engine-specific materialized view, which keeps the
zero-dependency contract. The measures match exactly what the six existing aggregates
return (§1.6), so the API response shape does not change.

**W2.2 — Incremental aggregator with a watermark.**
A background worker processes **closed** buckets only:

```
watermark = last fully aggregated bucket_start
loop every ROLLUP_INTERVAL_SEC (default 60):
    for each closed bucket in (watermark, now - grain):
        aggregate from logs WHERE created_at >= b AND created_at < b+grain
        UPSERT into log_usage_rollup
        advance watermark
```

Cost: one bounded-range aggregate per hour per grain instead of six unbounded aggregates
per page view. Range predicates stay on the raw `created_at` column with no function
wrapping, so partition pruning and index range scans apply.

The worker must run on exactly one node. Today no background worker in the trace or
retention area is master-gated (§1.2, §12), so this phase must either gate on
`IsMasterNode` or take the database-level advisory lock the compact-UUID coordinator uses
(a session advisory lock on PostgreSQL, `GET_LOCK` on MySQL, a file lock on SQLite, keyed by
the database's own identity rather than the DSN). The lock is an efficiency device, not a
correctness dependency: the upsert is idempotent, so two workers racing merely waste work.

Late writes: a provisional row (type 6) becomes a consume row (type 2) only after
post-consume reconciliation, which can land after its bucket closed (§1.5.2). The
aggregator therefore re-aggregates the trailing `ROLLUP_REAGGREGATE_BUCKETS` (default 2)
buckets on every cycle, and a nightly job recomputes whole days and emits a drift metric
(§11.4).

**W2.3 — Real-time read path.**
Dashboard queries become `rollup[start, watermark)` ∪ `live aggregate[watermark, end)`. The
live portion covers at most one grain (≤ 1 hour, ≤ 36M rows at 10k RPS, and it is a
contiguous index range on `created_at`). This reproduces the semantics of a TimescaleDB
real-time continuous aggregate using portable SQL.

**W2.4 — Log list pagination without `COUNT(*)`.**
- Default sort (`created_at DESC, id DESC`) switches to keyset pagination:
  `WHERE (created_at, id) < (?, ?)`. The frontend already has a page-based contract, so the
  API gains `cursor`/`next_cursor` fields and keeps `p` working for the first N pages.
- `total` becomes `total_estimated` when the exact count would scan more than
  `LOG_COUNT_EXACT_MAX_ROWS` (default 100000): serve the estimate from `log_usage_rollup`,
  and have the UI render `1000+`.
- Exact counts, when requested with filters, are cached in Redis for
  `DASHBOARD_CACHE_TTL_SEC`.

**W2.5 — Index diet.**
Gate the five sorting-only indexes (`quota`, `prompt_tokens`, `completion_tokens`,
`elapsed_time`, `cached_prompt_tokens`) plus `origin_model_name` behind
`LOG_SORT_INDEXES_ENABLED` (default `true` in `standalone`, `false` in `scaled`/`external`).
Add the composite indexes the queries actually want:

```sql
CREATE INDEX idx_logs_type_created  ON logs (type, created_at);
CREATE INDEX idx_logs_user_created  ON logs (user_id, type, created_at);
```

Removing six B-trees from a 10k-inserts/second table is the single cheapest write win after
Phase 1. Drops are performed only on explicit operator opt-in, are logged, and are reversible
by flipping the flag back and restarting. The existing `index_username_model_name`
(`model_name, username`) and `idx_created_at_type` (`created_at, type`) should be
re-examined in the same pass: the latter leads with the range column, which is the wrong
order for a `type = ? AND created_at BETWEEN` predicate.

**W2.6 — Optional `logs` retention.**
`LOG_DB_RETENTION_DAYS` (default 0 = disabled, preserving today's behavior; recommended 90
in `scaled`, 30 in `external` where the OLAP mirror holds history). Uses the same chunked
sweeper from W0.2 against the `LOG_DB` handle. Rollups are never swept — they are the
long-horizon record and are ~5 orders of magnitude smaller.

### Phase 3 — OTLP-native peripheral telemetry — partly shipped

**W3.1 — `otlpSink` for traces.** *Shipped in Phase 1.* Emits one self-contained span
named `one_api.request` per completed request (not events on the `otelgin` server span,
which has already ended by the time the request completes), with attributes
`one_api.trace_id`, `one_api.url`, `one_api.method`, `one_api.body_size`,
`one_api.status`, one event per lifecycle timestamp, one `one_api.external_call` event per
tool call, and error status for HTTP ≥ 400. With `TRACE_SINK=otlp` the `traces` table
receives zero writes. The sink resolves its tracer from the global provider, so it exports
only when `OTEL_ENABLED=true` (§12 records the failure mode when it is not).

**W3.2 — Application logs over OTLP.** Add `APP_LOG_SINK=otlp` using
`go.opentelemetry.io/otel/sdk/log` with `otlploghttp` and the
`go.opentelemetry.io/contrib/bridges/otelzap` bridge — the project's logger is already a
zap fork. The Logs API/SDK reached release-candidate status in 2026 and is in production
use; it is nonetheless gated off by default and documented as the newest of the three
signals. Log records automatically carry `trace_id`/`span_id`, giving log↔trace correlation
the file sink cannot provide.

**W3.3 — Dashboard-grade metrics.** Extend `common/metrics.MetricsRecorder` with the
counters/histograms the built-in dashboard shows (requests, quota, tokens, cache-hit ratio,
latency percentiles, per-model and per-group breakdowns) under the existing bounded-label
discipline. This enables a fully SQL-free operational view via `docs/grafana-dashboard.json`
and is what `external` deployments should treat as the primary dashboard.

**W3.4 — Reference collector configuration.** Ship `docs/ops/otel-collector-scaled.yaml`
documenting the two-tier gateway pattern: agent collectors near the gateway,
`loadbalancing` exporter keyed on trace ID, and a gateway tier applying `tail_sampling`
with `decision_wait` set to 2–3× observed p99. Include a ClickHouse exporter example and a
Loki/Tempo example.

### Phase 4 — Optional OLAP analytics backend — design only

**W4.1 — `UsageStore` interface.** New package `model/analytics`:

```go
// UsageStore abstracts where per-request usage events are written and where
// dashboard aggregates are read from, so a deployment can keep everything in the
// primary SQL database or offload analytics to a columnar store.
type UsageStore interface {
    // RecordUsage persists a batch of usage events. Implementations may buffer.
    RecordUsage(ctx context.Context, events []UsageEvent) error
    // QueryDailyByModel returns per-day, per-model aggregates over [start, end).
    QueryDailyByModel(ctx context.Context, q AggregateQuery) ([]*dto.LogStatistic, error)
    // QueryDailyByUser and QueryDailyByToken mirror QueryDailyByModel.
    QueryDailyByUser(ctx context.Context, q AggregateQuery) ([]*dto.LogStatisticByUser, error)
    QueryDailyByToken(ctx context.Context, q AggregateQuery) ([]*dto.LogStatisticByToken, error)
    // SearchEvents serves the detail/log-list view.
    SearchEvents(ctx context.Context, q SearchQuery) (EventPage, error)
    // Flush and Close mirror TraceSink semantics.
    Flush(ctx context.Context) error
    Close(ctx context.Context) error
}
```

`ANALYTICS_BACKEND=sql` (default) is the existing `logs`+rollup code behind the interface —
a pure refactor with no behavior change. `ANALYTICS_BACKEND=clickhouse` adds a mirror.

**W4.2 — ClickHouse schema.**

```sql
CREATE TABLE usage_events (
    created_at      DateTime64(3, 'UTC'),
    user_id         UInt32,
    user_uuid       String,
    token_name      LowCardinality(String),
    model_name      LowCardinality(String),
    origin_model    LowCardinality(String),
    channel_id      UInt32,
    channel_name    LowCardinality(String),
    api_format      LowCardinality(String),
    status          UInt16,
    is_stream       UInt8,
    quota           Int64,
    prompt_tokens   UInt32,
    completion_tokens UInt32,
    cached_prompt_tokens UInt32,
    elapsed_ms      UInt32,
    request_id      String,
    trace_id        String,
    content         String,
    metadata        String
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, created_at)
TTL toDateTime(created_at) + INTERVAL 180 DAY;

CREATE MATERIALIZED VIEW usage_daily_by_model_mv
ENGINE = SummingMergeTree
ORDER BY (day, user_id, model_name)
AS SELECT
    toDate(created_at) AS day, user_id, model_name,
    count()             AS request_count,
    sum(quota)          AS quota,
    sum(prompt_tokens)  AS prompt_tokens,
    sum(completion_tokens) AS completion_tokens,
    sum(cached_prompt_tokens) AS cached_prompt_tokens,
    countIf(cached_prompt_tokens > 0) AS cache_hit_count
FROM usage_events GROUP BY day, user_id, model_name;
```

`LowCardinality` on the label-like columns and `ORDER BY (user_id, created_at)` match the
dashboard's dominant predicate. The `SummingMergeTree` materialized view gives
O(days × models) dashboard reads.

**W4.3 — Ingestion.** Batch at the application layer (reuse the Phase-1 batched writer
shape: 500 rows or 1 s) and issue synchronous inserts. Do **not** stack application-level
batching on top of ClickHouse `async_insert` — when the client already batches, async
inserts add latency without benefit. Expose `CLICKHOUSE_ASYNC_INSERT` (default `false`) for
operators who prefer server-side buffering, in which case `wait_for_async_insert=1` is the
recommended durable setting.

**W4.4 — Source-of-truth policy.** SQL stays authoritative for billing. ClickHouse is a
mirror. A reconciliation job compares `SUM(quota)` per day between the two stores for the
trailing `N` days and emits `one_api_analytics_mirror_drift_ratio`; drift beyond a
threshold logs an ERROR and (optionally) fails the dashboard back to the SQL path. This is
the same OLTP/OLAP split that Langfuse and Helicone converged on after outgrowing Postgres:
transactional entities stay relational, high-cardinality events move to a columnar store
fed asynchronously.

**W4.5 — Dependency hygiene.** The ClickHouse driver is imported only from
`model/analytics/clickhouse`, behind the `clickhouse` build tag, so the default binary
neither links nor initializes it. Document that `ANALYTICS_BACKEND=clickhouse` requires a
build produced with `-tags clickhouse`, and ship that tag in the official images. (A
ClickHouse GORM driver is already present transitively in the module graph; it is not
imported by any one-api code.)

An alternative worth recording: TrueFoundry's AI gateway rejected ClickHouse operationally
and stores Parquet in the customer's S3 bucket behind Delta Lake, queried with DataFusion,
citing zero operational overhead and data sovereignty. That is a defensible design, but it
needs a query engine one-api does not embed. The OTLP path (Phase 3) already gives
sovereignty-conscious operators a vendor-neutral escape hatch without one-api owning a
lakehouse.

---

## 6. Backward-Compatibility Contract

Added 2026-09-06, after an audit of the implemented Phases 0 and 1 against the principle
below. This is the rule the rest of the document relies on; it originates in this proposal
(§1.8).

**The rule.** An operator who upgrades to a build containing this work, without changing
their configuration, must observe: no file or row deleted that the previous version kept,
no output contract changed, no capability removed, and no schema change that a rollback to
the previous binary cannot tolerate.

**How it is met.** Every optimization that would violate that rule is reachable only through
`OBSERVABILITY_PROFILE` or its own variable. The standalone defaults reproduce pre-proposal
behavior exactly:

| Optimization | Standalone default | Why it must be opt-in |
| --- | --- | --- |
| Log file retention (`LOG_RETENTION_DAYS`) | `0`, off | Deletes files the operator kept; retention was off before |
| Log directory size ceiling (`LOG_MAX_TOTAL_SIZE_MB`) | `0`, off | Same |
| Free-disk guard (`LOG_MIN_FREE_DISK_MB`) | `0`, off | Deletes files *and* raises the process log level |
| Compact per-request log line (`LOG_RECORD_LINE_FORMAT`) | `full` | An operator's log pipeline may parse the dropped fields |
| Application log sampling (`LOG_SAMPLE_INITIAL`) | `0`, off | Silently drops entries |
| Batched trace writes (`TRACE_WRITE_MODE`) | `sync` | A batched trace is not queryable until the request ends, so an in-flight streaming relay loses its live timeline |
| Trace path exclusions (`TRACE_EXCLUDED_PATH_PREFIXES`) | empty | Removes trace coverage that exists today |
| Trace sampling (`TRACE_SAMPLE_RATE`) | `1.0` | Discards traces |
| Dashboard aggregate cache (`DASHBOARD_CACHE_TTL_SEC`) | `0`, off | Serves data up to a minute stale where it was live |
| Site-wide dashboard range cap (`DASHBOARD_MAX_SITEWIDE_RANGE_DAYS`) | `365`, the existing limit | Rejects queries that previously ran |

`OBSERVABILITY_PROFILE=scaled` turns all of them on together. That is the intended path for
a deployment that actually has the volume problem.

What *did* change for every profile, and why it is compatible: the retention sweep interval
(24 h → 1 h, each sweep smaller), the sweeps being chunked rather than single statements
(same rows removed, no lock held for the duration), the retention cleaner logging one
warning at startup when all guards are off, `RecordTraceEnd` running from a `defer` (a
panicking handler now yields a complete trace rather than a partial row), and the six
nullable `ts_*` columns described next.

**Storage.** The six `ts_*` columns are additive and nullable, and the `timestamps` JSON
document is still written in full. A pre-change binary can read, update, and insert rows
this build wrote; this was verified against a real worktree of `397781e1` on PostgreSQL and
is pinned by `model.TestOldBinaryCanReadRowsWrittenByNewCode` and
`TestNewCodeReadsRowsWrittenByOldBinary` (`model/trace_compat_test.go`).

**Unmigrated schemas.** Only the master runs `AutoMigrate` (§1.2), so a `NODE_TYPE=slave`
upgraded ahead of its master sees a `traces` table without the new columns. The write path
probes for them (once per minute until found, then sticky) and omits them when absent
(`model.traceTimestampColumnsAvailable`), so such a node keeps recording complete traces
instead of losing all of them. Pinned by `model.TestTraceWritesSucceedOnUnmigratedSchema`.

**Go API.** `model.DeleteOldLog(targetTimestamp)` keeps its pre-change signature (it now
wraps the chunked variant with a background context); `DeleteOldLogContext(ctx, …)` is the
new variant, and the admin purge endpoint calls it with a context detached from the request
so a client disconnect cannot leave a multi-minute purge half done. `metrics.MetricsRecorder`
is unchanged: trace-pipeline metrics are delivered through the optional
`metrics.TracePipelineRecorder` extension (Appendix B.3), so an out-of-tree recorder still
compiles. Pinned by the mock in `relay/billing/billing_monitoring_test.go`, which implements
only the pre-change interface.

**Guarantees are tested, not asserted.** `common/config/compat_test.go` fails if any of the
standalone defaults above drifts. It pins: `LogRetentionDays == 0`, `LogMaxTotalSizeMB == 0`,
`LogMinFreeDiskMB == 0`, `LogRecordLineFormat == full`, `LogSampleInitial == 0`,
`AppLogSink == both`, `DashboardCacheTTLSec == 0`, `DashboardMaxSitewideRangeDays == 365`,
`TraceWriteMode == sync`, `TraceExcludedPathPrefixes` empty, `TraceSinks == [db]`,
`TraceSampleRate == 1.0`. Not pinned (free to drift without a test failure): the batch,
flush, queue, and writer-count sizes, the `RETENTION_*` knobs, the sampling `THEREAFTER` and
`TICK` values, and `TRACE_RETENTION_DAYS`.

---

## 7. Configuration Surface

### 7.1 Semantics that apply to every variable

Summarized from §1.7: an explicitly set variable always overrides the profile; an empty
value is treated as unset; an unparseable number silently takes the default; list values
are disabled with `-`; unknown enumerations fall back to the safe value **except**
`TRACE_WRITE_MODE`, where any value other than `sync` selects `batched` (§12). All
observability validators run after clamping and cannot fail startup from the environment.

### 7.2 Implemented variables

The `standalone` column is the default when `OBSERVABILITY_PROFILE` is unset. Bold entries
are the ones whose standalone value exists to preserve pre-proposal behavior (§6).

| Variable | standalone | scaled | external | Meaning | Consumed by |
| --- | --- | --- | --- | --- | --- |
| `OBSERVABILITY_PROFILE` | `standalone` | — | — | preset selector; unknown → `standalone` | every row below |
| `TRACE_SINK` | `db` | `db` | `otlp` | comma-separated `db`, `otlp`, `none`; unknown tokens dropped; empty → `db` | `tracing.InitSinks` |
| `TRACE_WRITE_MODE` | **`sync`** | `batched` | `batched` | `sync` is the pre-proposal per-mutation write path; anything else is `batched` | `common/tracing/tracing.go` |
| `TRACE_SAMPLE_RATE` | **`1.0`** | `0.05` | `1.0` | base sampling probability, clamped to [0,1] | `tracing.SampleDecision` |
| `TRACE_ALWAYS_SAMPLE_ERRORS` | `true` | `true` | `true` | keep status ≥ 400 | `tracing.SampleDecision` |
| `TRACE_ALWAYS_SAMPLE_SLOW_MS` | `0` | `5000` | `0` | keep requests at least this long; 0 disables | `tracing.SampleDecision` |
| `TRACE_BATCH_SIZE` | `500` | `500` | `500` | rows per INSERT | `sqlSink` |
| `TRACE_FLUSH_INTERVAL_MS` | `1000` | `1000` | `1000` | max age of a partial batch | `sqlSink` |
| `TRACE_QUEUE_SIZE` | `20000` | `50000` | `20000` | bounded backlog; a full queue drops the newest record | `sqlSink` |
| `TRACE_WRITER_COUNT` | `2` | `4` | `2` | flush goroutines | `sqlSink` |
| `TRACE_EXCLUDED_PATH_PREFIXES` | **empty** | `/api/status,/metrics,/health,/static,/assets,/favicon` | same as scaled | never-traced path prefixes; `-` disables | `tracing.PathExcluded` |
| `TRACE_RETENTION_DAYS` | `30` *(pre-existing)* | `30` | `30` | trace sweeper window; 0 disables | `model.StartTraceRetentionCleaner` |
| `ASYNC_TASK_RETENTION_DAYS` | `7` *(pre-existing)* | `7` | `7` | async-task binding sweeper window | `model.StartAsyncTaskRetentionCleaner` |
| `RETENTION_DELETE_BATCH_SIZE` | `5000` | `5000` | `5000` | rows per retention DELETE | `model.ChunkedDelete` |
| `RETENTION_DELETE_PAUSE_MS` | `10` | `10` | `10` | pause between chunks; chosen from a measured sweep (Appendix D.3) | all sweepers |
| `RETENTION_SWEEP_INTERVAL_MINUTES` | `60` | `60` | `60` | how often the trace, async-task, and log-file sweepers run | all sweepers |
| `LOG_RECORD_LINE_FORMAT` | **`full`** | `compact` | `compact` | shape of the per-request `record log` INFO line | `model.recordLogHelper` |
| `DASHBOARD_CACHE_TTL_SEC` | **`0`** | `60` | `60` | Redis cache of the six aggregates, and in-process cache of the site-wide `users` aggregate; 0 disables both | `controller/user_dashboard_cache.go`, `model/user_stats_cache.go` |
| `DASHBOARD_MAX_SITEWIDE_RANGE_DAYS` | **`365`** | `31` | `31` | site-wide range cap; per-user is never capped | `controller.GetUserDashboard` |
| `APP_LOG_SINK` | `both` | `both` | `both` | `file` \| `stdout` \| `both`; `stdout` disables file retention | `logger.SetupLogger` |
| `LOG_RETENTION_DAYS` | **`0`** | `3` | `1` | file age retention by modification time; 0 disables | `logger.StartLogRetentionCleaner`, rotation sink |
| `LOG_MAX_TOTAL_SIZE_MB` | **`0`** | `20480` | `10240` | directory size ceiling; 0 = unlimited | disk guard |
| `LOG_MIN_FREE_DISK_MB` | **`0`** | `1024` | `1024` | free-space guard; 0 disables; Unix only | disk guard |
| `LOG_SAMPLE_INITIAL` | **`0`** | `100` | `100` | entries kept per `(level, message)` per tick; 0 disables sampling | `logger.SetupEnhancedLogger` |
| `LOG_SAMPLE_THEREAFTER` | `100` | `100` | `100` | keep every Nth after the initial budget | same |
| `LOG_SAMPLE_TICK_MS` | `1000` | `1000` | `1000` | sampling window | same |

Existing variables the design depends on but does not change: `SQL_DSN`, `SQLITE_PATH`,
`LOG_SQL_DSN`, `REDIS_CONN_STRING`, `NODE_TYPE`, `SYNC_FREQUENCY` (default 120 s),
`BATCH_UPDATE_ENABLED` (default `false`) and `BATCH_UPDATE_INTERVAL` (default 5 s),
`OTEL_ENABLED` (default `false`), `OTEL_EXPORTER_OTLP_ENDPOINT`, `ENABLE_PROMETHEUS_METRICS`
(default `true`), `METRICS_TOKEN`, `LOG_LEVEL`, `LOG_ROTATION_INTERVAL` (default `daily`),
`ONLY_ONE_LOG_FILE`, the `--log-dir` flag, `SHUTDOWN_TIMEOUT` (default 360 s), and the pool
sizes `SQL_MAX_IDLE_CONNS` / `SQL_MAX_OPEN_CONNS` / `SQL_MAX_LIFETIME`. `BATCH_UPDATE_ENABLED`
should be documented as effectively mandatory above ~500 RPS.

### 7.3 Planned variables (Phases 2–4; none exist in the code today)

| Variable | Phase | Proposed default | Meaning |
| --- | --- | --- | --- |
| `DASHBOARD_ROLLUP_ENABLED` | 2 | `false` | enable rollup tables, the aggregator, and rollup-backed reads |
| `ROLLUP_INTERVAL_SEC` | 2 | `60` | aggregator cadence |
| `ROLLUP_REAGGREGATE_BUCKETS` | 2 | `2` | trailing closed buckets recomputed each cycle to absorb late reconciliation |
| `LOG_COUNT_EXACT_MAX_ROWS` | 2 | `100000` | above this, the log list returns an estimated total |
| `LOG_SORT_INDEXES_ENABLED` | 2 | `true` standalone, `false` scaled/external | keep or drop the six sorting-only indexes on `logs` |
| `LOG_DB_RETENTION_DAYS` | 2 | `0` | chunked age-based retention for the `logs` table; 0 disables |
| `APP_LOG_SINK=otlp` | 3 | — | new value for the existing variable |
| `ANALYTICS_BACKEND` | 4 | `sql` | `sql` or `clickhouse` (requires a `-tags clickhouse` build) |
| `CLICKHOUSE_ASYNC_INSERT` | 4 | `false` | server-side buffering instead of client batching |

---

## 8. Expected Impact

> **Status note (2026-09-06).** The table below was an estimate written before
> implementation. Phases 0 and 1 have since been measured against real PostgreSQL 17,
> MySQL 8.4 and SQLite, with a no-tracing floor arm, a work-conservation metric (WAL bytes
> per trace), and a cross-tree baseline at commit `397781e1`. Read the companion benchmark
> document (summarised in Appendix D) in preference to this table, which is retained for the
> record. The "After Phase 0+1" column describes the `scaled` profile, not what an operator
> gets by upgrading with unchanged configuration.
>
> Measurement contradicted this proposal in three places:
>
> 1. **W0.2's benefit is real but narrower than claimed, and the default pause was wrong.**
>    Under a fixed-rate concurrent workload, chunking cuts the longest DELETE 6–22× and
>    reduces per-request degradation on all three engines (PostgreSQL 11.76% → 0.00%). But
>    on SQLite it increases the TOTAL number of degraded requests, because spreading the
>    same work over a longer window exposes more of them. A measured sweep over the
>    inter-chunk pause moved the default from 100 ms to 10 ms: PostgreSQL is already at zero
>    degradation there, MySQL is at its optimum, and throughput is 1.6–4.5× higher, which is
>    what keeps an hourly sweep ahead of arrivals.
> 2. **Statements per trace is load-dependent**, not the constant 0.002 claimed here: 0.053
>    at 50 req/s.
> 3. **The W0.4 log saving is ~141 GB/day only with sampling off**, and the profiles that
>    reach 10k req/s enable sampling by default, where it is worth ~2.8 GB/day.

At 10,000 RPS, with the assumptions of §3.

| Metric | Today | After Phase 0+1 (`scaled`) | After Phase 2 | After Phase 3+4 |
| --- | --- | --- | --- | --- |
| SQL statements/s (trace) | ~120,000 | ~20 (batched) | ~20 | 0 |
| SQL statements/s (usage log) | 10,000 | 10,000 | ~50 (batched) | ~50 |
| Trace rows/day | 864M | 43M @ 5% sampling | 43M | 0 |
| `traces` storage @ retention | ~15.6 TB (30 d) | ~780 GB (30 d, 5%) | ~780 GB | 0 |
| Index maintenance per usage INSERT | 17 B-trees | 17 | 11 | 11 |
| Dashboard rows scanned (7 d, site-wide) | ~6.05 B × 6 queries | same, once per `DASHBOARD_CACHE_TTL_SEC` | ≤ 36M once + rollup read | rollup / MV read |
| Dashboard p95 | timeout | timeout on a cold cache | < 500 ms (target) | < 200 ms (target) |
| Log-list page load | full `COUNT(*)` | same | keyset, no count | keyset |
| App log bytes/day | ~1.35 TB, never deleted | sampled and capped by size/age/free-disk | same | 0 on local disk |

The Phase 0+1 line is the important one: it is roughly a **99.98% reduction in
trace-related SQL statements** with no new dependency, no OLAP store, and no frontend
change.

---

## 9. Interfaces and File Layout

New code goes in new files. `model/log.go` is already over 1,700 lines, well past the
800-line guidance (§1.8); this work should not add to it.

### 9.1 As implemented (Phases 0 and 1)

```
common/config/observability.go           profile selectors and every new variable
common/config/compat_test.go             pins the standalone defaults (§6)
common/tracing/recorder.go               Recorder (in-memory, per request; Finish is single-shot)
common/tracing/sampling.go               SampleDecision at request end; ForceTraceSample hook
common/tracing/sink.go                   TraceSink interface, InitSinks/Sink/Flush/Shutdown, multiSink, nullSink
common/tracing/sink_sql.go               bounded queue, writer goroutines, batched INSERT, drain on close
common/tracing/sink_otlp.go              one span per request via the global tracer provider
common/tracing/time.go                   millisecond ↔ time.Time helpers
common/tracing/tracing.go                lifecycle helpers (batched and sync branches), path exclusion
common/metrics/trace_pipeline.go         TracePipelineRecorder extension + outcome vocabulary
monitor/prometheus/recorder_trace_pipeline.go, monitor/otel/recorder_trace_pipeline.go
model/trace_batch.go                     NewTraceRow, InsertTraces, duplicate classification, column probe
model/trace_columns.go                   ts_* projection and column-over-document overlay
model/trace_compat_test.go               old-binary / new-binary / unmigrated-schema tests
model/retention_chunk.go                 ChunkedDelete + per-engine bounded statement + table allow-list
model/trace_retention.go, model/async_task_retention.go   sweepers on the shared interval
model/user_stats_cache.go                in-process site-wide quota snapshot
controller/user_dashboard_cache.go       Redis aggregate cache; range measurement
common/logger/sampling.go                levelBoundedSampler
common/logger/disk_guard.go, disk_free_unix.go, disk_free_other.go   size ceiling, free-disk floor, level escalation
common/logger/log_retention.go           StartLogRetentionCleaner (age + ceiling + floor, hourly)
common/benchdb/benchdb.go                benchmark database provisioning (PostgreSQL/MySQL opt-in, SQLite default)
```

### 9.2 Planned (Phases 2–4)

```
model/log_rollup.go                      rollup schema + upsert
model/log_rollup_worker.go               incremental aggregator + watermark + leader gating
model/log_query_rollup.go                rollup-backed dashboard queries (rollup ∪ live union)
model/log_pagination.go                  keyset pagination + estimated counts
model/analytics/store.go                 UsageStore interface + AggregateQuery/SearchQuery
model/analytics/sqlstore/                default implementation over logs + rollups
model/analytics/clickhouse/              build-tagged ClickHouse implementation
common/logger/otlp_sink.go               otelzap bridge wiring
docs/ops/otel-collector-scaled.yaml      reference collector config
docs/ops/scaling-runbook.md              operator-facing runbook
```

Per §1.8: every exported function carries a comment starting with its name and describing
purpose, parameters, and return values; errors are wrapped with
`github.com/Laisky/errors/v2` and handled exactly once; reads use explicit SQL and writes
use GORM; request paths use `gmw.GetLogger(c)` once per function; all times are UTC; date
ranges stay half-open.

---

## 10. Testing and Acceptance

**Unit (testify `require`)**
- `Recorder`: concurrent timestamp marks, external-call append ordering, no-I/O guarantee
  (a sink that fails the test if called before request end).
- Sampling: rate boundaries, always-keep for status ≥ 400 and slow requests, determinism
  given an injected decider.
- Batched sink: flush on size, flush on interval, drop accounting on a full queue,
  flush-on-shutdown completeness.
- Rollup aggregator (Phase 2): watermark advancement, idempotent re-processing of a bucket,
  correctness of the rollup ∪ live union against a direct aggregate on the same fixture
  (this is the key correctness test), and absorption of a provisional→consume flip that
  lands after its bucket closed.
- Chunked delete: total rows removed equals the unbounded equivalent; loop terminates;
  respects context cancellation.
- Keyset pagination (Phase 2): no duplicates and no gaps across page boundaries with
  identical `created_at` values.
- Disk guard: ceiling enforcement deletes oldest-first; free-space threshold triggers level
  escalation.

**Multi-database**
Follow the existing `*_multidb_test.go` pattern so rollup DDL, upsert syntax, and chunked
deletes are verified on SQLite, MySQL, and PostgreSQL.

**Load / acceptance gates**
A synthetic relay target and a load generator, asserting at 10k RPS on a reference
8 vCPU / 32 GB PostgreSQL (the sizing LiteLLM publishes for the 1k–5k RPS band,
deliberately under-provisioned to prove headroom):

| Gate | Threshold |
| --- | --- |
| Added relay latency p99 from observability | ≤ 2 ms |
| Trace-related statements/s | ≤ 100 |
| Dropped trace records | ≤ 0.1% at `TRACE_QUEUE_SIZE` default |
| Dashboard p95, 7-day site-wide, 1M users | ≤ 500 ms |
| Log-list first page p95 | ≤ 300 ms |
| App log write rate | ≤ 2 MB/s under the `scaled` profile (`LOG_SAMPLE_INITIAL=100`, compact line) |
| Rollup vs raw aggregate drift, trailing 7 days | 0 |
| Graceful shutdown loses buffered usage logs | 0 rows |

**Regression**
Existing `model/log_dashboard_test.go`, `model/trace_test.go`,
`model/trace_retention_test.go`, and `controller/log_test.go` must pass unchanged under
`OBSERVABILITY_PROFILE=standalone`. That is the compatibility proof.

### 10.1 Phase 1 verification record (2026-09-05)

`go vet ./...` clean. `go test ./... -count=1` and `go test -race ./... -count=1` pass.

Tests added:

| File | Covers |
| --- | --- |
| `common/tracing/recorder_test.go` | every lifecycle key lands in the document; unknown keys reported; `Finish` single-shot; concurrent marks under `-race`; snapshot isolation from post-`Finish` appends; external-call defaults; duration from the completion mark; nil-receiver safety |
| `common/tracing/sampling_test.go` | rate boundaries; always-keep for errors, slow requests, and forced traces; strictness of the probabilistic comparison; test-hook reversibility |
| `common/tracing/sink_sql_test.go` | flush on batch size; flush on interval; drop-and-count on a saturated queue; no loss on `Close`; late submit dropped without panicking; `Flush` honors its deadline |
| `common/tracing/sink_test.go` | fan-out continues past a failing child sink; `Sink()` never nil before `InitSinks`; `TRACE_SINK` selection for `db`/`otlp`/`none`/fan-out; unknown sink rejected; OTLP sink writes no SQL |
| `model/trace_batch_test.go` | URL redaction and length bound; timestamp projection onto columns; column-over-document overlay including legacy and columns-only rows; multi-chunk batch insert with `BeforeCreate` UUIDs; duplicate trace id does not discard its batch; driver-text duplicate classification |
| `model/trace_compat_test.go` | old binary reads new rows; new binary reads old rows; writes succeed on an unmigrated schema |
| `middleware/tracing_batched_test.go` | the acceptance test (below) plus completeness of the single write, sampling drop, always-keep-errors, forced retention, panic-path recording, `sync`-mode rollback, and `TRACE_SINK=none` |

The acceptance test, `TestTracingMiddlewareIssuesNoStatementsOnRequestPath`, registers GORM
callbacks that count every issued statement and drives three requests through the real
middleware with the real relay lifecycle marks. It asserts:

- **0** statements of any kind on the request path (was ~12 per request);
- exactly **1** `INSERT` for the whole batch after the flush;
- **0** `SELECT` and **0** `UPDATE` against `traces` at any point;
- 3 rows persisted.

`TestTracingMiddlewarePersistsCompleteTrace` then asserts the single write carries everything
the six writes used to carry: all six timestamps, the external-call entry, the status, the
server-assigned UUID, the sanitized URL, and the populated per-timestamp columns.

One pre-existing test, `middleware/tracing_duplicate_traceid_test.go`, was updated: it
asserted row counts immediately after `ServeHTTP`, which no longer holds now that writes are
asynchronous. It now installs the batched sink and flushes before counting; its actual
intent — two requests sharing one OpenTelemetry trace id must produce two distinct rows — is
unchanged.

Measured cost of what remains on the request goroutine (`BenchmarkRecorderRequestLifecycle`,
`BenchmarkNewTraceRow`, AMD Ryzen 7 5700G, idle, 3 runs × 500k iterations):

| Stage | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| recorder: allocate + 5 marks + status + `Finish` | 765–788 | 288 | 8 |
| `model.NewTraceRow`: sanitize URL, marshal, project columns | 2100–2218 | 961 | 7 |
| **total added per request** | **~3 µs** | **~1.2 KB** | **15** |

That is the whole synchronous cost, against 12 database round trips before — three orders of
magnitude below the 2 ms p99 gate above. End to end under 16-way parallelism the batched
path costs 21–31 µs of wall time per request, which is not separable from the harness's
14–18 µs floor (Appendix D.1); both figures are true and measure different things.

Not covered by automated tests, and deliberately left to the Phase-1 rollout: the 10k RPS
load gates above. They need a load generator and a reference PostgreSQL instance, neither of
which belongs in `go test`. The two gates the unit suite does settle are "trace-related
statements/s" (now bounded by `rate / TRACE_BATCH_SIZE`, proven to be 1 INSERT per batch)
and "graceful shutdown loses buffered traces" (proven to be 0 on a clean close).

### 10.2 Phase 0 verification record (2026-09-05)

`go vet ./...` clean. `go test ./... -count=1` and `go test -race ./... -count=1` pass.

Tests added:

| File | Covers |
| --- | --- |
| `model/retention_chunk_test.go` | a chunked sweep removes exactly the rows an unbounded DELETE would; rows outside the predicate survive; the loop terminates when the count is an exact multiple of the batch size; a cancelled sweep reports what it removed instead of failing; the table allow-list rejects injection attempts and unlisted tables; a nil handle fails loudly; the bounded statement is correct for all three engines |
| `model/user_stats_cache_test.go` | the site-wide aggregate is recomputed at most once per TTL; an expired snapshot recomputes; a zero TTL disables caching entirely and populates no snapshot; a cache hit is independent of table size |
| `common/logger/disk_guard_test.go` | candidates are listed oldest-first and non-log entries are excluded; a missing directory is not an error; the size ceiling deletes oldest-first and always keeps the newest file; an under-budget directory is untouched; a zero budget disables the ceiling; an unsatisfiable free-disk floor purges every rotated file and escalates the level; recovery restores the configured level |
| `common/logger/sampling_test.go` | INFO chatter is thinned to the configured budget; the thereafter factor keeps every Nth; **WARN and ERROR are never thinned**; a rare message keeps its own budget rather than being starved by a chatty one; derived loggers stay sampled; sampling is off unless configured |
| `common/config/observability_test.go` | unknown profiles, sink names, and log sinks fall back to the safe value rather than silently changing behavior; sink list parsing, de-duplication, and the "typo must not discard traces" fallback; prefix list parsing including the `-` disable form; per-profile default selectors; numeric clamps; the fail-fast validators; duration conversions |
| `common/config/compat_test.go` | the standalone defaults listed in §6 |
| `common/tracing/exclusion_test.go` | prefix matching; an empty list traces everything; a nil or request-less context is treated as excluded rather than panicking; `TRACE_SINK=none` excludes every request |
| `middleware/tracing_batched_test.go` | end to end: excluded paths issue **zero** statements and create **zero** rows, while a relay path on the same engine is still traced |
| `controller/user_dashboard_cache_test.go` | range measurement including partial-day rounding and inverted ranges; the cache key is scoped by user and window and is stable; caching stays inert without a Redis client and with a zero TTL |

Behavior changes an operator must know about:

- **None of the standalone defaults delete data or change output** (§6). An operator who
  wants the protections sets `OBSERVABILITY_PROFILE=scaled` or the individual variables;
  the cleaner's startup warning names them.
- The admin-triggered `logs` purge (`DELETE /api/log/`) now deletes in chunks with a
  detached context. It therefore takes longer in wall-clock terms, and in exchange it no
  longer holds a table-wide lock for its whole duration and cannot be abandoned half-way by
  a client disconnect.
- Retention sweeps run hourly instead of daily on every profile.
- Under `scaled` and `external`, a root user requesting more than
  `DASHBOARD_MAX_SITEWIDE_RANGE_DAYS` of site-wide statistics gets an explanatory error
  instead of a request that would time out.

---

## 11. Migration, Compatibility, and Risk

### 11.1 Schema changes
All additive: six nullable columns on `traces` (shipped), two new rollup tables and two new
composite indexes on `logs` (Phase 2). Handled by the existing `AutoMigrate` step in
`model/main.go`, which runs on master nodes only. No column is dropped, renamed, or
retyped. Index *drops* under `LOG_SORT_INDEXES_ENABLED=false` (Phase 2) are the sole
destructive operation, are opt-in, are logged, and are recreated on the next boot if the
flag is flipped back.

### 11.2 Rollup backfill (Phase 2)
On first start with `DASHBOARD_ROLLUP_ENABLED=true`, the aggregator backfills historical
buckets oldest-first in bounded chunks with a pause between them, publishing progress
through metrics. Until the watermark reaches the requested range the dashboard transparently
falls back to the live query, so there is no period of wrong or missing data — only a period
of the current (slow) behavior.

### 11.3 Intentional behavior changes under the default profile
**None.** The earlier edition of this document changed the `LOG_RETENTION_DAYS` default to
7; the compatibility audit reversed that (§6). The changes that do apply to every profile —
hourly chunked sweeps, the startup warning, panic-path trace completion — remove nothing and
change no output. Release notes should still state that `OBSERVABILITY_PROFILE=scaled` is
the recommended setting for any deployment above the low hundreds of RPS.

### 11.4 Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Rollup drift vs raw logs (late writes, provisional→consume flips, clock skew) | Aggregate only closed buckets; re-aggregate the trailing `ROLLUP_REAGGREGATE_BUCKETS` (default 2) buckets each cycle; nightly full-day reconciliation job emitting a drift metric |
| Batched usage-log writes (Phase 2) lose rows on `kill -9` | Off by default; flush via `graceful.GoCritical`; bounded, documented loss window identical in kind to the existing `BATCH_UPDATE_ENABLED` trade-off; quota deduction is a separate, already-durable path |
| Batched trace writes (shipped) lose rows on a full queue or an unclean exit | Opt-in; drops are counted and logged; traces are best-effort by definition; the sync path remains one variable away. No benchmark yet exercises a saturated queue (§12) |
| Sampling hides the one trace an operator needs | Errors and slow requests are always kept; a server-side hook can force capture; the sample rate is a restart-time setting. There is deliberately no client-side override |
| Index drops slow ad-hoc admin sorting | Gated by profile; sorting is already capped at 30 days; the composite indexes added in W2.5 cover the common `type`+time and `user`+time paths |
| ClickHouse becomes a hidden hard dependency | Build-tagged, mirror-only, SQL remains source of truth, drift metric + automatic fallback |
| Multi-node duplicate rollup computation | Master gating or the compact-UUID advisory-lock coordinator (§5 W2.2); the upsert is idempotent so a race wastes work but cannot corrupt |
| OpenTelemetry Logs SDK API is not yet frozen | Off by default, isolated behind `common/logger/otlp_sink.go`; traces and metrics (both stable) are unaffected |
| Scope creep across five phases | Phases 0 and 1 are shipped and deliver the majority of the benefit; Phases 2–4 are separately reviewable |

---

## 12. Known Gaps and Open Questions

These were found while auditing the implementation for this revision. None is hidden in
the code; each is a decision the panel may want to weigh in on. Items marked **(shipped)**
are in the Phase 0/1 code; items marked **(design)** concern Phases 2–4.

1. **(shipped) `TRACE_WRITE_MODE` fails open into the behavior change.** Any value other
   than the exact string `sync` selects `batched`, so a typo such as `snyc` under the
   standalone profile silently opts a deployment into asynchronous trace writes
   (`common/config/observability.go:172-177`). Every other enumeration in this area fails
   closed (unknown profile → `standalone`, unknown sink → `db`). Proposed fix: unknown →
   `sync` plus a warning.
2. **(shipped) `external` profile without OpenTelemetry silently discards traces.**
   `TRACE_SINK=otlp` resolves its tracer from the global provider, which is a no-op unless
   `OTEL_ENABLED=true`; `OTEL_ENABLED` defaults to `false` independently of the profile. In
   that combination no span is exported, no SQL row is written, and the sink still counts
   the record as `exported` (`common/tracing/sink_otlp.go:114`). Nothing validates the
   combination. Proposed fix: a validator, or a fallback to `db` with a warning.
3. **(shipped) The observability validators are unreachable from the environment.** Every
   value is normalized or clamped before validation (§1.7), so `MustValidateEnvVars` can
   never reject a misconfigured observability variable; a nonsense value is silently
   replaced. This is safe but misleading to an operator reading the validator list.
4. **(shipped, pre-existing) Retention sweepers run on every node.** The trace and
   async-task sweepers are started without a master check (`main.go:96`, `main.go:103`),
   so an N-node deployment issues N concurrent chunked sweeps of the same tables on the same
   cadence. Idempotent, but N× the delete load, and the per-node "deleted N rows" logs
   disagree. Phase 2's aggregator must not repeat this (§5 W2.2), and the sweepers should
   probably be gated the same way.
5. **(pre-existing) `traces` cannot leave the primary database by configuration.**
   `LOG_SQL_DSN` moves only `logs` and `data_migrations` (§1.2). Until Phase 3's OTLP sink
   is the operator's choice, every trace write lands on the account database. Whether to
   route `traces` to `LOG_DB` as well is an open question; it would be a one-line change in
   `model/trace_batch.go` and `model/trace_retention.go` but it changes which database owns
   the table.
6. **(pre-existing) No automatic `logs` retention.** Plane B is unbounded until W2.6
   ships. In the meantime the only lever is the admin purge endpoint.
7. **(shipped) The Redis dashboard cache has no single-flight and no invalidation.** N
   concurrent cold requests for the same key all run the six aggregates; entries expire
   only by TTL, so a key whose range ends today serves numbers up to one TTL stale. Both
   are acceptable at a 60 s TTL and are stated here so they are not mistaken for
   guarantees. The in-process site-wide `users` snapshot, by contrast, is a mutex over the
   query and therefore does coalesce concurrent misses (Appendix D.5).
8. **(pre-existing) The log-list `COUNT(*)` is unbounded and has no timeout**, and the
   30-day cap applies only to sorted queries with both timestamps (§1.6). Phase 2 W2.4
   addresses it; until then an admin opening the unfiltered log page on a large table pays
   a full index scan per page.
9. **(pre-existing) Two independent file-retention mechanisms share one variable.** The
   rotation sink purges by the date stamp in the file name whenever it rotates; the
   retention cleaner purges by modification time on its own hourly schedule. Both read
   `LOG_RETENTION_DAYS`. They agree in practice, but an operator who touches a rotated file
   will see it survive one mechanism and not the other.
10. **(pre-existing) Level handling has two inconsistencies.** `SetupEnhancedLogger`
    forces the level to `info` or `debug`, overriding a `LOG_LEVEL=warn` chosen at startup;
    and the disk guard's recovery restores the *startup* level rather than the forced one.
    Neither affects the proposal's correctness, but the disk guard's escalation and
    recovery are built on this level machinery.
11. **(shipped) `tracing.ForceTraceSample` has no production caller.** It exists so that a
    server-side call site (for example, a channel test or an admin-flagged request) can pin
    a trace; nothing uses it yet.
12. **(shipped) Metric semantics to know when reading the counters.** `sampled_out` is
    counted before any sink runs; `nullSink` counts nothing; `exported` is counted whether
    or not a real provider is installed (item 2); `dropped_closed` counts submissions that
    arrive after shutdown began.
13. **(shipped) The batched writer's durability is not benchmarked at saturation.** Every
    benchmark arm submits 5,000 traces into a 20,000-entry queue, so the "0 traces lost"
    result is evidence for an under-provisioned-queue-free run only. Two loss modes exist by
    design: a full queue drops the newest record, and an unclean exit loses queued records.
    Sizing `TRACE_QUEUE_SIZE` and watching `oneapi_trace_records_total{outcome=
    "dropped_queue_full"}` is an operator responsibility.
14. **(shipped) The batched writer is sequenced by `main`, not tracked by `graceful`.**
    `tracing.Shutdown` is called explicitly after the HTTP server stops and before the
    database closes. The ordering is correct today, but a future change to `main`'s shutdown
    sequence could close the database before the drain without any compiler or test
    complaint. A test that pins the ordering would be cheap.
15. **(measured) Chunked retention increases total degraded requests on SQLite** (4 → 65
    per sweep in the fixture) even though it bounds the longest statement 22× and lowers
    per-request risk, because SQLite serializes writers. SQLite is the small-deployment
    default where sweeps are tiny, so this is a documented caveat rather than a regression,
    and the pause is one variable away (Appendix D.3).
16. **(units) `logs.created_at` is seconds; `traces.created_at` is milliseconds.** Every
    Phase 2 query, rollup bucket, and retention cutoff has to use the right unit for its
    table; the existing code does, but the asymmetry is a standing hazard.
17. **(design) Sampling order.** A rate of exactly 1.0 short-circuits before the
    always-keep rules, and a rate of 0 drops everything not caught by an always-keep rule.
    Both are intended; they are stated so that "0.05 with errors always kept" is read
    correctly.

---

## 13. Recommended Sequencing

| Phase | Content | Ships independently | Removes |
| --- | --- | --- | --- |
| 0 | path filter, chunked sweeps, log retention/size/disk guard, log sampling and compact line, dashboard cache + guard rails | **done 2026-09-05** | disk exhaustion, sweep stalls |
| 1 | in-memory recorder, `TraceSink`, batched writer, OTLP sink, sampling, per-timestamp columns | **done 2026-09-05** | ~92% of write statements |
| 2 | rollup tables, leader-gated aggregator, real-time union read, keyset pagination, index diet, `logs` retention | yes | dashboard timeouts |
| 3 | OTLP app logs, dashboard-grade metrics, collector reference config | yes | SQL telemetry writes entirely |
| 4 | `UsageStore` interface, ClickHouse mirror, reconciliation | yes | long-horizon analytics from the OLTP database |

Phases 0 and 1 are the response to "the system falls over." Phase 2 is the response to "the
dashboard times out." Phases 3 and 4 are the response to "we are a heavy user and want our
telemetry in our own OLAP stack."

---

## 14. References

Industry practice consulted for this design (2026):

- LiteLLM — [Database Sizing](https://docs.litellm.ai/docs/proxy/db_sizing) and
  [Production Best Practices](https://docs.litellm.ai/docs/proxy/prod): the write path
  determines database sizing, not the read path; batch spend writes; above ~1000 RPS route
  accounting writes through a Redis transaction buffer; budget 1–2 KB per logged request;
  set a retention policy or disable per-request rows.
- Langfuse — [Langfuse and ClickHouse](https://clickhouse.com/blog/langfuse-and-clickhouse-a-new-data-stack-for-modern-llm-applications)
  and [scaling for the agentic era](https://clickhouse.com/blog/langfuse-llm-analytics):
  outgrew a single Postgres at billions of rows; moved events to ClickHouse with Redis
  queueing and S3 for large payloads while keeping transactional entities relational;
  1000+ self-hosted deployments run the split.
- Respan — [scaling LLM observability with ClickHouse Cloud](https://clickhouse.com/blog/respan-ai-llm-observability):
  outgrew Postgres at 50–100 writes/second; ~50M daily events; query latency from minutes
  to near real-time after moving analytics off the OLTP store.
- Helicone — [self-host architecture](https://docs.helicone.ai/getting-started/self-host/manual):
  Postgres for OLTP, ClickHouse for analytics, object storage for payloads, Kafka to
  decouple ingestion — the same three-plane split proposed here.
- TrueFoundry — [Logging Architecture for AI Gateway](https://www.truefoundry.com/blog/logging-architecture-ai-gateway):
  the dissenting design — OTel ingestion into customer-owned S3 with Delta Lake and
  DataFusion, explicitly rejecting ClickHouse on operational-burden grounds. Cited as the
  argument for keeping the OTLP path first-class and the OLAP store optional.
- ClickHouse — [Asynchronous inserts](https://clickhouse.com/docs/optimize/asynchronous-inserts)
  and [LLM observability](https://clickhouse.com/resources/engineering/llm-observability):
  do not stack `async_insert` on top of client-side batching; prefer
  `wait_for_async_insert=1` when async inserts are used.
- OpenTelemetry — [Sampling](https://opentelemetry.io/docs/concepts/sampling/),
  [tail-based sampling with the Collector](https://www.controltheory.com/resources/tail-sampling-with-the-otel-collector/),
  and [Datadog's trace-volume guide](https://www.datadoghq.com/blog/control-trace-volume-with-opentelemetry-tail-based-sampling/):
  head sampling to cut bulk volume, tail policies to retain errors and slow traces;
  `decision_wait` at 2–3× p99; two-tier gateway with a load-balancing exporter.
- OpenTelemetry Go — [Logs API and SDK reach release candidate](https://opentelemetry.io/blog/2026/go-logs-api-sdk-rc/),
  [`otelzap` bridge](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelzap),
  [`otlploghttp`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp).
- Timescale — [continuous aggregates](https://www.tigerdata.com/docs/learn/continuous-aggregates):
  the real-time materialized-view semantics reproduced portably in W2.2/W2.3 (pre-computed
  history + live aggregate above the watermark), without taking a TimescaleDB dependency.
- PostgreSQL time-series practice —
  [BRIN indexes](https://www.crunchydata.com/blog/postgresql-brin-indexes-big-data-performance-with-minimal-storage),
  [time-based partitioning](https://oneuptime.com/blog/post/2026-01-26-time-based-partitioning-postgresql/view):
  keep range predicates on the raw column (never wrap it in a function), keep
  per-partition indexes minimal, partition above ~100 GB. Directly motivates W2.3 and W2.5.
  Native partitioning of `traces`/`logs` is deliberately left out of scope here because it
  cannot be expressed portably across SQLite, MySQL, and PostgreSQL; it belongs in a
  PostgreSQL-specific follow-up.

---

# Part III — Appendices

## Appendix A. Schema Reference

### A.1 `traces`

As migrated by the measured tree (columns after `updated_at` are the Phase 1 additions):

```sql
CREATE TABLE traces (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid                        CHAR(36),                 -- external identifier; UNIQUE via a separate migration
    trace_id                    VARCHAR(64) NOT NULL,     -- UNIQUE INDEX idx_traces_trace_id
    url                         TEXT NOT NULL,            -- sanitized, capped at 4096 bytes
    method                      VARCHAR(16) NOT NULL,
    body_size                   BIGINT DEFAULT 0,
    status                      INTEGER DEFAULT 0,        -- HTTP status; 0 coerced to 200 at request end
    timestamps                  TEXT,                     -- JSON document, see below
    created_at                  BIGINT,                   -- unix MILLISECONDS; INDEX idx_traces_created_at
    updated_at                  BIGINT,                   -- unix milliseconds
    ts_request_received         BIGINT NULL,              -- Phase 1: projection of timestamps.request_received
    ts_request_forwarded        BIGINT NULL,
    ts_first_upstream_response  BIGINT NULL,
    ts_first_client_response    BIGINT NULL,
    ts_upstream_completed       BIGINT NULL,
    ts_request_completed        BIGINT NULL
);
```

There is no index on any `ts_*` column. The `timestamps` document
(`model/trace.go:52-73`):

```go
type TraceTimestamps struct {
    RequestReceived       *int64              `json:"request_received,omitempty"`
    RequestForwarded      *int64              `json:"request_forwarded,omitempty"`
    FirstUpstreamResponse *int64              `json:"first_upstream_response,omitempty"`
    FirstClientResponse   *int64              `json:"first_client_response,omitempty"`
    UpstreamCompleted     *int64              `json:"upstream_completed,omitempty"`
    RequestCompleted      *int64              `json:"request_completed,omitempty"`
    ExternalCalls         []TraceExternalCall `json:"external_calls,omitempty"`
}

type TraceExternalCall struct {
    Key         string `json:"key,omitempty"`
    Source      string `json:"source,omitempty"`      // "mcp" today; defaults to "external"
    Tool        string `json:"tool,omitempty"`
    ServerID    int    `json:"server_id,omitempty"`
    ServerLabel string `json:"server_label,omitempty"`
    StartedAt   int64  `json:"started_at,omitempty"`
    EndedAt     int64  `json:"ended_at,omitempty"`
    DurationMs  int64  `json:"duration_ms,omitempty"`
    IsError     bool   `json:"is_error,omitempty"`
}
```

Read path: `Trace.GetTraceTimestamps()` unmarshals the document and then overwrites each
field with the corresponding `ts_*` column when that column is non-null; `external_calls`
lives only in the document.

### A.2 `logs`

Columns (`model/log.go:26-59`):

| Column | Type | Notes |
| --- | --- | --- |
| `id` | INT PK | |
| `uuid` | CHAR(36) | external identifier; UNIQUE via a separate migration |
| `user_id` | INT | indexed; also leads `idx_user_token` |
| `user_uuid` | CHAR(36) NULL | indexed |
| `created_at` | BIGINT | unix **SECONDS**; leads `idx_created_at_type` |
| `type` | INT | 1 top-up, 2 consume, 3 manage, 4 system, 5 test, 6 provisional, 7 tool; second column of `idx_created_at_type` |
| `content` | TEXT | rendered human-readable description |
| `username` | VARCHAR | second column of `index_username_model_name` |
| `token_name` | VARCHAR | indexed; second column of `idx_user_token` |
| `token_uuid` | CHAR(36) NULL | indexed |
| `model_name` | VARCHAR | indexed; leads `index_username_model_name` |
| `origin_model_name` | VARCHAR | indexed |
| `quota` | INT | indexed (sorting only) |
| `prompt_tokens` | INT | indexed (sorting only) |
| `completion_tokens` | INT | indexed (sorting only) |
| `channel_id` | INT | indexed |
| `channel_uuid` | CHAR(36) NULL | indexed |
| `request_id` | VARCHAR | not indexed |
| `trace_id` | VARCHAR(64) | indexed |
| `updated_at` | BIGINT | unix milliseconds |
| `elapsed_time` | BIGINT | milliseconds; indexed (sorting only) |
| `is_stream` | BOOL | |
| `system_prompt_reset` | BOOL | |
| `cached_prompt_tokens` | INT | indexed (sorting only) |
| `metadata` | TEXT | JSON |

Secondary indexes: 14 single-column + 3 composite declared on the struct, plus the
migration-created unique index on `uuid` = 18 in a migrated database. Every user-facing
query adds `type != 6`.

### A.3 Dashboard aggregate row shapes (`dto/log_statistics.go`)

`LogStatistic`: `day`, `model_name`, `request_count`, `quota`, `prompt_tokens`,
`completion_tokens`, `cached_prompt_tokens`, `cache_hit_count`, `cache_hit_quota`.
`LogStatisticByUser` replaces `model_name` with `username`, `user_uuid`.
`LogStatisticByToken` adds `token_name`. The three `ToolLogStatistic*` shapes carry only
`day`, the dimension (`tool_name` / `username`+`user_uuid` / `+token_name`),
`request_count`, and `quota`. Internal integer user ids are never serialized; only UUIDs
cross the API boundary.

---

## Appendix B. Implemented Interfaces, Quoted

### B.1 `TraceSink` (`common/tracing/sink.go:23-37`)

```go
// TraceSink accepts completed request traces for durable or external recording.
//
// Implementations must be safe for concurrent use and must never block the
// calling request goroutine.
type TraceSink interface {
	// Submit hands a completed trace to the sink. It returns an error only for
	// programming faults; transport and capacity failures are counted as
	// dropped records and reported through metrics, never propagated to the
	// request path.
	Submit(ctx context.Context, row *model.Trace) error
	// Flush blocks until buffered records are durably handed off or ctx expires.
	Flush(ctx context.Context) error
	// Close flushes and releases resources. It is idempotent.
	Close(ctx context.Context) error
}
```

Sinks take the GORM row (`*model.Trace`) built by `model.NewTraceRow`, not a bespoke
record type. Process-wide access is `tracing.Sink()` (never nil; a drop-everything sink
before `InitSinks`), `tracing.Flush(ctx)`, and `tracing.Shutdown(ctx)`.

### B.2 Sampling decision (`common/tracing/sampling.go:40-57`)

```go
func SampleDecision(status int, durationMs int64, forced bool) bool {
	if config.TraceSampleRate >= 1 {
		return true
	}
	if forced {
		return true
	}
	if config.TraceAlwaysSampleErrors && status >= 400 {
		return true
	}
	if config.TraceAlwaysSampleSlowMs > 0 && durationMs >= int64(config.TraceAlwaysSampleSlowMs) {
		return true
	}
	if config.TraceSampleRate <= 0 {
		return false
	}
	return randomUnitFloat() < config.TraceSampleRate
}
```

`randomUnitFloat` is `math/rand/v2`'s `Float64`, replaceable by tests through
`SetSampleDeciderForTest`.

### B.3 Trace-pipeline metrics (`common/metrics/trace_pipeline.go`)

```go
// TracePipelineRecorder is the OPTIONAL extension a recorder may implement to
// receive trace-pipeline accounting.
//
// It is deliberately not part of MetricsRecorder: adding methods to that
// interface would stop any out-of-tree implementation from compiling, which the
// project's backward-compatibility rules forbid. Recorders that do not
// implement this interface are skipped.
type TracePipelineRecorder interface {
	// RecordTraceRecord tallies count trace records with the given outcome.
	RecordTraceRecord(outcome string, count int)
	// UpdateTraceQueueDepth publishes writer-queue occupancy and capacity.
	UpdateTraceQueueDepth(depth, capacity float64)
}
```

The only permitted `outcome` values, all compile-time constants:

| Constant | Value | Emitted when |
| --- | --- | --- |
| `TraceOutcomeSampledOut` | `sampled_out` | the sampler discards a trace, before any sink |
| `TraceOutcomeQueued` | `queued` | `sqlSink` accepts a record |
| `TraceOutcomeDroppedQueueFull` | `dropped_queue_full` | `sqlSink` queue is full; the submitted record is discarded |
| `TraceOutcomeDroppedClosed` | `dropped_closed` | a record arrives after `sqlSink` began shutting down |
| `TraceOutcomeWritten` | `written` | rows durably inserted (count = rows) |
| `TraceOutcomeWriteFailed` | `write_failed` | rows a batch failed to insert |
| `TraceOutcomeExported` | `exported` | `otlpSink` handed a span to the tracer (whether or not a real provider is installed) |

Instrument names, identical in the Prometheus and OpenTelemetry recorders:

| Name | Type | Labels |
| --- | --- | --- |
| `oneapi_trace_records_total` | counter | `outcome` |
| `oneapi_trace_queue_depth` | gauge | none |
| `oneapi_trace_queue_capacity` | gauge | none |

A queue depth that tracks the capacity means the database cannot keep pace with trace
ingestion.

### B.4 Chunked retention (`model/retention_chunk.go`)

```go
// ChunkedDeleteOptions describes one bounded retention sweep.
type ChunkedDeleteOptions struct {
	// Table is the physical table name, used to build the dialect-specific
	// bounded DELETE. It must be a compile-time constant from this package,
	// never a caller-supplied value: it is interpolated into SQL.
	Table string
	// Where is the row-selection predicate, with `?` placeholders.
	Where string
	// Args are the predicate arguments.
	Args []any
	// BatchSize bounds rows removed per statement; <= 0 uses the configured
	// default.
	BatchSize int
	// Pause is how long to wait between chunks; <= 0 disables the pause.
	Pause time.Duration
}

func ChunkedDelete(ctx context.Context, db *gorm.DB, opts ChunkedDeleteOptions) (int64, error)
```

Allowed tables: `traces`, `logs`, `async_task_bindings`. The loop checks the context
before each chunk, executes one bounded statement, stops when a chunk removes fewer rows
than the batch size, and otherwise sleeps `Pause`. A cancelled sweep returns the rows it
removed **and** a wrapped `ctx.Err()`, so a background sweeper can treat cancellation as
shutdown while an operator-triggered purge never reports success for a partial delete. The
bounded statement per engine, dialect read from the handle:

```go
switch dialect {
case "postgres":
	return "DELETE FROM " + table +
		" WHERE ctid IN (SELECT ctid FROM " + table +
		" WHERE " + where + " LIMIT " + limit + ")"
case "mysql":
	return "DELETE FROM " + table + " WHERE " + where + " LIMIT " + limit
default: // sqlite
	return "DELETE FROM " + table +
		" WHERE rowid IN (SELECT rowid FROM " + table +
		" WHERE " + where + " LIMIT " + limit + ")"
}
```

Callers: `CleanExpiredTracesContext` (primary handle, predicate
`created_at < <now − TRACE_RETENTION_DAYS, in ms>`), `DeleteOldLogContext` (`LOG_DB`
handle, `created_at < <cutoff, in s>`), `CleanExpiredAsyncTaskBindingsContext` (primary
handle, a `CASE` predicate on last-accessed-or-created time that no index serves).

### B.5 Trace lifecycle helpers (`common/tracing/tracing.go`)

| Function | Batched mode | Sync mode (`TRACE_WRITE_MODE=sync`) |
| --- | --- | --- |
| `RecordTraceStart(c)` | allocate a `Recorder`, bind it to the gin context, annotate the OpenTelemetry span | `model.CreateTrace` (one INSERT); no recorder is bound |
| `RecordTraceTimestamp(c, key)` | `Recorder.Mark(key)` + span event | `model.UpdateTraceTimestamp` (SELECT + UPDATE) |
| `RecordTraceExternalCall(c, call)` | `Recorder.AppendExternalCall` | `model.AppendTraceExternalCall` (SELECT + UPDATE) |
| `RecordTraceStatus(c, status)` | `Recorder.SetStatus` + span attribute | `model.UpdateTraceStatus` (UPDATE) |
| `ForceTraceSample(c)` | `Recorder.ForceSample` | no-op (no recorder) |
| `RecordTraceEnd(c)` | mark `request_completed` and status; `Finish()`; `SampleDecision`; `NewTraceRow`; `Sink().Submit` with a context that survives client cancellation | mark `request_completed` and status via the legacy UPDATEs; returns before sampling |

Every helper first checks `requestExcluded(c)`: true when `TRACE_SINK` is exactly `none`,
when the context carries no request, or when the path matches an excluded prefix.

---

## Appendix C. Glossary

| Term | Meaning in this document |
| --- | --- |
| **Ability** | Denormalized routing row `(group, model, channel)`; the filter for channel selection. |
| **Adaptor** | Per-provider code under `relay/adaptor/` that converts requests and responses to and from an upstream's wire format. |
| **Application log** | The process's structured text log lines (files and/or stdout). Not the `logs` table. |
| **`AutoMigrate`** | GORM's schema migration; additive only; run by master nodes at startup. |
| **Batch updater** | Existing opt-in (`BATCH_UPDATE_ENABLED`) in-memory accumulator for quota counters, flushed every few seconds and on shutdown. The pattern the batched trace writer copies. |
| **Bucket** | A rollup row's time window (hour or day), identified by its UTC-aligned start. |
| **Channel** | One configured upstream provider account/endpoint. |
| **Compact-UUID coordinator** | Existing database-level advisory-lock election used by the compact UUID storage migration so several master nodes do not run the same expensive work. Proposed for Phase 2's aggregator. |
| **Consume row** | A `logs` row of type 2: the billing record of one request. |
| **Distribute** | The middleware that selects a channel for a request. |
| **`glog`** | The logger wrapper from `github.com/Laisky/go-utils` around the zap fork. |
| **`gmw`** | `github.com/Laisky/gin-middlewares`: request-scoped logger, trace id, and `gmw.GetLogger(c)`. |
| **Grain** | Rollup resolution: `hour` or `day`. |
| **Group** | Pricing/routing tier name on users and channels. |
| **`LOG_DB`** | The GORM handle for the `logs` table; equals the primary handle unless `LOG_SQL_DSN` is set. |
| **Master / slave node** | `NODE_TYPE`; master nodes run migrations; all nodes serve all routes and run all sweepers. |
| **Option** | Admin-editable runtime setting stored in the `options` table and re-read every `SYNC_FREQUENCY` seconds. |
| **OTLP** | The OpenTelemetry wire protocol; this project exports over HTTP with gzip. |
| **Plane A / B / C** | Account state / usage ledger / peripheral telemetry (§2). |
| **Profile** | A named set of configuration defaults selected by `OBSERVABILITY_PROFILE`. |
| **Provisional row** | A `logs` row of type 6 written at pre-consume and later reconciled into a consume row. |
| **Quota** | The internal billing unit; balances on users and tokens, usage counters on channels. |
| **Recorder** | The per-request in-memory trace accumulator (Phase 1). |
| **Relay** | The gateway's proxy path from an incoming model request to an upstream provider and back. |
| **Rollup** | A pre-aggregated per-bucket table that replaces scanning raw `logs` rows for the dashboard (Phase 2). |
| **Sink** | A destination for completed traces: SQL, OTLP, or none. |
| **Site-wide** | A root user's dashboard over all users (`targetUserId = 0`). |
| **Statement** | One SQL statement sent to the database. |
| **Token** | A user-issued API key (`sk-…`) with its own quota. |
| **Trace** | The per-request latency record in the `traces` table (or an exported span). |
| **Trace id** | A per-request identifier minted by the logger middleware; span-scoped, so distinct even when several requests share one distributed OpenTelemetry trace. |
| **Usage log** | A row in the `logs` table. |
| **WAL** | Write-ahead log; used here as the measure of durable work a database performed. |
| **Watermark** | The newest bucket the rollup aggregator has fully processed. |

---

## Appendix D. Summary of the Measured Results

The companion document contains the full methodology, the raw tables, and fourteen
"honest negatives". This appendix summarizes what a reviewer needs in order to read §8 and
§10. Machine: AMD Ryzen 7 5700G (16 threads), 27 GB RAM, Linux 6.8, Go 1.27.1; PostgreSQL
17 and MySQL 8.4 in Docker on loopback; SQLite file-backed with WAL. Sink configuration in
every arm is the production default. The "after" arms correspond to the `scaled` profile.

### D.1 Trace write pipeline (`BenchmarkTracePipeline`, 5,000 requests × 6 runs, 16-way parallel)

Five controls were added because without them each measurement produces a flattering
number that is not true: a **floor arm** with no tracing (so costs are quoted as absolute
microseconds attributable to tracing, not as ratios); **work-conservation metrics** (WAL
bytes per trace from `pg_current_wal_lsn()`, so that relocating work to another core cannot
be mistaken for removing it); the **production connection-pool policy** (the default pool
starved the twelve-statement baseline and manufactured much of the apparent win);
an **arrival-rate curve** (D.2); and **verified row counts**.

| Engine | statements/req, sync | statements/req, batched | trace-attributable µs/req, sync | batched | saved |
| --- | --- | --- | --- | --- | --- |
| SQLite | 12.00 ± 0% | 0.0022 | 1,760 ± 5% | 20.7 (± ~41%) | 1,740 |
| PostgreSQL | 12.00 ± 0% | 0.0022 | 4,080 ± 5% | 21.4 (± ~89%) | 4,058 |
| MySQL | 12.00 ± 0% | 0.0022 | 6,209 ± 29% | 30.7 (interval includes 0) | 6,178 |

The request goroutine issues exactly zero statements under batched mode, proven
deterministically by `TestTracingMiddlewareIssuesNoStatementsOnRequestPath`; the residual
0.0022 is the writer's INSERT amortized over ~500 traces. The batched path's own cost is
below the harness's resolution (14–18 µs floor); only the saving is robust. Under
`b.RunParallel` the µs figures are wall clock per request, i.e. reciprocal throughput, not
per-request latency.

Work conservation on PostgreSQL:

| Metric | sync | batched | batched @ 5% sampling |
| --- | --- | --- | --- |
| WAL bytes per request | 2,602 ± 4% | 746 ± 10% | 36 ± 5% |
| WAL bytes per **persisted** trace | 2,602 | 746 | ~708 |
| row versions updated per trace | 6 | 0 | 0 |
| transactions per trace | 12 | ~0.002 | ~0.0006 |

The rewrite cuts durable work 3.5× per trace by eliminating six whole-row rewrites of a
TEXT column. Sampling cuts total work a further ~20× by storing ~20× fewer traces, **not**
by making a trace cheaper.

### D.2 Statements per trace is a curve (`BenchmarkTraceArrivalRate`, PostgreSQL)

| Achieved rate | traces per INSERT | statements per trace |
| --- | --- | --- |
| ~50 /s | 21 | 0.053 |
| 355–491 /s | 152–246 | 0.005–0.008 |
| ~1,870 /s (driver saturates) | ~431 | ~0.0025 |

At low load the 1-second flush ticker, not the 500-row batch, decides when a write happens.
Quoting only the saturated figure would overstate the result ~21× at realistic load; even
the worst point is 225× fewer statements than the baseline's 12.

### D.3 Chunked retention (`BenchmarkRetentionSweep`, 200k expired + 5k fresh rows, 400 req/s competing load)

| Engine | Variant | longest DELETE | degraded requests (≥ 25 ms) | delete rows/s |
| --- | --- | --- | --- | --- |
| PostgreSQL | unbounded | 134 ms | 11.76% (4) | 1,489k |
| PostgreSQL | chunked, pause 10 ms | 22 ms | 0.00% (0) | 194k |
| MySQL | unbounded | 2,890 ms | 0.86% (9) | 69k |
| MySQL | chunked, pause 10 ms | 383 ms | 0.28% (7) | 32k |
| SQLite | unbounded | 1,495 ms | 26.67% (4) | 134k |
| SQLite | chunked, pause 10 ms | 68 ms | 18.24% (65) | 111k |

The longest statement is bounded 6–22× on every engine and per-request risk falls on every
engine; on SQLite the **total** number of degraded requests rises because SQLite serializes
writers and the sweep now spans a longer window. Throughput falls 2–8× versus the unbounded
DELETE, which is the price of bounded statements. A sweep over pauses of 0/10/25/50/100/200
ms set the default at 10 ms: PostgreSQL reaches 0% degradation there, it is MySQL's optimum
on all three measures, and throughput is 1.6–4.5× higher than at the earlier 100 ms default.
Delete throughput here is an upper bound measured on a fully cached table. Two earlier
versions of this measurement used a closed-loop load generator and reached wrong
conclusions; both are superseded.

### D.4 Application log line (`BenchmarkRecordLogLineBytes`, cross-tree against `397781e1`)

Console encoding (production): the `record log` line shrinks from 516 / 576 / 751 bytes
(short / typical / long `content`) to a flat 413 bytes, i.e. 103–338 bytes (20–45%) per
billed request. At 10k req/s with typical content that is ~141 GB/day **with sampling off**;
under the `scaled`/`external` profiles sampling already thins this line to ~199/s, and the
compact form is then worth ~2.8 GB/day. The two mechanisms are not additive. An earlier
code comment claimed "roughly 1 TB/day" for this one line; that conflated it with the whole
process's volume and was corrected.

### D.5 Dashboard site-wide aggregate (`BenchmarkSiteWideQuotaStats*`, PostgreSQL)

| Users | uncached, per load | cached hit |
| --- | --- | --- |
| 10,000 | 3.05 ms ± 17% | 66 ns, 0 allocations |
| 100,000 | 25.1 ms ± 4% | 66 ns |
| 1,000,000 | 114 ms ± 18% | 66 ns |

The cost is **amortized, not removed**: a single cold viewer still pays the full scan once
per TTL. With 32 concurrent cold viewers the cached path stays flat at one query's cost
(~26 ms) while the uncached path grows to 121 ms — the single-flight signature of the mutex
held across the query — a 4.7× ratio (an earlier 54× figure was measured on a table full of
dead tuples and is withdrawn).

### D.6 Claims supported, claims narrowed

| Proposal claim | Measured |
| --- | --- |
| ~12 statements per request → ~0 on the request path | 12.00 → 0 on the request goroutine; 0.0006–0.0016 total before flush |
| One batched INSERT per `TRACE_BATCH_SIZE` traces | ~431 per INSERT at ~1.9k req/s; 21 at 50 req/s |
| ~3 µs of in-memory work per request | true in isolation; end to end 21–31 µs, inseparable from the harness floor |
| Retention sweeps stop holding one giant transaction | longest DELETE bounded 6–22×; total sweep 2–8× slower; SQLite total harm rises |
| Site-wide aggregate removed from the hot path | amortized to once per TTL; a lone cold viewer still pays it |
| Batched writer loses no traces | true with a 20k queue and 5k submissions; saturation not exercised |
