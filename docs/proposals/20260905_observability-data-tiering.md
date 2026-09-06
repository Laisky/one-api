# Observability Data Tiering for High-Volume Deployments

- Revision: 2026-09-06, implementation plan revised against the expert panel review.
- Decision: retain the three data planes and zero-dependency default; release verified,
  opt-in trace improvements first. Phases 2–4 require the gates below.
- Target: approximately one million registered users and sustained 10,000 relay RPS.
  This is a validation target, **not demonstrated capacity**.
- Owners: Backend implements capture and APIs; Database Operations owns migrations and
  history retention; SRE owns load evidence and rollout; Frontend owns Modern integration.
- Baseline: `397781e1443063772817f0114c065fa1a373ff3d`.
- Earlier benchmark snapshot: `11663356cc7953ddb99b0008393238e8d00d3f37`, a Git stash
  snapshot, not a published implementation commit.
- Repository HEAD inspected for this revision: `30dff1971238f2750986c0e4e3b3a9d81343754d`,
  with existing staged implementation changes. File references describe that working tree.
- Historical evidence: [Phase 0/1 benchmark record](../benchmarks/20260905_observability-phase0-phase1.md).
  This revision does not claim to rerun those benchmarks.

## 0. Decisions, status, and delivery boundaries

This document replaces the earlier execution plan, including its contradictory impact
estimates. Sections 2 and 3 are the authoritative compatibility and configuration contract;
work items must comply with them. Requirements marked **planned** are not current behavior.

| Area | Implemented in inspected tree | Test / benchmark evidence | Release status |
| --- | --- | --- | --- |
| Phase 0: exclusions, chunked deletion, file guards, sampling, dashboard caches | Yes | Targeted tests and historical component benchmarks exist | Close compatibility, disk, cache-failure, and resource gates in W0 |
| Phase 1: recorder, SQL batching, local sampling, OTLP sink, timestamp columns | Yes | Targeted tests and historical component benchmarks exist | Close lifecycle, outcome, memory, and OTLP correctness gates in W1 |
| Phase 2: mutation-aware SQL projections, additive pagination, explicit index migrations | No | Acceptance specification below | Planned; synchronous usage-write semantics remain |
| Phase 3: optional OTLP application logs and operational metrics | No; trace sink is already Phase 1 code | No integration acceptance yet | Optional; does not block SQL optimization |
| Phase 4: durable replay and corrected ClickHouse mirror | No | No replay or reconciliation evidence yet | Optional; cannot authorize billing-history deletion |
| Sustained full-relay 10,000 RPS | Not established | Component results are insufficient | Require G5 before publishing capacity claims |

“Implemented,” “unit-tested,” “benchmarked,” and “validated at target load” are separate
statuses. The external review could not find the companion evidence on its inspected
baseline branch. Both Git objects and the companion file are available locally now, which
improves traceability but does not independently reproduce the measurements.

Every release record must attach its published implementation commit, any additional diff,
build/image digest, benchmark harness revision, exact commands, effective configuration,
engine versions, raw output, and pass/fail summary. Archive the snapshot as a retrievable
artifact; a local stash object alone is not a durable review record. Redact credentials,
DSNs, tokens, request content, and customer identifiers from evidence.

Delivery order is deliberately limited:

1. Finish W0/W1 and release their opt-in telemetry improvements under G1.
2. Ship additive cursor/count APIs, then shadow SQL projections, then opt-in read cutover
   under G2. Do not introduce asynchronous billing writes.
3. Add optional OTLP application logs and operational views under G3.
4. Add a durable, correction-aware analytics mirror under G4 if measurements justify it.
5. Certify a specific deployment topology and retention policy under G5.

## 1. System context and the actual bottlenecks

one-api authenticates gateway-issued API tokens, routes requests through provider channels,
converts between ChatCompletion, Responses, and Claude Messages formats, forwards streaming
or non-streaming responses, charges quota, and records usage. All three API formats and
all adapters must retain their billing and tracing behavior through this work.

| Plane | Data and authority | Performance implications |
| --- | --- | --- |
| A: account state | Users, tokens, channels, quota and payment state in primary SQL | Quota mutations scale with requests; hot users, shared tokens and popular channels can contend |
| B: usage ledger | Mutable detailed `logs` rows in `LOG_DB`; SQL remains authoritative | Inserts, later updates, indexes, reconciliation and audit retention remain real work |
| C: peripheral telemetry | SQL traces, application log files/stdout, metrics and optional OTLP | Best-effort data can be sampled, buffered or dropped with explicit accounting |

`LOG_DB` equals the primary handle unless `LOG_SQL_DSN` selects another database. Traces
use the primary handle. New usage queries, capture transactions, ownership locks and
migrations must use **the usage-owning handle and its dialect**. Existing
`dayAggregationSelect()` chooses from process-global engine flags despite querying
`LOG_DB`; W2 must fix this for heterogeneous primary/log database deployments.

`logs.created_at` uses Unix seconds; trace creation/lifecycle timestamps and usage
`updated_at` use milliseconds. Normalize units explicitly and calculate UTC day boundaries
independently of database session time zones.

The baseline trace lifecycle performs approximately twelve synchronous SQL statements per
representative relay: an insert, timestamp read/modify/write cycles, and a status update.
The exact count depends on the request path. Batching removes these from the request
goroutine in the optimized path; it does not remove billing inserts or account updates.

Usage rows are mutable:

- `RecordProvisionalConsumeLog` synchronously inserts type 6 and returns its database ID.
- `ReconcileConsumeLogDetailed` updates that row into type 2, changing quota, tokens,
  cache values, elapsed time, content and metadata without advancing `created_at`.
- `UpdateConsumeLogByID` permits later corrections.
- `RecordToolLogs` writes separate type 7 invocation rows, which can share request/trace
  correlators with each other and with the main consume row.
- Retention, administrative deletion and identity repair can also change projected data.

Therefore a closed hour is not a finalized billing period, and a trace ID is not a unique
usage-event key. Current helpers may report a usage insertion failure after billing has
already completed. Neither the baseline nor this proposal promises an atomic transaction
across account state and a separate `LOG_DB`.

`GET /api/user/dashboard` returns six aggregation result sets, not six separate HTTP
endpoints. Today those queries scan `logs` and group by UTC day and selected dimensions.
Their raw `created_at` predicates can use range access; a date expression in the grouping
does not categorically prevent index access or pruning. Obtain execution plans to explain
actual sorting, grouping, heap fetches and scan cost.

Legacy log lists use offsets and exact counts, including filters that rollups cannot
represent. Cached dashboard queries only amortize these costs. Cache expiry and Redis
failure can restore raw work in the current implementation.

Application logs are separate from `logs` billing rows. File rotation is currently by
UTC time, and the disk guard excludes the open file. Those features do not impose a hard
bound on active-file size or protect against an unlimited warning/error storm.

## 2. Authoritative compatibility and durability contract

### 2.1 Unchanged configuration

A deployment upgraded without changing configuration retains:

- SQLite-only operation, with no required Redis, collector, ClickHouse or external queue.
- Synchronous trace writes, live visibility of in-flight traces, full coverage and existing
  trace response fields.
- Full application log-line fields, existing sink selection and disabled file deletion by
  default. `LOG_RETENTION_DAYS` controls application files, not billing history.
- Dashboard freshness and permitted ranges, legacy pagination, filters, sorts and exact
  `total` semantics. Normal-user range restrictions remain in force.
- Existing billing retention behavior: no new automatic deletion of authoritative usage.

Additive timestamp columns remain nullable. Writers retain the **complete timestamp JSON
document**, including external calls, as well as available columns. Readers overlay
non-null columns on the document. Old binaries must read new rows; upgraded writers must
work against an unmigrated schema. Dropping the document requires a separate migration.

`DeleteOldLog(targetTimestamp)` keeps its signature; `DeleteOldLogContext` is additive.
`MetricsRecorder` stays source-compatible. New instrumentation uses optional extension
interfaces, following `TracePipelineRecorder`, including W3 operational metrics.

Changes to sweeper cadence or deletion timing must be listed and compatibility-tested.
The current shared 60-minute interval differs from historical 24-hour scheduling; strict
unchanged-configuration compatibility requires restoring the historical default where
applicable or obtaining a separately explicit cadence opt-in before G1.

### 2.2 Explicit optimization choices

Profiles select defaults; individual environment values override them. Selecting `scaled`
consents to documented trace sampling, delayed trace visibility, application-log sampling
and dashboard cache/range changes. It does **not** authorize lossy billing writes, dropping
indexes, switching legacy APIs to approximate counts, or shortening billing history.

A completed trace may be sampled out, rejected by a full queue, fail persistence, or be
lost on process crash. “Always keep errors” is a selection rule, not guaranteed delivery.
Sampling and sink selection do not affect usage or quota records.
Forced local sampling is a server-side hook only; no client debug header can force capture.

