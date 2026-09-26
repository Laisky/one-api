# Web API E2E performance

This opt-in harness targets the actual HTTP endpoints used by Modern dashboard and logs pages. It starts the real gateway, obtains its migrated schema, populates a private deterministic SQLite fixture, and checks complete response values before trusting timing. It never contacts an upstream model provider. It does not measure browser rendering, WAN/TLS, production capacity, or native PostgreSQL/MySQL query performance.

## Run and audit

Build immutable main-baseline and index-candidate binaries with the same Go version, dependencies and frontend/embed inputs; do not time an uncommitted or mixed-version pair. Prepare the existing two tokenizer assets with `tests/stream-perf/cache_tokens.py` in a network-enabled preparation step. Pass their local directory explicitly. No credentials or production database are required.

```sh
python3 tests/web-perf/run.py \
  --baseline /absolute/path/one-api-baseline \
  --candidate /absolute/path/one-api-candidate \
  --token-cache /absolute/path/token-cache \
  --output /absolute/path/new-web-results
python3 tests/web-perf/report.py /absolute/path/new-web-results
```

Defaults implement the registered independent recovery confirmation: 200,000 rows over90 days,32 users,80% heavy-user history,1,024 UTF-8 content bytes/row. Four targets (self dashboard, first/deep legacy log pages, explicit cursor first page), four controls (site dashboard, administrator list/stat, user self), concurrency1/8, five alternating variant pairs. Each persistent connection validates two warm-up requests before a shared start barrier; targets then issue128 requests and controls512. The full matrix has160 endpoint trials/51,200 timed responses. Warm-ups, first observations and startup are stored separately. They are not a flushed-OS-cache experiment.

The registered gate requires >=20% paired-median P95 improvement in at least two personal targets with at least4/5 favorable pairs. It rejects control P95 regressions exceeding BOTH10% and5ms, >20% gateway RSS growth, any invalid/missing response, or incomplete/unqualified variants. Medians and changes are computed from independent runs, not pooled requests. The auditor rechecks every persisted response-observation count/index, nearest-rank latency distribution, per-worker warmup and exact summary. It cannot reconstruct private HTTP payloads from timing JSON; the independent payload oracles run in the actual request path. Authentication values, database files and private gateway logs are never output artifacts.

## Scope and fidelity

The fixtures use runtime-generated per-user access tokens with disabled password login, loopback-only API listening, GOMAXPROCS2 and default logging/GC/dashboard cache/SQLite settings. The existing global API limiter remains enabled with a10,000,000 ceiling to prevent the load test becoming a rate-limit test. `LOG_CURSOR_ENABLED=true` is an explicit identical fixture opt-in; the product default remainsOFF. Existing bounded cursor counts may be exact, lower-bound or unavailable with their original provenance semantics. Legacy totals must be exact. No response projection, cache, timeout or admission optimization is introduced.

Every measured response checks the complete log DTO and expected page/order/ownership, or all six dashboard aggregate groups and quota/status fields. Independent negative calls verify auth, denied administrator paths, user scope, invalid dashboard ranges, filtered counts, empty shape and cross-principal cursor rejection. Post-timing committed fixture writes must become visible to legacy rows/counts and dashboard aggregates. This is read-API freshness, not an audit of the production writer or financial ledger. Legacy equal-timestamp ordering remains governed by its existing SQL (no new tie-order guarantee); cursor ordering keeps its explicit `(created_at,id)` contract.

HTTP latency spans sending the request through reading its complete bounded body, before JSON and oracle work. Throughput includes client validation and scheduling, so it is not server-only capacity. Gateway CPU and20-ms sampled RSS are separate from the Python client; short CPU samples have clock-tick quantization. The sequential endpoint order, closed-loop load, shared host and changing page caches must be considered when interpreting results. Runtime defaults remain intact; startup migration/UUID work can warm data before measurement. `schema.query_plan` is obtained with the host Python SQLite reader and labels that version; native Go migration tests separately check the actual application's access path. First-hit results are not cold-storage latency.

## Tests and recovery

`go test ./tests/web-perf` invokes the pure fixture/oracle/audit contracts plus a tiny local HTTP transport test. It does not run the full performance matrix or use timing thresholds as CI gates. The production-index migration/ordering test lives in `model/log_user_time_index_test.go`, including available native PostgreSQL/MySQL CI databases.

`run.py` refuses an existing output directory, writes an atomic incomplete summary after every endpoint, and reaps owned processes. Never splice an interrupted run together with another binary/machine or replace unfavorable cells. The earlier32-request startup-inclusive experiment is a separate publicly recorded rejection; this recovery study must not be presented as its raw continuation. See PLAN.md and RECOVERY.md. No new remote branches or workflows are needed.