The initial performance release keeps synchronous usage persistence and the provisional-ID
contract. W2 adds transactional invalidation to these writes, not a memory-buffered usage
writer. If invalidation cannot commit, its usage mutation must not commit independently;
return or report the error through the existing caller contract and alert on the failure.
This improves projection capture atomicity, not account/ledger atomicity.

### 2.3 Gate for any future usage batching

Usage batching is outside Phases 0–4. Remove any assumption of approximately 50 usage
statements/second. A future proposal must define all of the following before implementation:

| Boundary | Required contract |
| --- | --- |
| Acknowledgment | Success means a committed SQL mutation or committed durable outbox record; memory acceptance is not durable acknowledgment |
| Identity and ordering | Preserve provisional database IDs and serialize reconciliation after insert commit; define stable idempotency keys and revisions |
| Retry | Resolve ambiguous commits and deduplicate retries; keep retry state durably until acknowledged |
| Saturation / outage | Apply bounded backpressure or explicitly fail; never apply TraceSink's drop-and-count semantics to usage |
| Crash / shutdown | State the recovery procedure and what committed work survives process death or a shutdown deadline |
| Lossy alternative | Requires separate explicit opt-in with limits in records, bytes, age and affected quota; expose actual loss and stop admission at the agreed bound |

A flush interval does not bound outage loss: pending data may span the whole outage.
`BATCH_UPDATE_ENABLED` remains independent and defaults to false. Its current in-memory
quota updater detaches maps before writes, documents crash loss, and does not requeue all
failed/cancelled work. A clean worker exit is not persistence proof. No arbitrary RPS
threshold makes that option mandatory; measure it as a distinct consistency tradeoff.

## 3. Configuration: current behavior and release requirements

### 3.1 Implemented settings

All entries in this table exist in the inspected tree. Values are current defaults, not
certified capacity settings. File-size settings named `_MB` use MiB (`1 << 20`) internally.

| Setting | standalone | scaled | external |
| --- | --- | --- | --- |
| `OBSERVABILITY_PROFILE` | `standalone` when unset | explicit selection | explicit selection |
| `TRACE_SINK` | `db` | `db` | `otlp` |
| `TRACE_WRITE_MODE` | `sync` | `batched` | `batched` |
| `TRACE_SAMPLE_RATE` | `1` | `0.05` | `1` |
| `TRACE_ALWAYS_SAMPLE_ERRORS` | `true` | `true` | `true` |
| `TRACE_ALWAYS_SAMPLE_SLOW_MS` | `0` | `5000` | `0` |
| `TRACE_BATCH_SIZE` | `500` | `500` | `500` |
| `TRACE_FLUSH_INTERVAL_MS` | `1000` | `1000` | `1000` |
| `TRACE_QUEUE_SIZE` | `20000` | `50000` | `20000` |
| `TRACE_WRITER_COUNT` | `2` | `4` | `2` |
| `TRACE_EXCLUDED_PATH_PREFIXES` | empty | `/api/status,/metrics,/health,/static,/assets,/favicon` | same as scaled |
| `TRACE_RETENTION_DAYS` | `30` | `30` | `30` |
| `ASYNC_TASK_RETENTION_DAYS` | `7` | `7` | `7` |
| `RETENTION_DELETE_BATCH_SIZE` | `5000` | `5000` | `5000` |
| `RETENTION_DELETE_PAUSE_MS` | `10` | `10` | `10` |
| `RETENTION_SWEEP_INTERVAL_MINUTES` | `60` | `60` | `60` |
| `LOG_RECORD_LINE_FORMAT` | `full` | `compact` | `compact` |
| `APP_LOG_SINK` | `both` | `both` | `both` |
| `LOG_RETENTION_DAYS` | `0` | `3` | `1` |
| `LOG_MAX_TOTAL_SIZE_MB` | `0` | `20480` | `10240` |
| `LOG_MIN_FREE_DISK_MB` | `0` | `1024` | `1024` |
| `LOG_SAMPLE_INITIAL` | `0` | `100` | `100` |
| `LOG_SAMPLE_THEREAFTER` | `100` | `100` | `100` |
| `LOG_SAMPLE_TICK_MS` | `1000` | `1000` | `1000` |
| `DASHBOARD_CACHE_TTL_SEC` | `0` | `60` | `60` |
| `DASHBOARD_MAX_SITEWIDE_RANGE_DAYS` | `365` | `31` | `31` |

`TRACE_EXCLUDED_PATH_PREFIXES=-` disables exclusions. `APP_LOG_SINK` currently accepts
`file`, `stdout` or `both`; OTLP application logs are planned. `--log-dir` is a CLI flag,
not `LOG_DIR`. Existing `LOG_ROTATION_INTERVAL` defaults to daily; `ONLY_ONE_LOG_FILE`
disables time rotation. `OTEL_ENABLED` defaults to false; `external` does not enable it
or populate `OTEL_EXPORTER_OTLP_ENDPOINT` automatically.

Current environment readers treat empty as unset. Invalid numbers fall back to defaults;
some values are clamped before validation. Unknown profiles resolve to standalone; unknown
write modes resolve to batched; unknown sinks are discarded and an all-invalid list
resolves to `db`. This is **not raw-input fail-fast validation**. W1 must reject explicitly
invalid observability values before normalization can silently change their meaning.

### 3.2 Valid combinations

The following is the required release contract. Current checks cover important combinations,
but initialization and malformed-input tests must establish the complete matrix for G1.

| `TRACE_SINK` | `TRACE_WRITE_MODE` | `OTEL_ENABLED` | Required result |
| --- | --- | --- | --- |
| `db` | `sync` | false or true | Legacy SQL path; sample rate must be 1 |
| `none` | `sync` | false or true | Explicitly disable local trace recording; sample rate must be 1 |
| `db` | `batched` | false or true | Completed-record SQL writer and local sampling |
| `none` | `batched` | false or true | Explicit local-trace exclusion; no SQL fallback |
| `otlp` or `db,otlp` | `batched` | true | Require endpoint and successfully initialized provider |
| Any sink including `otlp` | `sync` | any | Reject; sync bypasses the completed-record sink |
| Any sink including `otlp` | `batched` | false | Reject; never count a no-op provider as export |
| `none` combined with another sink, unknown explicit values | any | any | Reject ambiguous or invalid configuration |

`OTEL_ENABLED=true` requires a configured provider even when local tracing is disabled;
existing OTel instrumentation is separate from `TRACE_SINK=none`. Syntactically valid
configuration followed by a collector outage is a runtime transport failure: report it,
apply bounded queues, and do not silently restore SQL tracing. Do not require a reachable
collector for every restart if the SDK can initialize offline with the configured exporter.

### 3.3 Planned settings and activation

These names are implementation targets; none may be advertised as functioning today.
Defaults remain disabled or legacy for every profile until explicitly configured.

| Planned setting / capability | Default and activation rule |
| --- | --- |
| `DASHBOARD_ROLLUP_ENABLED` | `false`; enables capture/workers only after schema and all-writer readiness checks |
| `DASHBOARD_READ_MODE` | `legacy`; `projection` requires verified coverage and enables the explicit stale/partial contract |
| `ROLLUP_INTERVAL_SEC` | Initial tuning value 60; freshness goal, not a durability window |
| Rollup budgets | Explicit worker concurrency, source-row, snapshot-age, memory and database-time limits; tune from W2 measurements |
| Rollup retention | Configure fine-grained and daily horizons separately; neither authorizes deleting billing records |
| `LOG_DB_RETENTION_DAYS` | `0` for all profiles; cannot be nonzero without the authority gate in W2.6 |
| Cursor/count capability | Explicit versioned request; legacy `p` and exact `total` remain unchanged |
| `LOG_COUNT_EXACT_MAX_ROWS` | Initial optimized probe budget 100000; never discover this limit using an unbounded exact count |
| `APP_LOG_SINK=otlp` | Optional new value after W3 adapter integration passes |
| `ANALYTICS_BACKEND` | `sql`; `clickhouse` selects verified mirror reads only after G4 |
| `CLICKHOUSE_ASYNC_INSERT` | Initial `false`; when enabled require `wait_for_async_insert=1` |

Do not introduce a profile-controlled `LOG_SORT_INDEXES_ENABLED` restart migration.
Index changes are explicit operational work. Do not retain `ROLLUP_REAGGREGATE_BUCKETS` as
a correctness mechanism; late corrections have no fixed two-bucket limit.

For planned analytics: `sql` requires no ClickHouse configuration. Selecting `clickhouse`
requires a build containing its optional driver, valid connection settings, durable capture,
replay workers and a verified projection generation. Invalid configurations fail startup;
a later mirror outage serves healthy SQL **projections**, explicitly stale data, or an
analytics-unavailable response, never unrestricted raw SQL.

## 4. Phase 0/1: finish the focused telemetry release

### W0 — Containment and compatibility (Backend + SRE)

Implemented locations: `common/logger/{sampling,disk_guard,log_retention}.go`,
`model/retention_chunk.go`, `controller/user_dashboard_cache.go`,
`model/user_stats_cache.go`, and exclusion helpers in `common/tracing/tracing.go`.

Required remaining work:

1. Preserve default fields, trace coverage, range and freshness; resolve the cadence
   exception in section 2.1. Keep full timestamp JSON and mixed-schema support.
2. Add writer-aware maximum active-file size, safe size rotation, and accounting for active
   plus rotated files. Define behavior when `ONLY_ONE_LOG_FILE` conflicts with a configured
   size ceiling: reject the combination rather than promise an impossible bound.
3. Separate pressure-check cadence from slow retention sweeps. Choose headroom using
   measured worst-case bytes/second, check/rotation latency and emergency reserve. At
   16 MB/s, 1 GB lasts about 62.5 seconds; an hourly guard cannot protect it. Start testing
   with checks at most every five seconds, then validate under the actual write rate.
4. On exhausted headroom, use a bounded emergency policy for best-effort application logs:
   cap bytes written, suppress excess including repeated WARN/ERROR, increment bounded-label
   suppression counters, and emit a rate-limited recovery summary. Specify hysteresis and
   recovery tests. Avoid recursive logging when the writer itself fails. Billing SQL is
   outside this suppression policy.
5. Test sampling through the real gin access logger with changing URLs and request IDs.
   Keep message text stable and variable values in fields. Current sampling is per
   `(level, message)`, not a request probability; WARN+ is currently unsampled.
6. Coalesce dashboard aggregate cache misses and enforce a budget in explicit optimized
   mode, including when Redis is unavailable. The current in-process user-total cache
   coalesces its query; that does not prove coalescing for all six Redis-backed aggregates.

Chunked deletion currently uses MySQL `DELETE ... LIMIT`, PostgreSQL a limited `ctid`
subquery, and SQLite a limited `rowid` subquery. Keep SQLite independent of the optional
`SQLITE_ENABLE_UPDATE_DELETE_LIMIT` build feature. Also bound the work to locate candidates,
not merely deleted rows; async-task predicates need their own plan. Retention must keep up
with newly eligible rows under concurrent traffic. Section 8 covers production operations.

### W1 — Trace correctness, memory and lifecycle (Backend + SRE)

Implemented locations are one flat `common/tracing` package: `recorder.go`, `sampling.go`,
`sink.go`, `sink_sql.go`, `sink_otlp.go`, and `tracing.go`. SQL row construction/batch writes
live in `model/trace_batch.go`; column overlay lives in `model/trace_columns.go`.
Lifecycle integration is in `middleware/tracing.go` and `main.go`. Keep this layout;
Phase 3 does not create a second OTLP trace sink.

Required remaining work:

| Boundary | Implementation / acceptance requirement |
| --- | --- |
| Outcome | Preserve existing panic-to-500 handling and test; add production-order tests for errors after streaming HTTP 200, upstream timeout, client cancellation and recovery |
| Sampling | Define semantic failure selection as well as HTTP status; measure effective retained fraction and separate time-to-first-token from total streaming lifetime |
| Recorder memory | Bound per-record bytes, external calls/events and retained string lengths; record truncation without credentials or payloads |
| Active requests | Budget active-recorder memory and admission under long-lived requests; the completed queue alone is insufficient |
| SQL batching | Bound rows **and bytes**, database parameters and flush-local buffers; do not drain the entire queue into an unbounded temporary slice |
| Flush | Barrier must include queued and in-flight batches from every writer; report failed persistence rather than equating zero pending work with successful persistence |
| Cancellation | Request cancellation must not cancel accepted sink work; worker lifetime is independent and controlled by shutdown deadlines |
| Read-after-write | A 500 ms flush-and-retry is best effort; sampling, drops, failures and deadlines can still leave no row |
| OTLP | Enrich the existing active request span, with one request SERVER span and correct parentage, timestamps, status and events |
| Shutdown | Stop admissions, drain HTTP and background/billing producers, close consuming sinks, flush exporters, then close databases; deadlines report unfinished work |

The current OTLP sink creates another `one_api.request` SERVER span. `otelgin` encloses
tracing middleware in `main.go`; a comment claiming the enclosing span already ended is
not sufficient evidence. Verify with an in-memory exporter and the actual middleware order,
then reuse that span. The existing telemetry setup already has an SDK batch processor.

Local selection at completion is not distributed tail sampling. SDK head sampling can
make a parent nonrecording, and local error selection cannot recover its missing spans.
Document SDK/collector policies together. Distributed tail sampling, when needed, belongs
in the optional collector deployment, with trace-affine routing and state capacity tests.

`TRACE_SAMPLE_RATE`, exclusions and `TRACE_SINK=none` govern local recorder/detail/SQL
selection. Reusing the enclosing `otelgin` span does not suppress its independent SDK
export. Local 5% sampling must not be advertised as 5% of all OTLP spans; reducing that
volume requires the separately configured and tested SDK/collector policy.

If `p` is the base rate and `q` the fraction selected by any unconditional rule, effective
selection is `q + (1 - q) * p`, before path exclusions and delivery failures. At `p=0.05`
and `q=0.5`, it is 52.5%, not 5%. Measure this for realistic streaming lifetimes.
At 10,000 RPS and 60 seconds average lifetime there are approximately 600,000 active
requests; multiply by bounded recorder bytes and include runtime/queue/batch overhead.

Current `oneapi_trace_records_total{outcome}` values include `sampled_out`, `queued`,
`dropped_queue_full`, `dropped_closed`, `written`, `write_failed` and `exported`.
`exported` currently means SDK handoff, not collector persistence. Retain bounded labels,
expose transport failures separately, and distinguish exclusions from sampling and drops.
Current SQL failures decrement pending work, so successful Flush is not yet proof that
all accepted records persisted. Trace loss remains permitted but must be observable.

G1 tests include queue saturation, partial batches, multiple writers, database outage,
ambiguous acknowledgments, exporter failure, cancellation and shutdown timeout. A healthy
shutdown test or a 5000-record benchmark below queue capacity cannot substitute for them.

## 5. Phase 2: bounded SQL reads with correct mutable projections

### W2.1 — Schema and semantic fixtures (Backend + Database Operations)

Before DDL, freeze differential fixtures for the six dashboard result sets and their
serialized DTO fields. Internal user IDs remain hidden. Preserve the existing capitalized
Go-derived fields (`Day`, `RequestCount`, etc.) and explicit `user_uuid` field rather than
silently introducing another wire schema.

All six datasets accept an authorized user scope; site-wide is available only to the
existing privileged caller. Retain historical identity labels stored on each usage row.

| Response array | Source filter | Exact grouping within scope | Measures |
| --- | --- | --- | --- |
| `logs` | `type=2` consume | UTC day, `model_name` | All consume measures below |
| `user_logs` | `type=2` | UTC day, historical `username`, `user_id`, `user_uuid` | All consume measures |
| `token_logs` | `type=2` | UTC day, historical `token_name`, `username`, `user_id`, `user_uuid` | All consume measures |
| `tool_logs` | `type=7` tool | UTC day, `model_name` exposed as `ToolName` | `RequestCount`, `Quota` |
| `tool_user_logs` | `type=7` | UTC day, historical `username`, `user_id`, `user_uuid` | `RequestCount`, `Quota` |
| `tool_token_logs` | `type=7` | UTC day, historical `token_name`, `username`, `user_id`, `user_uuid` | `RequestCount`, `Quota` |

| Consume field | Source expression |
| --- | --- |
| `RequestCount` | Number of consume rows |
| `Quota` | Sum of `quota` |
| `PromptTokens` | Sum of `prompt_tokens` |
| `CompletionTokens` | Sum of `completion_tokens` |
| `CachedPromptTokens` | Sum of `cached_prompt_tokens` |
| `CacheHitCount` | Number of rows with `cached_prompt_tokens > 0` |
| `CacheHitQuota` | Sum of the **whole row quota** for those cache-hit rows |

Tool counts represent tool invocations, not relay requests. Provisional, top-up, manage,
system and test rows do not enter either accounting dataset. No operational error count is
derived from these billing rows. Current account `total_quota`, `used_quota` and `status`
keep their existing source and freshness contract.

Do not join current names to reconstruct historical labels. Token UUID may aid identity,
but does not replace existing token-name grouping. Preserve null/empty identity behavior,
case/collation behavior and integer totals with overflow tests.

Use six physical projection families matching the six query shapes, not a universal
`user × model × token` table. Common key components are bucket start, grain, source shard,
projection generation and explicit authorization scope. Each family adds **only** its
required grouping fields. Use 64-bit aggregate storage with checked DTO conversion.

For model/tool totals, maintain distinct `scope_kind=user` and `scope_kind=site` rows;
site readers select only site rows. User/token breakdowns retain user identity and can be
filtered for a user or paged site-wide. Never sum site totals alongside their constituent
user rows. Retain per-shard contributions internally, but publish compact daily/current
read tables so requests do not perform source-shard reconstruction.

Start the shadow prototype with minute buckets for current work and daily read projections.
A candidate capture partition is 64 stable shards derived from immutable log UUID, including
hot users distributed across shards. This is a measured tuning candidate, not a throughput
guarantee. Store/query the partition efficiently; do not repeatedly calculate an unindexed
hash across history. Measure the added column/index cost before committing this layout.

Estimate physical cardinality as the sum of observed distinct keys **per family, bucket,
scope and shard**, including staging and daily tables. Sample active user/token/model
combinations at realistic traffic. Fine-grained projections can approach raw cardinality;
there is no assumed five-orders-of-magnitude reduction. Set independent fine/daily retention
from measured storage and required dashboard horizons before enabling projection reads.

### W2.2 — Durable changed buckets and fenced replacement (Backend)

Choose a SQL changed-bucket mechanism for the first implementation. No external queue and
no per-request ClickHouse call are required. The design adds synchronous SQL capture cost;
include that cost in the full relay benchmark before enabling it broadly.

Logical control tables in `LOG_DB`:

| Table | Key / required state |
| --- | --- |
| `usage_projection_dirty` | `(bucket_start, grain, shard)`; monotonic `generation`, `published_generation`, first dirty time, retry/error state |
| `usage_projection_owner` | Work partition; owner identity, database-issued monotonic fencing epoch and lease expiry |
| `usage_projection_build` | Unit, source generation, owner epoch, routing epoch, build ID, bounded staging progress and status |
| `usage_projection_coverage` | Published unit/build, source generation, snapshot time, historical coverage and completeness state |

Persist a routing epoch and active/retired state for each unit. Compaction and repair routing
changes advance that epoch so work begun against an earlier layout cannot publish afterward.

Use portable logical types with engine-specific, tested DDL. Generation increments and
initial dirty-row creation must be atomic under concurrent writers. Do not implement an
unlocked read-then-write counter. Give multi-unit updates a deterministic lock order and
retry transient conflicts with cancellation and a bounded retry budget.

**Capture transaction:** route generic inserts, tool inserts, provisional inserts,
reconciliation, corrections, relevant identity repairs and logical deletions through the
usage mutation helper. Commit the usage change and dirty-generation increment in the same
`LOG_DB` transaction. Record both old and new units when time or partition identity changes.
Bulk deletion must identify affected units before removing their source rows. No dirty
notification may be merely an in-memory callback after commit.

The synchronous provisional helper still returns its committed database ID. Dirty-row
failure rolls back its usage transaction; callers cannot assume projection capture succeeded
because the usage insert executed first. Do not change existing account/quota transactions
or claim cross-database atomicity.

**Worker algorithm:**

1. Claim a bounded set of dirty units under an owner epoch. Eligibility flags such as
   `IsMasterNode` restrict candidates but do not elect a unique owner.
2. Open a consistent source snapshot; read its dirty generation `g` and source rows from
   that same snapshot. Calculate all six applicable projections with UTC ranges and the
   source handle's dialect. Publish no intermediate result.
3. Stage a full replacement for the unit, including an empty result if all groups vanished.
   Enforce source rows per page, total bytes, snapshot age and database-time budgets.
4. In a short publication transaction, lock/check the current owner and routing epochs and
   compare the unit's already-published generation. Reject a superseded owner, retired unit
   or older build. Replace the complete unit, deleting obsolete groups, and publish its
   coverage atomically.
5. Acknowledge **only generation `g`**. If a concurrent source mutation produced `g+1`,
   leave it dirty. A consistent older snapshot may be published as explicitly stale while
   the newer generation remains scheduled; never let it overwrite a newer publication.
6. In the same transaction, invalidate any parent daily/current read projection. Publish
   each parent from a consistent snapshot of its child contributions and coverage; expose
   its own generation. Parent workers use the same owner/routing fences, dirty-generation
   comparison and atomic publish/acknowledgment protocol. Requests read only published parents.

A worker's snapshot and acknowledgment identify the actual processed boundary. `MAX(id)`,
a wall-clock timestamp or the largest observed `updated_at` does not prove that earlier
transactions committed. `as_of` is descriptive metadata, not an acknowledgment protocol.

Reuse the compact-UUID coordinator's pinned-session advisory lock mechanics where suitable,
but use a distinct namespace based on **LOG_DB identity**. That coordinator currently
coordinates the primary database and checks process-local ownership around independently
safe operations. Those checks are insufficient for replacement aggregates. Validate the
persisted fencing epoch **inside the publication transaction** so a worker that lost its
lease cannot overwrite its successor. SQLite ownership and publication use its tested
single-writer transaction behavior; PostgreSQL/MySQL use their own transaction/lock syntax.

Capture row contention is a release criterion. Sharding can reduce contention but adds
projection rows and source-index maintenance. If the dirty-row lock budget fails under hot
traffic, stop rollout and evaluate a transactional append-only invalidation feed as a
separate measured alternative. Do not move invalidation into lossy memory to meet latency.

**Backfill, compaction and repairs:**

- Deploy additive schema, then upgrade **all** usage writers and maintenance tools before
  enabling capture. During mixed-version deployment keep legacy reads; old writers cannot
  silently bypass capture while projections are trusted. Require an explicit readiness
  fence and stop unsupported old-writer rollback after activation until projection reads
  are disabled and a fresh backfill is scheduled. Triggers are a separate, tested option
  only if mixed-version capture is operationally necessary.
- Prioritize current dirty units, then history within a separate backfill budget. Persist
  unit-level completion, retry state and uncovered intervals; empty intervals also need
  coverage records. Source generation zero means “not yet built,” not “empty and complete.”
  Atomically seed each historical unit, including empty intervals, with generation at least
  one and unpublished coverage before its source snapshot; preserve any existing dirtiness.
- A bounded source scan uses a consistent snapshot. If its time/memory budget expires,
  abandon its staging result and retry or subdivide the unit; do not resume a different
  snapshot and declare one consistent result. Historical units that exceed budgets must
  be split before scanning. Never hold a database snapshot across the entire history.
- Keep minute contributions until daily publication is complete and verified. Compaction
  atomically changes each historical unit's repair routing to daily rebuild before removing
  fine contributions, advances its routing epoch, retires obsolete builds, and transfers
  outstanding dirty work to daily repair. Capture consults that state transactionally, so
  a correction during compaction cannot be lost. Late minute/parent builds must fail their
  routing fence. Retained raw SQL remains available for historical rebuilding.
- A historical daily repair can stage smaller source subunits. Publish only if its captured
  dirty generation is still valid for the complete rebuild; otherwise retain pending work
  and retry. Do not add a corrected minute to a daily total whose old contribution is
  unavailable. Throttle repairs independently from current-period refresh.
- A correction months later must reopen its retained historical unit. There is no trailing
  two-hour correctness limit. Periodic audits detect missed writers and corruption; they
  supplement transactional capture rather than replace it.

G2 must cover a provisional row created at 10:59, aggregated before completion, finalized
hours later, and corrected days later; moving labels/groups; deletion of the last row in a
group; duplicate work; crashes between staging and publish; ownership transfer; and a source
commit racing publication. Assert all six datasets against raw SQL at a common source
snapshot/generation, including empty groups and negative quota corrections.

### W2.3 — Projection-only optimized reads (Backend + Frontend)

Explicit `DASHBOARD_READ_MODE=projection` serves current and historical data from published
SQL read projections. It never scans the current hour, the current minute, or all missing
history on a dashboard request. At 10,000 RPS those raw windows can contain 36 million and
600,000 rows respectively; smaller grain alone is not an execution budget.

Use an opt-in response capability exposing:

- `as_of`: UTC source-snapshot/publication timing with documented meaning.
- `generation`: opaque reference to the selected immutable read generation.
- `coverage`: requested interval, covered intervals and historical gaps.
- `freshness`: current, stale or unavailable; include pending-correction status and lag.

If different units have different source snapshots, say so in coverage; do not claim a
single globally consistent timestamp. A response must not combine advanced coverage with
an older/incomplete aggregate. Read data and its manifest from one transaction or pin the
immutable generation throughout the response. Paginated breakdowns pin that same generation
until its documented expiry, then return an explicit restart-required result.

Parent read generations and manifests are immutable and retained through issued cursor
expiry and the configured stale-serving window. Child contributions may be replaced in
place. Garbage collection must respect published/active references; deliberate history
expiry explicitly invalidates affected cursors before removing their generations.

Return available projections during backfill with honest partial coverage. For a request
requiring complete data, return a bounded unavailable response. Redis/ClickHouse failure
or lag never initiates raw fallback in optimized mode. Preserve the last verified generation
for a configured stale-serving window; after expiry, report unavailable.

Legacy requests retain their original arrays, totals and range/freshness behavior. Add
versioned overview and paginated model/user/token/tool breakdown capabilities for Modern.
These must cap page size and encoded response bytes and support arbitrary navigation through
explicit continuation, without silently truncating legacy arrays. Translate loading, stale,
partial, unavailable and restart-required UI messages in every Modern locale; maintain
Default, Berry and Air against the legacy API.

Initial **test budgets**, to be adjusted only with recorded evidence before release:
projection query timeout 1 second; breakdown page at most 200 rows and 1 MiB encoded;
at most two concurrent expensive reads per node; cache-miss coalescing per effective scope
and normalized query. Oversized individual records receive an explicit error, not silent
field truncation. Query cancellation, row limits and serialized-byte limits must all work.
These budgets constrain work; they are not measured latency claims.

Cache keys include caller authorization scope, target user, normalized range/filters, dataset,
read/count mode and generation. Recheck authorization before serving a hit. Keep bounded
in-process coalescing and admission even without Redis. For explicitly budgeted standalone
raw reads, enforce the same timeout/admission framework; legacy behavior changes require
opt-in. Rollups cannot authorize access that raw APIs would deny.

### W2.4 — Additive keyset pagination and honest counts (Backend + Frontend)

Legacy `/api/log/` and `/api/log/self` preserve `p`, supported `size`/`items_per_page`,
offset behavior, filters, sorts, provisional exclusion and exact `total`. Their default
sort is **`id DESC`**. Existing list `end_timestamp` is inclusive; preserve that wire
contract. Dashboard inclusive calendar dates instead normalize to `[start, next-day-00:00)`
in UTC. New cursor date semantics must be explicit, not a silent legacy boundary change.

Introduce a versioned cursor capability used explicitly by Modern:

1. Validate page size, allowlisted sort and normalized filters before SQL construction.
   Initial cursor sort is `created_at DESC, id DESC`; use a unique tie-breaker and a
   portable lexicographic predicate. Unsupported cursor sorts return a clear error.
2. Fetch `page_size + 1`, return `has_more` and `next_cursor`. Do not run an exact count
   as part of ordinary cursor navigation.
3. Use an opaque authenticated cursor binding last key, normalized filters/sort, effective
   authorization scope, version and expiry. Enforce authorization independently on every
   request; compare sensitive authenticators in constant time. Bound cursor size/decoding.
4. Define live traversal semantics: newer inserts before the anchor do not shift subsequent
   pages; deletes may shorten them; corrections or late provisional finalization can change
   membership. This is not a stable audit snapshot. Use a separately pinned export for
   snapshot-complete audit work. Expired or mismatched cursors require restart.
5. Represent counts as `{value, quality}` with `quality=exact|lower_bound|estimate|unavailable`.
   A bounded probe of at most `N+1` matching rows can establish exactness or a lower bound;
   an exhausted time budget yields unavailable. An estimate is never rendered as `1000+`.
6. An optional exact-count request needs its own explicit time/admission budget and cache.
   Do not first compute an unbounded count to discover it exceeds `LOG_COUNT_EXACT_MAX_ROWS`.
   Do not estimate keyword, UUID, content, or other unrepresented filters from rollups.

Select indexes from real query families and execution plans: global time/id cursor,
authorized user time/id cursor, type-filtered paths, and supported detail filters. A
`(type, created_at)` index alone does not establish an efficient unfiltered global cursor.
Candidate `(created_at, id)` and `(user_id, created_at, id)` paths must earn their write/storage
cost. Do not add every possible composite index. Validate plans with skew, broad filters,
deep history, cold caches and concurrent inserts/retention.

### W2.5 — Explicit index migrations (Database Operations)

Inventory actual database catalogs: the inspected model declares 17 secondary indexes and a
separate migration adds unique `uuid`, but deployed counts can differ. Measure usage,
index size, selectivity, sort/query latency and WAL/redo before choosing removals. Integer
or sorting indexes are not inherently useless.

Ship a dry-run migration plan with engine/version prerequisites, estimated disk headroom,
lock/time limits, progress, cancellation and recovery steps. Do not build/drop large indexes
inside routine application restart. PostgreSQL concurrent builds need explicit handling for
failure and invalid indexes; they cannot be treated as ordinary transactional DDL. Follow
[PostgreSQL CREATE INDEX documentation](https://www.postgresql.org/docs/current/sql-createindex.html).
MySQL and SQLite need separately tested lock/rebuild procedures for the pinned versions.

Build replacement access paths, verify plans, then remove only approved old indexes. Prevent
old/new `AutoMigrate` from unexpectedly recreating deliberately removed indexes. Test real
old-binary startup. Rollback includes measured rebuild time, spare disk and admission limits;
flipping a flag and restarting is not a multi-terabyte rollback procedure.

### W2.6 — Authoritative history and deletion (Database Operations + product owner)

**Chosen policy: retained SQL authority.** Keep detailed `logs` records in SQL for the
operator's explicitly documented audit/support period. ClickHouse and rollups are rebuildable
analytics. There is no default 30-day SQL or 180-day mirror TTL that overrides this policy.
`LOG_DB_RETENTION_DAYS=0` remains the default for every profile; no universal audit period
is prescribed.

Before enabling any automatic billing deletion, record the required audit horizon, customer
investigation needs, backup/restore capability and who accepted the history expiry. The
configured cutoff must retain at least that horizon, and published dashboard coverage must
not claim detailed authority beyond it.

A deletion unit is eligible only when all applicable checks hold:

- It is older than the explicitly configured authority horizon.
- It contains no unresolved provisional records or records required by pending reconciliation.
- Required projections/backfill/repairs have finished and no dirty generation depends on it.
- Any separately required mirror replay and archive verification have acknowledged it.
- Concurrent mutation/capture and retention use transactional eligibility checks so a
  correction cannot race past the retention checkpoint.

Distinguish administrative logical deletion (invalidates affected aggregate contributions)
from deliberate expiry of a whole supported history interval (expires its coverage and
read generations consistently). Do not leave a projection silently serving a different
history range from the configured contract. Manual purges also need an explicit history
consequence and safe capture; preserve the public Go signature and legacy API compatibility.

Shortening operational SQL below the required audit horizon is **blocked** until a separate
archived-authority implementation exists. That implementation must provide canonical row
identity/revisions, immutable manifests/checksums, durable storage, verified retrieval and
restore, correction linkage and archive-before-delete acknowledgment. An analytics TTL,
aggregate table, or unverified export is not that archive. This revision does not implement
or implicitly approve an archive.

## 6. Phase 3: optional telemetry integration

### W3.1 — Reuse the Phase 1 trace pipeline

Close W1's existing-span and provider validation gates; do not duplicate the implemented
OTLP sink as new work. A single collector is sufficient for ordinary export. Two-tier
collectors with trace-ID-affine routing and stateful tail sampling are optional for a
measured distributed tracing requirement. Document memory, maximum trace duration,
`decision_wait`, incomplete/late spans and collector failure behavior.

### W3.2 — Optional application-log bridge

The upstream `otelzap` bridge uses `go.uber.org/zap/zapcore`; this repository uses the
Laisky fork. Compile a minimal adapter against the actual module graph before designing
configuration around it. The bridge needs a context-bearing field to emit with request
context; otherwise it uses a background context. Test actual exported trace/span IDs from
`gmw.GetLogger(c)` in the request path. See the
[upstream bridge source](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/bridges/otelzap).

Existing file logs already contain stored request/trace correlators. OTLP standardizes
transport and context representation; it does not introduce correlation for the first time.
Keep the bridge isolated and optional, preserve file/stdout defaults, and test queue bounds,
transport outage, sampling and final flush. Do not put account IDs, tokens or request content
into unbounded metric labels or expose credentials in logs.

Pin and integration-test compatible module versions. The August 31, 2026 announcement puts
`otel/log` and `otel/sdk/log` in release candidate status; exporters are outside that RC
scope. Do not label the whole pipeline stable based on that announcement. See
[OpenTelemetry Go Logs API/SDK RC](https://opentelemetry.io/blog/2026/go-logs-api-sdk-rc/).

### W3.3 — Operational metrics

Extend through optional recorder interfaces; do not add mandatory `MetricsRecorder` methods.
Record request outcomes, time-to-first-token, latency, queue bytes, drops, projection lag,
backfill backlog and retention throughput from their actual operational sources. Use
bounded, documented label sets and exemplars/correlators where appropriate. Grafana can
serve an operational view, but sampled telemetry is never billing truth or a replacement
for per-user financial aggregation. Keep SDK instrumentation and dashboards consistent.

## 7. Phase 4: optional durable analytics mirror

### W4.1 — SQL outbox and replay before a ClickHouse driver

Keep the SQL mutation helper as the ledger writer. Use separate read and replay boundaries,
for example an `AnalyticsReader` implementing all six datasets and a `UsageMirror` worker
consuming committed mutations. Do not define a `UsageStore.RecordUsage` whose buffering,
Flush or Close inherits telemetry drop semantics.

Commit an outbox record with each usage mutation in **the same LOG_DB transaction**.
Capture inserts, provisional transitions, corrections, identity repairs and relevant deletes.
Outbox payloads contain a complete after-image with a stable event UUID, strictly monotonic
per-event revision, mutation/tombstone type, original timestamp, historical identity labels and IDs, log type,
quota/token/cache values, and the detail fields needed to reproduce the retained SQL
contract. Reuse immutable `logs.uuid` where available; repair missing UUIDs before activation.
Request and trace IDs are correlators, never unique usage-event IDs.

Per-event revision increment and outbox creation are atomic. `updated_at` milliseconds
alone is not a collision-free revision. Outbox delivery is at least once: claim bounded
batches, retry with backoff and stable retry identity, and acknowledge only confirmed
records. Sequence IDs help traversal but do not establish commit order. Persist pending
and delivered state per claimed work; never skip a delayed transaction because a larger
ID was observed. Retain enough replay evidence for bounded repair.

Mixed-version rollout follows W2's all-writer fence. Before capture is universal, use only
shadow mirror reads. After activation, rolling back a writer requires disabling trusted
mirror/projection reads and arranging recapture/backfill. If old writers must remain, a
verified database capture mechanism is required; request-side hooks alone are insufficient.

Bootstrap pre-outbox history explicitly. After additive UUID/revision preparation and the
all-writer fence, enable durable capture and retain every subsequent mutation for bootstrap
replay. Export retained SQL in bounded, consistent source-unit snapshots with event UUID and
revision, recording each unit's coverage and source boundary. Use the same revision space
for initial rows and subsequent updates. A scan's largest key only bounds enumeration; it
is not a commit checkpoint. Replay all captured concurrent mutations, including inserts
that commit after an already-scanned range. Deduplicate overlapping snapshot/outbox records
by `(event UUID, revision)` and select the newest included revision, retaining tombstones.
Do not garbage-collect bootstrap replay state or serve trusted history until every required
unit is covered and reconciled at the explicit publication boundary in W4.2. Interrupted
exports resume by unit, without claiming that separate snapshots are one global SQL snapshot.

On ClickHouse outage, the SQL outbox accumulates durably. Define maximum backlog bytes,
age and available SQL disk, alert thresholds, recovery capacity, and admission behavior
before exhaustion. Never silently discard committed replay work. An outbox failure must
not leave a usage mutation committed without its outbox; account/ledger atomicity remains
a separate baseline limitation. A mirror does not remove SQL insert or index workload.

### W4.2 — Selected correction model: canonical versions and rebuilt aggregates

Use **versioned canonical records with controlled aggregate rebuilding**. Do not use an
incremental summing view over appended versions. An incremental ClickHouse materialized
view processes inserted blocks and does not automatically undo an old row's contribution
when a revised row arrives. See
[ClickHouse incremental materialized views](https://clickhouse.com/docs/concepts/features/materialized-views/incremental-materialized-view).

The storage contract is:

| Object | Required behavior |
| --- | --- |
| Canonical event versions | Immutable event identity, revision, deletion marker, log type and historical fields; partition/key must keep one event's revisions reconcilable |
| Canonical query | Select latest revision per event at the chosen replay boundary, then apply tombstone/type filters; retries of the same revision must have identical payload |
| Projection staging | Build all six semantic families from canonical rows for affected units, with source boundary and generation recorded |
| Published manifest | Switch to a complete, verified generation atomically from the reader's perspective; incomplete generations remain invisible |

Mutable fields such as username/model/token must not become part of the identity used to
deduplicate an event. Keep the original event partition immutable; a timestamp correction
must explicitly invalidate both aggregate periods. Version/tombstone retention must prevent
an older replay from resurrecting deleted data.

`ReplacingMergeTree` may optimize canonical storage but deduplicates during background
merges; readers requiring current logical state need an explicit latest-version query.
It does not repair downstream aggregates already summed. See
[ReplacingMergeTree documentation](https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/replacingmergetree).
This design does not require `SummingMergeTree`; if later used for partial aggregates,
queries must still group and sum remaining physical partial rows rather than assume merges
finished. The previous example materialized-view DDL is intentionally replaced by this
contract; final DDL must pass correction/replay fixtures before acceptance.

A concrete publication protocol is required for G4: for a bounded source unit, take a
consistent LOG_DB snapshot of every committed mutation not covered by its prior verified
generation, together with matching canonical SQL comparison data. Persist that generation's
membership/export, then claim, deliver and verify the set in bounded batches. The prior
verified base plus this complete set defines the boundary; an arbitrary subset of claimed
rows does not. Later commits belong to subsequent generations. Publish the canonical
boundary and completed aggregate manifests only after verification. A missing or ambiguous
acknowledgment prevents advancement. Rebuild from that exact boundary; never use unrestricted
“latest” versions to compare against an earlier SQL snapshot. Subdivide units that exceed
snapshot budgets and expose per-unit boundaries rather than pretending to have one global cut.

### W4.3 — Ingestion and version-specific settings

Pin the tested ClickHouse patch version and engine settings. Current documentation describes
async-insert deduplication with dependent views from 26.2; plain `MergeTree` needs positive
`non_replicated_deduplication_window`, while replicated engines maintain a log by default.
Deduplication windows provide retry protection, not permanent event uniqueness or billing
correction semantics. Require `wait_for_async_insert=1` for server buffering. See
[ClickHouse asynchronous inserts](https://clickhouse.com/docs/concepts/features/operations/insert/asyncinserts).

Benchmark application batching, server buffering and their combination using batch bytes,
producer concurrency, acknowledgment latency, parts created and merge backlog. Server
buffers can combine multiple producers, so combining them with client batches is not
categorically wasteful. Do not copy the trace writer's 500-row setting. Test retries both
inside and outside the deduplication window, identical retry identities, changed block
boundaries and ambiguous network failure; canonical revision semantics must remain correct.

Keep the optional driver isolated, with a clear build capability check. Default binaries
must not initialize ClickHouse. Packaging an optional driver and enabling mirror reads are
separate decisions; standalone remains independent of it.

### W4.4 — Reconciliation, repair and cutover

At a common processed boundary, compare counts and quota/prompt/completion/cached-token
sums by **UTC day, user and log type** at minimum; also compare the six endpoint datasets,
cache-hit measures and historical labels. Include duplicate zero-quota events, offsetting
quota errors, wrong-user attribution and missing tool rows. A matching global daily sum
is insufficient. Use wider comparisons and event-level checksums for discrepant units.

Separate ingestion lag from divergence: only compare boundaries both sides have processed.
Preserve the SQL snapshot or a canonical export for that boundary until verification ends.
Expose backlog, oldest pending age, retry/failure counts and reconciliation status. Repair a
bounded set of event revisions or rebuild an affected projection generation; verify before
read cutover. Retain an operator command/runbook to inspect and replay work without creating
additional charges.

On failure, select healthy SQL projections, an explicitly stale verified generation, or
unavailable analytics. Never fan out synchronous request writes to SQL and ClickHouse and
never recover the dashboard by starting unrestricted raw SQL scans. Mirror retention stays
independent of the retained SQL authority policy in W2.6.

## 8. Capacity model and production retention

### 8.1 Arithmetic, not a hardware claim

Use decimal units in this table. These figures are calculations from stated assumptions,
not measurements of deployed row size or achievable throughput.

| Assumption | Calculated volume |
| --- | --- |
| 10000 relay RPS, one usage insert per request | 864 million usage rows/day |
| Usage data at 1500 bytes/row including assumed indexes | 1.296 TB/day |
| Same usage data for 30 days | 38.88 TB |
| Same usage data for 90 days | 116.64 TB |
| Traces at 600 bytes, exactly 5% retained, for 30 days | 777.6 GB |
| Unsampled application output at 16 MB/second | 1.3824 TB/day |

Add replicas, backups, staging, outboxes, projection tables, WAL/redo, temporary sorts,
compaction, index builds and filesystem reserve separately. Tool invocations can add usage
rows; provisional reconciliation and later corrections add mutations and row versions.
A mirror adds ingestion/replay work while SQL still receives the authoritative mutation.

For each workload publish:

- `usage_inserts = relay_rate * mean_insert_rows_per_relay`.
- Usage updates, account updates and capture/outbox writes per relay, separately.
- Retained traces using measured effective selection, not just `TRACE_SAMPLE_RATE`.
- `trace_insert_statements = persisted_trace_rows / observed_rows_per_successful_statement`,
  plus retry, split and failed statements. No `rate / configured_batch_size` upper bound.
- Bytes per event and index, WAL/redo bytes per relay, database CPU/IOPS and lock wait.
- Active requests and recorder bytes, queue/batch bytes, memory/GC and exporter backlog.

Timed flushes, multiple writers, parameter limits and retries change batch occupancy.
Statement reduction is not equivalent to row, index, storage or CPU reduction. Registered
user count does not bound Plane A: benchmark hot account/token/channel keys and real
pre/post-consume paths. An 8-vCPU PostgreSQL host is an experiment configuration, not a
sizing recommendation inferred from another gateway.

### 8.2 What the existing benchmarks support

The companion report used Go 1.27.1, PostgreSQL 17, MySQL 8.4 and file-backed SQLite on a
Ryzen 7 5700G machine with 27 GB RAM. Preserve exact image digests and settings in the next
record. The trace arms used batch 500, flush 1000 ms, **2 writers and a 20000 queue**;
current scaled defaults are 4 writers and a 50000 queue. Do not call those arms the exact
scaled profile.

| Historical observation | Supported conclusion | Limit |
| --- | --- | --- |
| Representative sync path 12 statements/request; batched request goroutine 0 | Opt-in batching removes synchronous trace SQL from that path | Unchanged standalone retains sync behavior; billing/account work remains |
| Saturated batched workload about 0.0022 statements/trace | Statement/commit overhead can be amortized | Only observed occupancy; low-rate runs differ |
| PostgreSQL WAL about 2602 to 746 bytes/request without sampling | Eliminating repeated trace-row updates reduced durable trace work about 3.5 times in that arm | Not total gateway/database work |
| Arrival curve: about 21 traces/insert at 50/s and 431 at about 1870/s | Batch efficiency depends on arrivals | Driver saturated around 1900/s, not 10000/s |
| Chunked deletion reduced longest DELETE approximately 6–22 times | Smaller transactions reduce individual lock exposure | Sweep was 2–8 times slower; total degraded SQLite requests increased |
| Site-wide user-total cache amortizes a scan and coalesces misses | Repeated reads can avoid repeated account-table aggregation | Cold viewer still pays a scan; this is not the six usage-rollup queries |
| Compact `record log` line saved 20–45% for tested content | Less application text per emitted line | Sampling already removes lines; do not add both unsampled savings |

The 16-way `RunParallel` measurements report reciprocal throughput, not individual request
latency or p99. The batched attributable cost was near the harness floor. A recorder-only
approximately 3 microseconds does not prove end-to-end p99 overhead below 2 ms. The in-tree
sync comparator includes new columns and some extra work compared with the historical
binary, biasing that comparison in favor of the optimization.

Unsampled persistence was checked for 5000 submissions against a 20000-record queue; this
does not exercise saturation. Retention used a fully cached 205000-row fixture and does
not prove steady-state catch-up on tens of terabytes. Keep these honest limits beside the
numbers, not in a disconnected obsolete table.

### 8.3 Retention feasibility is part of capacity acceptance

At steady state retention removes **newly expired** data. Deleting 26 billion rows is a
backlog scenario, not every ordinary sweep for a 30-day horizon. Measure eligible arrivals
including tools, sweep throughput, scan work, lock duration, oldest eligible age and disk
reclamation under concurrent relay/dashboard load. Require sustained catch-up greater than
eligible arrivals with agreed headroom; if it cannot catch up, the topology fails G5.

Keep portable bounded-key deletion as the default. A certified high-volume PostgreSQL or
MySQL deployment may need time partitioning and partition retirement, but that requires
engine-specific migration, keys/uniqueness analysis, eligibility checks and restoration
procedures. It need not block G1/G2, but it cannot remain an unmeasured promise in a
10,000-RPS production claim. SQLite remains supported for the zero-dependency deployment;
it receives no implied 10,000-RPS certification from this proposal.

Before bulk retention or index work, take and verify a recoverable backup, record table and
index catalogs, estimate temporary storage, configure lock/statement limits, and establish
stop conditions. During work, monitor relay latency, lock waits, database disk/WAL and sweep
lag; pause when budgets are exceeded. Test interrupted/resumed operations and mixed-version
startup. Restore/rebuild time belongs in the rollback plan, not just the happy-path timing.

## 9. Acceptance gates and evidence requirements

Each work item needs an assigned individual before implementation begins. Backend owns
correctness fixtures, SRE owns reproducible load runs, Database Operations signs off
migration/history safety, and Frontend verifies opt-in behavior and translations.

| Gate | Required evidence | Enables |
| --- | --- | --- |
| G1: focused telemetry | Default compatibility, mixed schemas/binaries, config matrix, W0 disk/cache limits, W1 semantic outcomes/memory/flush/OTLP order | Release opt-in Phase 0/1; no target-load claim |
| G2: SQL read cutover | Transactional capture, all-writer fence, mutation/replay equality, ownership failure, backfill/repair, cursor/count and response budgets | Opt-in projection dashboards and upgraded Modern navigation |
| G3: optional OTLP logs | Compile/integration with forked Zap, real trace/span correlation, pinned SDK/exporters, outage/resource/shutdown tests | Optional OTLP application logs and operational views |
| G4: mirror cutover | Durable outbox, correction/deduplication, common-boundary reconciliation, bounded replay, outage/backlog/repair and publication tests | Optional ClickHouse reads; no shorter authoritative SQL history |
| G5: deployment capacity | Full relay offered/achieved load, retained-data workload, storage/retention catch-up, failure recovery and operational rollback | Capacity claim for that exact tested topology/configuration |

G2's cursor work can release before projection cutover if independently tested. G3 and G4
are optional; neither blocks earlier releases. Enabling billing retention additionally
requires the W2.6 authority gate regardless of which release introduced its settings.

### 9.1 Required correctness and failure matrix

| Area | Minimum cases |
| --- | --- |
| Compatibility | No-env standalone, literal old/new binaries both directions, unmigrated schema, old writer after activation, legacy response/sort/count/date boundaries |
| Protocols | ChatCompletion, Responses and Claude Messages conversions; streaming/non-streaming; tool invocation and asynchronous reconciliation |
| Trace pipeline | No-tracing baseline, excluded/sampled/selected/dropped separation, post-200 error, panic, timeout, cancellation, queue-byte limits, active long requests, multi-writer in-flight flush |
| Usage capture | Provisional ID preserved; capture rollback; commit ambiguity; retry; late correction; dirty generation concurrent with publication; no claimed account/LOG_DB atomicity |
| Projections | Exact six-dataset equality, historical name changes, same token name across UUIDs, cache-hit quota semantics, empty groups, negative corrections, leap day and UTC final-day boundary |
| Ownership | Lost database lock/session, lease expiration, successor publication, delayed old worker, process crash during build/publication |
| Backfill | Current data prioritized, holes visible, empty ranges covered, interrupted staging, compaction racing correction, correction outside fine-grained retention |
| Read APIs | Hits/misses and concurrent cold callers, Redis unavailable, bounded count probe, filters absent from rollups, cursor tampering/scope change, retention/concurrent inserts, result byte limit |
| Mirror | Duplicate revision, reorder, tombstone, zero-quota duplicate, wrong user, offsetting quota errors, retry-window expiry, interrupted snapshot/replay, collector/store outage |
| Retention / DDL | No unresolved source deletion, concurrent reconciliation, archive gate when applicable, scan and lock budgets, backlog catch-up, cancellation/resume, real old/new startup |
| Application logs | Variable URLs/IDs, active-file overflow, error storm, exhausted disk, writer failure, suppression metrics and recovery |
| Shutdown | Drain every producer before sinks; failure and timeout accounting; exporters before DB close; crash restart recovery for durable capture/replay |

Where new durable acknowledgment is introduced, assert no silent loss or double application
of acknowledged records under crashes, retries and ambiguous responses. For existing billing
helpers, document and measure their failure behavior instead of silently upgrading the stated
guarantee. For best-effort telemetry, assert accounting and resource bounds rather than
zero loss under every failure.

Use `github.com/stretchr/testify/require`. Keep test scenarios in the repository, not ad hoc
one-off scripts. New code follows the 800-line limit, function/interface comment rules,
context propagation, structured request logging, wrapped errors and single-processing rule.
Use the owning handle's dialect, validate all query input and maintain UTC behavior.

### 9.2 Reproducible full-relay load gate

Create a checked-in harness with an **open arrival-rate** workload and controlled upstream
simulator. Report attempted/offered, achieved and failed RPS, load-generator dropped
iterations and gateway rejections separately. A slowing gateway must not make the generator
quietly lower its offer. Distinguish injected upstream failures from gateway failures.

Record the complete topology: gateway node count/CPU/RAM, engine patch versions, database
hardware/storage, pools, network, Redis/collector/mirror if enabled, SQL/LOG_DB separation,
all effective settings, compiler/build and tracing policy. Publish traffic mixes and row
sizes, hot-user/token/channel distributions, token/tool use, stream lifetimes and time to
first token, cancellations, retries and corrections. Include the 60-second-average streaming
scenario or explicitly limit the resulting certification to a shorter measured workload.

Test no-tracing, unchanged standalone and explicit optimized configurations independently.
For each supported capacity topology, ramp offered traffic through 1000, 5000 and 10000 RPS,
then run at least one hour at target after warmup, plus a soak spanning retention/rotation
and projection catch-up cycles. Seed representative retained data matching the configured
history, indexes, cardinality and storage occupancy; a small hot table is not a substitute.
If full-scale retention data is unavailable, label the result partial and leave G5 open.

Report at minimum:

- Relay end-to-end and gateway-attributable p50/p95/p99, time to first token, achieved RPS,
  error rates and dropped iterations; use paired runs and uncertainty intervals.
- Account/usage/trace/capture statements and rows, effective batch occupancy, index bytes,
  WAL/redo bytes, CPU, disk IO, lock waits and pool saturation.
- Active requests, memory/GC, recorder and queue bytes, effective sampling and all losses.
- Projection freshness/coverage, dirty/backfill backlog, worker scanned rows/bytes and
  duration, current versus historical work, cache hit/miss latency and response bytes.
- Outbox replay/reconciliation lag when enabled; retention eligible-arrival and deletion
  rates, oldest eligible age, active-file pressure and storage reserve.

Initial performance goals, **not current results**: trace-attributable p99 overhead at most
2 ms against the no-tracing arm; optimized dashboard p95 at most 500 ms and p99 at most
1 second for bounded responses on both hits and misses; current-period projection freshness
p99 at most 120 seconds in healthy steady state. Sustain target offered throughput with no
unexplained dropped iterations, no unbounded queues/memory/backlog, and no hidden raw fallback.
Set allowed upstream/gateway error and maintenance degradation budgets before the run.

Inject database slowdown/outage, collector/mirror outage, worker death, ownership transfer,
cache loss and shutdown timeout. Measure recovery throughput greater than normal ingress
with enough storage to drain the measured backlog. Intentional best-effort drops remain
visible and within the operator's configured telemetry policy; durable replay is not dropped.
A successful component benchmark or healthy shutdown does not close these gates.

### 9.3 Routine repository checks and this revision's boundary

Required implementation checks remain:

```sh
go vet ./...
go test -race ./...
make build-frontend-modern
```

The frontend target uses Yarn. During this documentation revision, all three commands
completed successfully against the existing workspace; many Go packages used cached results.
Only this proposal was intentionally edited. These checks validate repository build/test
health, not the planned features, historical benchmark reproduction or G1–G5 acceptance.

## 10. Rollout, rollback and implementation handoff

| Step | Owner | Concrete deliverable | Stop / rollback condition |
| --- | --- | --- | --- |
| 1. Freeze baseline | Backend + SRE | Published source/evidence bundle and default-behavior fixtures | Missing source/config/raw results prevents performance claims |
| 2. Finish telemetry | Backend + SRE | W0/W1 fixes, resource tests and G1 record | Unexpected loss, memory growth or latency regression: restore explicit prior trace/log settings within measured DB capacity |
| 3. Add API capability | Backend + Frontend | Cursor/count and bounded dashboard contracts, Modern translations, legacy differential tests | Contract regression: disable new capability; keep legacy clients operational |
| 4. Install projection schema | Database Operations | Dry run, additive DDL, plans, backups and lock budgets | Lock/disk budget exceeded: stop migration and follow recovery procedure |
| 5. Enable capture in shadow | Backend | All-writer readiness fence, synchronous invalidation, lag metrics | Capture errors/contention: disable projection trust, stop activation, preserve ledger behavior |
| 6. Backfill and compare | Backend + SRE | Current-first progress, historical gaps, six-dataset equality at documented boundaries | Divergence: hold cutover and repair bounded units |
| 7. Cut over opt-in reads | SRE + Frontend | Canary projection reads, response/freshness budgets and stale UI | Serve last verified generation or unavailable; never unrestricted high-volume raw fallback |
| 8. Optional telemetry/mirror | Backend + SRE | Separate G3/G4 records, replay and outage runbooks | Disable affected external read/export feature; preserve committed replay backlog |
| 9. Enable explicit retention | Database Operations + product owner | W2.6 signed history policy and eligibility/restore evidence | Missing checkpoint, unresolved usage or insufficient catch-up: pause deletion |
| 10. Publish capacity | SRE | G5 workload/topology report with limitations | Any failed load/storage/recovery gate: publish partial evidence only |

Turning batching off restores a more expensive SQL path; confirm spare database capacity
or reduce admission before rollback. Turning projections off at high volume must select a
safe stale/unavailable path rather than restore unbounded raw scans. Retain additive schema,
control state and replay data for rollback. Index rebuilding, deleted-history restoration
and mixed-version writer rollback require their own procedures.

Suggested code ownership keeps responsibilities separate: mutation capture and projection
state in focused `model/` files, workers with explicit lifecycle wiring, query readers apart
from capture, optional analytics packages isolated from the ledger writer, and Modern UI
capability handling apart from legacy DTO conversion. Do not change every adapter merely
to route usage through a new generic telemetry interface.

Before starting each phase, resolve its bounded tuning decisions in the implementation PR:
source-shard count and lock budget, projection storage/retention horizons, worker/read
budgets, chosen database migration procedure, authority horizon, and optional exporter or
ClickHouse version. Record measurements and tradeoffs there; changing a tuning value cannot
relax the correctness/compatibility contracts in sections 2 and 5.

## 11. Expert-review disposition and source map

| Expert concern | Resolution in this plan |
| --- | --- |
| Contradictory compatibility/configuration | Sections 2–3 are authoritative; implemented/planned settings and valid combinations separated |
| Billing writes treated as telemetry | Synchronous usage path retained; future batching contract isolated in 2.3 |
| Time-only rollups and stale ownership | Transactional changed buckets, generation-specific acknowledgment and SQL publication fencing in W2.2 |
| Raw current-period dashboard scans | Projection-only optimized reads and explicit coverage/response budgets in W2.3 |
| Lost endpoint semantics / cardinality explosion | Field-level six-result mapping and query-shaped projection families in W2.1 |
| Breaking pagination and dishonest counts | Additive cursor capability and explicit count quality in W2.4 |
| Unsafe mirror fan-out and summing revisions | Durable outbox, canonical event revisions and controlled rebuilds in W4 |
| Missing authoritative history | Retained SQL authority; separate archive gate before shortening it in W2.6 |
| Trace sampling/outcome/memory/flush gaps | Focused W1 release requirements, including actual middleware order |
| Disk survival and incompatible log bridge | Writer-aware limits/emergency policy in W0; tested fork adapter in W3 |
| Retention/index migration assumptions | Explicit catalog/plan/lock/recovery work in W2.5 and section 8 |
| Unsupported capacity conclusions | Correct arithmetic, narrow historical evidence and open-arrival full-relay G5 |

Local sources for implementation and acceptance:

- `model/log.go`, `dto/log_statistics.go`, `controller/user.go`, `controller/log.go`:
  mutable usage, six aggregate contracts, list filters, dates and serialization.
- `model/utils.go`: existing quota batch-updater failure and shutdown behavior.
- `model/compact_uuid_election.go`, `model/compact_uuid_coordinator_state.go`:
  reusable lock mechanics and limits of current ownership checks.
- `common/config/observability.go`, `common/config/validation.go`,
  `common/config/compat_test.go`: implemented defaults and configuration checks.
- `common/tracing/`, `middleware/tracing.go`, `main.go`, `common/telemetry/telemetry.go`:
  recorder/sinks, actual middleware order, exporter and shutdown lifecycle.
- `common/logger/`, `model/retention_chunk.go`: sampling, active-file limitations and
  engine-specific bounded deletion.
- `controller/user_dashboard_cache.go`, `model/user_stats_cache.go`: existing cache behavior.
- [Benchmark record](../benchmarks/20260905_observability-phase0-phase1.md): historical
  commands, reported tables and limitations; does not replace a release evidence bundle.

External technical references are linked beside the specific claims they support above.
The plan intentionally makes no sizing claim from another gateway's deployment guide.
