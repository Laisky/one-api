# W0/W1 acceptance evidence: containment bounds, recorder memory, and trace lifecycle

- Date: 2026-09-08
- Proposal: [Observability Data Tiering](../proposals/20260905_observability-data-tiering.md), §4 (W0 and W1)
- Gate: **G1**, the "W0 disk/cache limits, W1 semantic outcomes/memory/flush/OTLP order" portion.
  This bundle does **not** discharge G1's compatibility or config-matrix portions, and it
  closes no part of G5.
- Baseline tree (**before**): commit `0b701d25` — carries the Phase 0/1 telemetry release but
  none of the W0/W1 remediation
- Measured tree (**after**): `9248cd2f67794357583cf10c8f2798dbe69f72b7`
  (a `git stash create` snapshot of the working tree; the work is not committed)

> **Post-measurement hardening:** maintainer review subsequently changed
> retention batches to delete exact located primary keys, added stricter
> recorder/configuration bounds, and strengthened shutdown and logger lifecycle
> handling. Regression tests validate those behaviors, but the component timing
> and statement-count figures below remain historical measurements of the named
> snapshot and must be re-measured before being attributed to the current tree.

Companion records: [Phase 0/1](20260905_observability-phase0-phase1.md) and
[W2.4 cursor plans](20260906_w24-cursor-plans.md). This one follows their conventions:
every number carries the command that produced it and the limit that bounds it.

---

## 1. Claims and verdicts

| # | Claim under test | Before | After | Verdict |
| --- | --- | --- | --- | --- |
| 1 | A flush never materializes the whole queue in one slice | **50,000 rows / 39.6 MB** pinned in one writer | **500 rows / 396 KB** | **Supported** (100x) |
| 1b | A large `TRACE_BATCH_SIZE` cannot exceed the driver's parameter ceiling | 5,000-row statement: **0 of 5,000 written**, `too many SQL variables` | 1,365-row statements: **5,000 written** | **Supported** |
| 2 | Per-active-recorder bytes are bounded | tool loop **524,600 B** | **33,080 B** (scaled) | **Partly supported** — see 2b |
| 2b | Retained *string bytes* are bounded | 1 MiB URL retained **1,056,944 B** behind an 8,192-byte value | **8,373 B**, flat from 64 KiB to 1 MiB of input | **Supported after fix** (126x) — defect found here, fixed, re-measured (§4.2) |
| 2c | Admission converts an unbounded active set into a ceiling | no ceiling | 200,000 x measured worst case | **Supported as arithmetic only** |
| 3 | Candidate-location work is bounded, not just deleted rows | PG **646,400** rows examined | PG **20,002** | **Supported** (32.3x PG / 18.5x MySQL, steady state) |
| 3b | The rewrite is faster in every case | — | — | **Not supported** — 4% slower on SQLite and 11% slower on PostgreSQL backlog sweeps; MySQL neutral |
| 4 | Concurrent cold dashboard misses are coalesced, Redis or not | **256** computations for 256 viewers | **1** | **Supported** (256x) |
| 4b | The concurrency budget is free | — | — | **Not supported** — 32 distinct scopes take **26 ms → 414 ms** at budget 2 |
| 5 | Disk pressure is detected on a survival cadence | worst case **24 h** (standalone) / **1 h** (scaled) | **5 s** | **Supported** (arithmetic; guard sample costs 14.6 µs) |
| 6 | An error storm is bounded, which level escalation never did | **254–257 MB/s** written | **0.83–0.90 MB/s** | **Supported** (284–310x) |
| 7 | One SERVER span per request | **2** spans | **1** span | **Supported** (cited test, re-measured here) |
| 8 | The new bounds did not cost throughput | batched 28.2 / 31.7 µs per request | 28.0 / 29.2 µs | **Supported end to end**; see 8b |
| 8b | The in-memory recorder path is unchanged | **631.4 ns** | **664.9 ns** | **Regression: +5.29 %, p=0.007** |

Two findings below were defects rather than results, and **both have since been fixed and
re-measured**. §4.2 was the important one: the retained-string bound clipped length without
releasing memory, so the W1 requirement "bound […] retained string lengths" was met in
*length* and not in *bytes*; `clipString` now copies, and retention is flat in input size.
§10.3 was a smaller latent one (`maxRowsPerStatement` read a process-global dialect flag
rather than the owning handle's); the ceiling now comes from `model.TraceMaxRowsPerStatement`,
which reads the dialect of the handle that actually writes traces.

Finding a defect is the point of measuring. Both are recorded here in full — the original
measurement, the fix, and the post-fix numbers — rather than being quietly edited away.

---

## 2. Environment, and how to reproduce

### 2.1 Machine and toolchain

| | |
| --- | --- |
| CPU | AMD Ryzen 7 5700G with Radeon Graphics, 16 threads (1 thread/core, 16 vCPU exposed) |
| RAM | 27 GiB |
| Kernel | Linux 6.8.0-139-generic, x86_64 |
| Go | `go1.27.1 linux/amd64` |
| PostgreSQL | 17.11 (`postgres:17-alpine`), digest `sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73`, image id `sha256:1bea307dfb3ee30541a7acf7de14b58bcd6948da98e5d31a04c627c4d35ec64b` |
| MySQL | 8.4.11 (`mysql:8.4`), digest `sha256:b3b90af2a6552ae30c266fdb7d5dd55f3afb72404bb78d37fe8a23eb857fd3fb`, image id `sha256:bced325a4ab7aec848f4688371c7433351dcb5dba26fbcc29c67727d898ae5cb` |
| SQLite | file-backed, `WAL` + `synchronous=NORMAL`, matching `model.openSQLite` (via `common/benchdb`) |

Both containers ran on loopback with default server settings — **not** tuned, and not the
`shared_buffers`/`innodb-buffer-pool-size` settings the W2.4 record used. The 200,000-row
fixture fits in either engine's default cache, so every database figure below is a
**warm-cache** figure.

### 2.2 Effective configuration

Printed from the built binary's `common/config` package with no environment set:

```
ObservabilityProfile=standalone
TraceQueueSize=20000 TraceBatchSize=500 TraceWriterCount=2 TraceFlushIntervalMs=1000
TraceBatchMaxBytes=8388608 TraceMaxRecordBytes=262144 TraceMaxExternalCalls=1024 TraceMaxActiveRecorders=200000
LogMaxActiveFileSizeMB=4096 LogDiskCheckIntervalSec=5 LogEmergencyMaxBytesPerSec=1048576 LogDiskRecoveryMarginPct=20
DashboardMaxConcurrentAggregates=0 DashboardCacheTTLSec=0
RetentionDeleteBatchSize=5000 RetentionDeletePauseMs=10 RetentionSweepIntervalMinutes=1440
```

`TRACE_QUEUE_SIZE` is **20000 on standalone and 50000 on scaled**; `TRACE_WRITER_COUNT` is
2 and 4. §3 uses 50000 because that is the figure the proposal names, so its "before"
number is the **scaled** worst case, not what a standalone operator had.

### 2.3 Commands

```sh
docker run -d --name oneapi-w01-pg -e POSTGRES_PASSWORD=... -e POSTGRES_DB=oneapi_bench \
  -p 127.0.0.1:15442:5432 postgres:17-alpine
docker run -d --name oneapi-w01-mysql -e MYSQL_ROOT_PASSWORD=... -e MYSQL_DATABASE=oneapi_bench \
  -p 127.0.0.1:13316:3306 mysql:8.4

export ONEAPI_BENCH_PG_DSN='host=127.0.0.1 port=15442 user=... password=... dbname=oneapi_bench sslmode=disable'
export ONEAPI_BENCH_MYSQL_DSN='...:...@tcp(127.0.0.1:13316)/oneapi_bench?charset=utf8mb4&parseTime=True&loc=Local'

# 1, 1b, 7 -- always available, no container, no opt-in
go test ./common/tracing/ -run 'TestMeasureFlushLocalDrainFootprint|TestMeasureStatementParameterCeiling|TestMeasureOTLPSpansPerRequest' -v -count=3

# 2, 2b, 2c -- opt-in: the unbounded arms retain gigabytes
ONEAPI_MEASURE=1 go test ./common/tracing/ -run 'TestMeasureActiveRecorderMemory|TestMeasureAdmissionCeiling|TestMeasureClippedStringRetention|TestMeasureClippedExternalCallStringRetention' -v -count=3 -timeout 25m

# 3 -- opt-in: seeds 200000 rows per arm per engine
ONEAPI_MEASURE=1 go test ./model/ -run TestMeasureRetentionCandidateScan -v -count=3 -timeout 90m

# 4
go test ./controller/ -run 'TestMeasureDashboardMissCoalescing|TestMeasureDashboardConcurrencyBudget' -v

# 5 and the emergency hot path
go test ./common/logger/ -run TestMeasureDiskGuardReactionTime -v
go test ./common/logger/ -run XXX -bench BenchmarkMeasureEmergencyGateHotPath -benchmem -count=6

# 6 -- opt-in: a 3-second CPU-saturating storm per arm
ONEAPI_MEASURE=1 go test ./common/logger/ -run TestMeasureErrorStormContainment -v

# 8 -- cross-tree, byte-identical benchmark files
git worktree add --detach /tmp/oneapi-w01-base 0b701d25
go test ./common/tracing/ -run XXX -bench 'BenchmarkRecorderRequestLifecycle|BenchmarkNewTraceRow' -benchmem -count=8
( cd /tmp/oneapi-w01-base && go test ./common/tracing/ -run XXX \
    -bench 'BenchmarkRecorderRequestLifecycle|BenchmarkNewTraceRow' -benchmem -count=8 )
go test ./middleware/ -run XXX \
    -bench 'BenchmarkTracePipeline/(sqlite|postgres)/(floor_no_tracing|batched)$' \
    -benchtime=2000x -count=6 -timeout 90m
( cd /tmp/oneapi-w01-base && go test ./middleware/ -run XXX \
    -bench 'BenchmarkTracePipeline/(sqlite|postgres)/(floor_no_tracing|batched)$' \
    -benchtime=2000x -count=6 -timeout 90m )
benchstat before.txt after.txt

# admission contention, isolated
go test ./common/tracing/ -run XXX -bench BenchmarkMeasureRecorderAdmissionParallel -benchmem -count=6
```

`ONEAPI_MEASURE=1` gates the three heavy measurements, following the `ONEAPI_W24_PLANS`
convention: the scenarios stay in the repository — the proposal forbids ad-hoc one-off
scripts — without putting a 2 GB allocation or a 6-second storm on every `go test ./...`.
Without any DSN variable the retention measurement runs SQLite only.

Credentials are redacted above. No raw output in this record contains a DSN, token,
customer identifier or request content; the retention fixtures are generated.

### 2.4 How each "before" arm is reconstructed

Every "before" number below comes from one of three methods, named at each measurement:

1. **Configure the new bound to its old value.** §3 sets the batch's row ceiling to the
   queue capacity and its byte ceiling to `MaxInt64`, which makes the shipped `fill()`
   execute the identical loop the deleted `takeQueued` did. §8's admission arms set
   `TRACE_MAX_ACTIVE_RECORDERS=0`.
2. **Re-run the deleted code path.** §5's `legacyChunkedSweep` is the pre-remediation
   `ChunkedDelete` body, built from the same `boundedDeleteStatement` the shipped code
   still uses; §6's `legacyResolveDashboardAggregates` is the pre-remediation
   `resolveDashboardAggregates` body calling the same cache helpers; §9's before arm
   calls `emitStandaloneRequestSpan`, the function the old sink called unconditionally.
3. **Cross-tree.** §10 runs `BenchmarkRecorderRequestLifecycle`, `BenchmarkNewTraceRow`
   and `BenchmarkTracePipeline` in a worktree at `0b701d25`. `middleware/tracing_pipeline_bench_test.go`
   is **byte-identical** between the trees (verified with `diff`), and the recorder
   benchmark bodies are identical.

Where a before-state is not reconstructible it is said so: §4.2's URL bound is a
**constant**, not a setting, so the unbounded arm builds the `Recorder` struct directly
rather than pretending a configuration restores the old behaviour.

---

## 3. Bounded batch drain (W1, "SQL batching")

**Method.** A `sqlSink` queue of 50,000 realistically shaped rows — query string, five
lifecycle timestamps, two external-call entries — is drained once by `fill()` in each arm.
The per-row heap cost is measured separately by allocating 50,000 rows and taking the
`runtime.MemStats.HeapAlloc` delta across two collections, minus the slice header, so it
includes real allocator size-class rounding.

**Raw output** (identical across three runs):

```
measured: 792 bytes retained per queued trace row (50000 rows, 39631912 bytes total heap)
arm="unbounded (pre-remediation takeQueued: whole queue in one slice)" peak_rows=50000 accounted_bytes=43450000 pinned_row_heap_bytes=39600000 slice_bytes=400000 left_queued=0 drain_wall=2.449851ms
arm="bounded default (TRACE_BATCH_SIZE=500, TRACE_BATCH_MAX_BYTES=8MiB)" peak_rows=500 accounted_bytes=434500 pinned_row_heap_bytes=396000 slice_bytes=4000 left_queued=49500 drain_wall=21.53µs
```

| | unbounded (before) | bounded default (after) | ratio |
| --- | --- | --- | --- |
| Peak rows in one flush-local slice | **50,000** | **500** | 100x |
| Sink's own accounted bytes | 43,450,000 | 434,500 | 100x |
| Measured heap pinned by those rows | **39,600,000 B (39.6 MB)** | **396,000 B** | 100x |
| Slice pointer array | 400,000 B | 4,000 B | 100x |
| Rows left queued after one drain | 0 | 49,500 | — |
| Wall time of one drain | 1.99–2.45 ms | 19.9–21.5 µs | — |

**Conclusion.** The proposal's specific concern — "a 50000-record slice materialized in
one writer" — measures at **39.6 MB of trace records pinned by a single writer**, and
the bound reduces it to 396 KB. At the scaled default of four writers the pre-remediation
worst case is bounded above by the queue contents (39.6 MB shared), not 4x that; after
the change it is 4 x 396 KB = 1.58 MB.

**Limits.** 792 B/row is for *this* row shape. A row with a longer URL or more external
calls costs more, and the sink's own accounting (`estimateTraceRowBytes`) charges 869 B
for the same row — 10% conservative, which is the correct direction for a budget. The
drain wall times are single executions on a warm channel and are not latency figures.
This measures the writer's buffer, **not** the queue itself: `TRACE_QUEUE_SIZE=50000`
still holds up to 39.6 MB of rows, which nothing in W1 changes.

### 3.1 Per-statement parameter ceiling

The pre-remediation writer passed `TRACE_BATCH_SIZE` straight to `model.InsertTraces` as
the rows-per-statement.

```
measured: maxRowsPerStatement()=1365 rows on SQLite, 2730 rows on MySQL/PostgreSQL (24 parameters charged per row; driver ceilings 32766 and 65535)
arm="pre-remediation (TRACE_BATCH_SIZE used verbatim)" rows_per_statement=5000 written=0 err=failed to insert 5000 of 5000 trace rows: too many SQL variables
arm="bounded (min(TRACE_BATCH_SIZE, parameter ceiling))" rows_per_statement=1365 written=5000 err=<nil>
```

An operator raising `TRACE_BATCH_SIZE` to 5,000 on SQLite lost **every** trace, not some.
The bound splits instead. At the default batch size of 500 the ceiling never binds, so
this changes nothing for a default deployment.

---

## 4. Recorder memory (W1, "Recorder memory" and "Active requests")

**Method.** A population of live, unfinished `Recorder`s is built and held, and the
`HeapAlloc` delta across two collections is divided by the population. "Unbounded"
constructs the struct directly with the untrimmed URL and raises
`TRACE_MAX_RECORD_BYTES` / `TRACE_MAX_EXTERNAL_CALLS` to 2^30.

**Raw output** (three runs agreed to within 4 bytes per recorder):

| Request shape | unbounded (before) | standalone bound | scaled bound |
| --- | --- | --- | --- |
| ordinary relay request (72 B URL, 2 external calls) | **504 B** | **504 B** | **504 B** |
| tool loop (72 B URL, 5,000 external calls) | **524,600 B** | **106,808 B** | **33,080 B** |
| hostile input (256 KiB URL, 5,000 calls with 4 KiB strings) | **790,728 B** | **299,208 B** | **274,632 B** |

The ordinary shape is identical in all three arms: **the bounds cost an ordinary request
nothing and change nothing about it.** That is the most important row in the table, and it
is the one that makes the arithmetic below meaningful.

**Arithmetic projection to the proposal's scenario.** These are *calculations* from the
measured per-recorder figure — nothing here ran 600,000 concurrent requests:

| Shape and profile | measured B/recorder | x 600,000 active | |
| --- | --- | --- | --- |
| ordinary, any profile | 504 | **302,400,000 B ≈ 288 MiB** | the realistic case |
| tool loop, unbounded | 524,600 | 314,760,000,000 B ≈ 293 GiB | before |
| tool loop, scaled bound | 33,080 | 19,848,000,000 B ≈ 18.5 GiB | after |
| hostile, scaled bound | 274,632 | 164,779,200,000 B ≈ 153 GiB | **after — see §4.2** |

Admission ceiling, also arithmetic: `TRACE_MAX_ACTIVE_RECORDERS=200000` x the measured
worst-case bounded recorder of 274,632 B = **54.9 GB (51.15 GiB)**. With the defect in
§4.2 fixed this ceiling would fall to roughly 200,000 x 65,536 = 13.1 GB.

### 4.2 DEFECT (found here, since FIXED): the retained-string bound clipped length, not memory

`clipString` ends with `return s[:cut], true`. Slicing a Go string does not copy: the
result shares the original backing array, so the whole allocation stays reachable for the
recorder's lifetime. The bound therefore caps what is *stored and serialized*, and does
not cap what is *retained*.

Measured by varying only the input URL size, with every bound at its shipped default:

```
input_url_bytes=1024    clipped_url_len=1025  bytes_per_active_recorder=1328
input_url_bytes=8192    clipped_url_len=8192  bytes_per_active_recorder=9648
input_url_bytes=65536   clipped_url_len=8192  bytes_per_active_recorder=73904
input_url_bytes=262144  clipped_url_len=8192  bytes_per_active_recorder=270512
input_url_bytes=1048576 clipped_url_len=8192  bytes_per_active_recorder=1056944
```

`clipped_url_len` is correctly pinned at 8,192. `bytes_per_active_recorder` tracks the
**input** size, one-for-one, to at least 1 MiB. `TRACE_MAX_RECORD_BYTES` is 262,144 and
does not bind, because the accounting only ever charges `len()` of the clipped value.

The same holds for external-call strings, where each entry carries its own
upstream-derived value:

```
input_string_bytes=64    calls=4 clipped_string_len=64  accounted_call_bytes=1292 bytes_per_active_recorder=848
input_string_bytes=4096  calls=4 clipped_string_len=256 accounted_call_bytes=3596 bytes_per_active_recorder=16976
input_string_bytes=16384 calls=4 clipped_string_len=256 accounted_call_bytes=3596 bytes_per_active_recorder=66133
```

`accounted_call_bytes` is flat at 3,596 while measured retention rises 3.9x with the
input. At 16 KiB inputs the record retains **18.4x what the sink believes it retains**.

**Impact.** The bound still helps a great deal — the tool-loop shape drops 15.9x because
it bounds the *number* of entries, which is a real reduction. What it does not do is make
per-record bytes provable from untrusted input, which is exactly what W1 asked for: a
client sending a 10 MB request line pins 10 MB per in-flight request no matter what
`TRACE_MAX_RECORD_BYTES` says. `model.SanitizeTraceURL` bounds the *stored* value at
`Finish`, so nothing incorrect is persisted; the exposure is live memory for the request's
lifetime, multiplied by concurrency.

**FIXED after this measurement.** `clipString` now returns `strings.Clone(s[:cut])`, so a
clipped value owns its own storage and the original allocation becomes collectable. The
copy runs only when a value is over-long; the common path still returns `s` untouched and
allocates nothing. Re-running the same two measurements against the fix:

```
input_url_bytes=1024    clipped_url_len=1025  bytes_per_active_recorder=1330
input_url_bytes=8192    clipped_url_len=8192  bytes_per_active_recorder=8368
input_url_bytes=65536   clipped_url_len=8192  bytes_per_active_recorder=8371
input_url_bytes=262144  clipped_url_len=8192  bytes_per_active_recorder=8373
input_url_bytes=1048576 clipped_url_len=8192  bytes_per_active_recorder=8373
```

```
input_string_bytes=64    calls=4 clipped_string_len=64  accounted_call_bytes=1292 bytes_per_active_recorder=848
input_string_bytes=4096  calls=4 clipped_string_len=256 accounted_call_bytes=3596 bytes_per_active_recorder=3664
input_string_bytes=16384 calls=4 clipped_string_len=256 accounted_call_bytes=3596 bytes_per_active_recorder=3664
```

| Input | Retained before | Retained after | Reduction |
| --- | --- | --- | --- |
| 1 MiB URL | 1,056,944 B | **8,373 B** | 126x |
| 256 KiB URL | 270,512 B | **8,373 B** | 32x |
| 64 KiB URL | 73,904 B | **8,371 B** | 8.8x |
| 16 KiB external-call strings | 66,133 B | **3,664 B** | 18.1x |

Retention is now **flat** from 64 KiB to 1 MiB of input rather than linear in it, which is
the property W1 asked for. The accounting is also no longer a lie: measured retention
tracks `accounted_call_bytes` to within 2% (3,664 vs 3,596) where it was previously 18.4x
above it, so `TRACE_MAX_RECORD_BYTES` now bounds something real.

`TestMeasureClippedStringRetention` and `TestMeasureClippedExternalCallStringRetention`
remain in the tree as the regression evidence.

**Limits.** Heap accounting is `HeapAlloc` across `runtime.GC()`, so it includes allocator
rounding and any transient the collector had not yet reclaimed; the three runs agreeing to
single bytes bounds that error. Nothing here measures GC pressure, pause time, or the
behaviour of a real 600,000-recorder heap.

---

## 5. Retention candidate scan (W0.8)

**Method.** 200,000 `async_task_bindings` rows are seeded per arm, statistics refreshed
(`VACUUM ANALYZE` / `ANALYZE TABLE` / `ANALYZE`), then the table is swept twice from an
identical fixture: once by `legacyChunkedSweep` with the original `CASE` predicate and no
keyset, once by the shipped `CleanExpiredAsyncTaskBindingsStats`. `RETENTION_DELETE_PAUSE_MS`
is 0 so wall time reflects work rather than the configured yield (production default: 10 ms
per chunk). Both arms must delete exactly the same row count, asserted.

Rows examined comes from the engine, not from the harness:
`SHOW SESSION STATUS LIKE 'Handler_read%'` on MySQL (per session — the pool is pinned to
one connection), `pg_stat_user_tables.seq_tup_read + idx_tup_fetch` flushed with
`pg_stat_force_next_flush()` on PostgreSQL. **SQLite exposes no such counter**, so its rows
are reported as unavailable rather than estimated.

Two eligibility distributions are measured, because the distribution is the whole result:

- **backlog sweep** — 750/1000 rows eligible, eligibility correlated with `created_at`
  (the first sweep after enabling retention).
- **steady state** — 100/1000 eligible, eligibility **uncorrelated** with `created_at`.
  This is what an async-task table actually looks like: `last_accessed_at` is stamped on
  every poll, so a binding created yesterday and abandoned an hour later ages out while
  sitting late in `created_at` order.

**Raw output.** Rows examined and statement counts were **bit-identical across all three
runs**; wall time is given as its observed range with the median of the three *paired*
before/after ratios in bold, because pairing within a run is the only comparison the
run-to-run variation does not swamp.

| Engine | Mix | rows examined before | after | ratio | statements before → after | wall before → after |
| --- | --- | --- | --- | --- | --- | --- |
| postgres | **steady state** | **646,400** | **20,002** | **32.3x** | 9 → 15 | 102–112 ms → 56–63 ms (**1.72x**) |
| mysql | **steady state** | **646,411** | **35,006** | **18.5x** | 7 → 13 | 806–913 ms → 549–769 ms (**1.45x**) |
| sqlite | steady state | not exposed | not exposed | — | 5 → 11 | 606–628 ms → 577–599 ms (**1.05x**) |
| postgres | backlog | 970,000 | 234,516 | 4.1x | 35 → 66 | 443–459 ms → 460–525 ms (**0.89x**) |
| mysql | backlog | 970,063 | 290,014 | 3.3x | 33 → 64 | 4.33–4.93 s → 4.26–5.01 s (**1.02x**) |
| sqlite | backlog | not exposed | not exposed | — | 31 → 62 | 2.71–2.76 s → 2.83–2.90 s (**0.96x**) |

Rows deleted are identical in every cell (150,000 backlog / 20,000 steady state), and the
shipped sweep reports `passes=1 backlog=0` everywhere: one keyset walk drained the table.

### 5.1 Plans

PostgreSQL, steady state, old predicate — an index scan that filters:

```
Limit (actual time=0.152..7.723 rows=5000 loops=1)
  Buffers: shared hit=963
  ->  Index Scan using idx_async_task_bindings_created_at on async_task_bindings (actual time=0.151..7.435 rows=5000 loops=1)
        Filter: (CASE WHEN (last_accessed_at > 0) THEN last_accessed_at ELSE created_at END < '1788277502533'::bigint)
        Rows Removed by Filter: 45000
        Buffers: shared hit=963
Execution Time: 7.894 ms
```

PostgreSQL, steady state, new pass 1 — an index-only range on the composite:

```
Limit (actual time=4.984..5.482 rows=5000 loops=1)
  Buffers: shared hit=139
  ->  Sort (actual time=4.982..5.168 rows=5000 loops=1)
        Sort Key: created_at
        ->  Index Only Scan using idx_async_task_retention on async_task_bindings (actual time=0.024..1.862 rows=20000 loops=1)
              Index Cond: ((last_accessed_at > 0) AND (last_accessed_at < '1788277502533'::bigint))
              Heap Fetches: 0
              Buffers: shared hit=139
Execution Time: 5.733 ms
```

**963 buffers against 139**, and 45,000 rows discarded by a filter against 0.

MySQL 8.4, steady state, old predicate versus new pass 1:

```
-> Covering index scan on async_task_bindings using idx_async_task_retention  (actual time=0.0179..27.5 rows=200000 loops=1)
   -> Filter: ((case when (last_accessed_at > 0) then last_accessed_at else created_at end) < 1788277432034)  (actual time=0.0215..38.6 rows=20000 loops=1)
```

```
-> Covering index range scan on async_task_bindings using idx_async_task_retention over (0 < last_accessed_at < 1788277432034)  (actual time=0.0159..3.52 rows=20000 loops=1)
```

**200,000 rows scanned to find 20,000, versus 20,000 sought directly.**

SQLite (`EXPLAIN QUERY PLAN`, all three predicates):

```
CASE WHEN last_accessed_at > 0 THEN last_accessed_at ELSE created_at END < ?
  SCAN async_task_bindings USING INDEX idx_async_task_bindings_created_at

last_accessed_at > 0 AND last_accessed_at < ?
  SEARCH async_task_bindings USING COVERING INDEX idx_async_task_retention (last_accessed_at>? AND last_accessed_at<?)
  USE TEMP B-TREE FOR ORDER BY

(last_accessed_at IS NULL OR last_accessed_at <= 0) AND created_at < ?
  SEARCH async_task_bindings USING INDEX idx_async_task_bindings_created_at (created_at<?)
```

`SCAN` becomes `SEARCH` — the structural claim W0.8 rests on — and pass 2 falls back to
the `created_at` index with a per-row recheck, exactly as `async_task_retention.go`
documents.

### 5.2 Honest negative: the rewrite is slower on backlog sweeps and on SQLite

The keyset walk issues **one extra `SELECT` per chunk** to locate candidates, so statement
counts roughly double (31 → 62, 33 → 64, 35 → 66 on the backlog mix). On a **fully cached**
200,000-row table where the old plan already found its 5,000 victims in the first few
thousand index entries, that round trip costs more than the seek saves:

- SQLite backlog: **4% slower** (paired ratios 0.97 / 0.96 / 0.94 — stable and consistent)
- PostgreSQL backlog: **11% slower** (paired ratios 0.89 / 1.00 / 0.87)
- MySQL backlog: **neutral** (paired ratios 1.02 / 0.87 / 1.08 — the spread swamps the effect)

This is the same trade the Phase 0/1 record recorded for chunking itself: bounded work per
statement costs total throughput. It is worth taking because the *steady-state* case — the
one that runs every sweep interval forever — improves 18–32x in rows examined and 1.45–1.72x
in wall time, and because the rows-examined reduction is what protects a table that does
**not** fit in the buffer pool. It is recorded here rather than hidden.

**Limits.** Warm cache, 200,000 rows, default engine settings, no concurrent foreground
load (unlike §5 of the Phase 0/1 record, which offered 400 req/s against the swept table).
Nothing here shows steady-state catch-up against arrivals, which is §8.3 of the proposal
and remains open. `Candidates` (the rows the keyset probes located: 140,000 backlog /
15,000 steady state) counts probe output, not engine rows read; the engine counters above
are the authority.

---

## 6. Dashboard miss coalescing (W0.6)

**Method.** N goroutines are released simultaneously against one cache key. The six
aggregate queries are replaced by a stub that counts executions, tracks peak concurrency,
and costs a fixed 25 ms — the site-wide aggregate measured at 25.1 ms over 100,000 users in
[§7 of the Phase 0/1 record](20260905_observability-phase0-phase1.md). A free stub would
make every arm look identical. The before arm is the pre-remediation
`resolveDashboardAggregates` body; the after arm is the shipped
`resolveDashboardAggregatesForKey`.

| Redis | viewers | computations before | computations after | peak concurrent after | wall before → after |
| --- | --- | --- | --- | --- | --- |
| unavailable | 1 | 1 | 1 | 1 | 26.1 ms → 25.8 ms |
| unavailable | 4 | 4 | **1** | 1 | 26.5 ms → 25.8 ms |
| unavailable | 16 | 16 | **1** | 1 | 25.6 ms → 25.9 ms |
| unavailable | 64 | 64 | **1** | 1 | 25.9 ms → 25.8 ms |
| unavailable | 256 | **256** | **1** | 1 | 27.3 ms → 26.6 ms |
| available | 256 | **256** | **1** | 1 | **60.7 ms → 27.1 ms** (2.24x) |

**Conclusion.** Coalescing is unconditional and does not consult cache availability, which
is what the proposal required: with Redis absent the TTL cache is disabled outright, so
in-process coalescing is the only protection, and it delivers **1 computation for 256
concurrent cold viewers** there exactly as it does with Redis present.

**Limits.** The wall-clock column is not the point and should not be read as a latency
result: with a stub that sleeps, N concurrent computations finish in about one stub's time
regardless of arm. The result is the **computation count** — the load that reaches the
database. Whether 256 real six-query bundles would finish in 27 ms on a real database is a
question this measurement cannot answer, and §7.1 of the Phase 0/1 record shows they do
not (121 ms for 32 uncoalesced cold viewers against PostgreSQL).

### 6.1 The concurrency budget bounds distinct scopes, and costs latency to do it

Coalescing bounds one key. Thirty-two viewers of thirty-two *different* windows are
thirty-two different keys, and only the budget bounds those:

```
budget=0 (unlimited (standalone default)) distinct_scopes=32 aggregate_computations=32 peak_concurrent=32 refused=0 wall=26.449542ms
budget=2 (bounded)                        distinct_scopes=32 aggregate_computations=32 peak_concurrent=2  refused=0 wall=413.753034ms
budget=8 (bounded)                        distinct_scopes=32 aggregate_computations=32 peak_concurrent=8  refused=0 wall=101.652006ms
```

Peak concurrency is bounded exactly as configured and nothing is refused inside the wait
budget — but the wall time for the herd rises **15.6x at budget 2** and 3.8x at budget 8.
That is load shedding working as designed, and it is the reason the standalone default is
0 (unlimited): an upgrade must not introduce a queue nobody asked for. Operators enabling
`DASHBOARD_MAX_CONCURRENT_AGGREGATES=2` on the scaled profile are trading dashboard
latency for database protection, and this is the size of that trade.

---

## 7. Disk-pressure reaction time (W0.3)

The latency figures are **arithmetic over configured intervals**, not measurements: a
24-hour worst case cannot be observed in a test. The guard's per-sample cost is measured.

```
measured: one disk-pressure sample costs 14.601µs (2000 samples, one log file present)
profile=standalone worst_case_detection_before=24h0m0s worst_case_detection_after=5s improvement=17280x
profile=scaled     worst_case_detection_before=1h0m0s  worst_case_detection_after=5s improvement=720x
profile=external   worst_case_detection_before=1h0m0s  worst_case_detection_after=5s improvement=720x
```

Before, the free-disk floor was sampled on the retention sweep's ticker
(`RETENTION_SWEEP_INTERVAL_MINUTES`, 1440 standalone / 60 scaled). After, it runs on
`LOG_DISK_CHECK_INTERVAL_SEC=5`.

Headroom arithmetic against a 1 GiB reserve, recomputed here rather than quoted:

| write rate | time to exhaustion | guard samples available **after** | guard samples available **before** (standalone) |
| --- | --- | --- | --- |
| 1 MiB/s | 1,024 s | 205 | 0.0119 |
| 16 MiB/s | **64.0 s** | 13 | 0.0007 |
| 64 MiB/s | 16.0 s | 3 | 0.0002 |

The proposal's "about 62.5 seconds" is the same figure in decimal units (1 GB / 16 MB/s);
64.0 s is the binary-unit version. Either way the standalone guard got **less than
one-thousandth of a sample** before the volume filled, which is the defect W0.3 names.

**Cost.** 14.6 µs every 5 seconds is 2.9 parts per million of one core. The measured
sample includes one `os.Stat` of the log directory and the (stubbed) free-space probe;
a real `statfs` on a busy filesystem costs more, and a directory holding many rotated
files costs more again — this figure is for a directory with one file.

**Limits.** The 17,280x and 720x are ratios of *configuration*, not of observed behaviour.
`TestPressureGuardSamplesFasterThanTheRetentionSweep` and
`TestRetentionSweepNoLongerRunsTheDiskGuard` are the behavioural evidence that the loops
are actually split; this section only prices the consequence.

---

## 8. Error-storm containment (W0.4)

**Method.** Four goroutines emit `ERROR` and `WARN` lines as fast as they can for a fixed
3-second wall window, through the production console encoder at `zapcore.WarnLevel` —
which is exactly what `escalateLogLevel` raised the process to — into a counting
`WriteSyncer`. The before arm has the emergency core installed but not engaged; the after
arm has it engaged for disk pressure at the shipped 1 MiB/s budget.

```
arm="before: escalateLogLevel only (level raised to WARN, no byte budget)" window=3s attempted_lines=5264384 written_lines=5264384 written_bytes=771232256 bytes_per_second=257077419 suppressed_lines=0
arm="after: bounded emergency policy (LOG_EMERGENCY_MAX_BYTES_PER_SEC=1048576)" window=3s attempted_lines=6531584 written_lines=17001 written_bytes=2490824 bytes_per_second=830275 suppressed_lines=6514583
```

Second independent run: before 762,981,376 B (254.3 MB/s), after 2,687,134 B (0.90 MB/s).

| | before | after | ratio |
| --- | --- | --- | --- |
| Lines written in 3 s | 5,208,064–5,264,384 | **17,001–18,341** | ~300x |
| Bytes written in 3 s | 762,981,376–771,232,256 | **2,490,824–2,687,134** | ~300x |
| Effective rate | **254–257 MB/s** | **0.83–0.90 MB/s** | 284–310x |
| Lines suppressed and counted | 0 | 6,514,583–6,847,067 | — |

**This is the point the proposal makes about escalation.** Raising the level to WARN
suppressed nothing at all here, because every line in the storm *is* WARN or ERROR. The
byte budget is what bounds it, and it lands just under its 1 MiB/s configuration (0.83–0.90
MB/s) because `estimateEntryBytes` charges slightly more than the encoder emits — the
conservative direction for a budget.

**Limits, and they matter.** The before arm's 257 MB/s is what the process *offered* to a
counting writer with no disk behind it. A real filesystem would apply back-pressure and the
achieved rate would be lower — so 257 MB/s is an **upper bound on offered volume**, not a
claim that a disk absorbs it. The ratio is still the result: at any achieved rate, the
before arm has no bound and the after arm has one at 1 MiB/s. Neither arm ran against a
genuinely full volume; `TestEmergencyWriterFailureIsCountedNotLogged` covers that path.

### 8.1 The bounded policy costs a healthy process nothing measurable

`BenchmarkMeasureEmergencyGateHotPath`, 6 runs, 16-way `RunParallel`
(ns/op here is reciprocal throughput, not per-call latency):

| arm | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `before_no_emergency_core` (no core installed) | 294.4–308.4 (median ~302) | 120–121 | 4 |
| `after_healthy` (core installed, disk fine) | 290.5–305.2 (median ~296) | 120 | 4 |
| `after_engaged` (policy active) | 258.7–276.5 | 64 | 1 |

The healthy arm is indistinguishable from having no core at all: the hot path is one
`atomic.Bool` load. The engaged arm is *cheaper* only because most lines are discarded
before encoding — it is not a cost comparison and must not be read as one.

---

## 9. OTLP spans per request (W1, "OTLP")

The correctness result is already established and is **cited, not redone**. Under the real
middleware order from `main.go` (`otelgin.Middleware` enclosing `middleware.TracingMiddleware`)
against `sdktrace/tracetest.NewInMemoryExporter`:

| Test | Establishes | Result |
| --- | --- | --- |
| `middleware.TestOtelginSpanIsLiveWhenTraceSinkSubmits` | the enclosing otelgin span is still recording when the sink runs, and the sink enriches it instead of starting a second one | PASS |
| `middleware.TestOtelginSpanCarriesRemoteParentAndErrorStatus` | remote parentage and error status survive on that one span | PASS |
| `middleware.TestTraceSinkStartsItsOwnSpanWithoutEnclosingSpan` | with no enclosing span (a synthetic gin context) the sink still emits one | PASS |

The export-volume consequence, measured here:

```
arm="before: sink starts its own SERVER span beside otelgin's" spans_per_request=2 attributes=5 events=4 attribute_payload_bytes=299
arm="after: sink enriches the enclosing otelgin span"          spans_per_request=1 attributes=5 events=1 attribute_payload_bytes=230
```

**Spans per request: 2 → 1.** That is one fewer span record, span id, parent link,
start/end timestamp pair, resource reference and scope reference per request handed to the
SDK batch processor.

**The event drop from 4 to 1 is a relocation, not a saving, and must not be quoted as one.**
`enrichActiveRequestSpan` deliberately does not replay the lifecycle events, because in
batched mode `RecordTraceStart` / `RecordTraceTimestamp` / `RecordTraceStatus` already added
each of them to the same span at the instant it happened. In a real request those events
are present; this harness does not run that middleware, so the after arm shows only the
external-call event the completed document adds.

**`attribute_payload_bytes` is a proxy, not a wire measurement.** It sums attribute keys,
emitted attribute values and event names. The OTLP protobuf transform is in an `internal`
package and cannot be called from a test, so no serialized-byte figure is available. The
299 → 230 figure also understates the real difference, because the otelgin stand-in here
carries none of the ~15 `http.*` attributes real `otelgin` attaches — those are paid once
in the after arm and (as a duplicated SERVER span alongside them) twice in the before arm.

**Unchanged, and stated explicitly:** reusing the otelgin span does **not** suppress that
span's own independent SDK export. Reducing OTLP volume still requires the separately
configured SDK/collector sampling policy, exactly as §4 of the proposal says.

---

## 10. Trace-pipeline overhead: did the bounds cost throughput?

### 10.1 End to end — no, within resolution

`BenchmarkTracePipeline`, `-benchtime=2000x -count=6`, 16-way `RunParallel`,
**byte-identical benchmark file in both trees**, `benchstat` medians:

| arm | before (`0b701d25`) | after | verdict |
| --- | --- | --- | --- |
| sqlite/floor_no_tracing | 14.85 µs ±41% | 13.35 µs ±33% | ~ (p=0.065) |
| sqlite/batched | 28.20 µs ±29% | 27.99 µs ±15% | ~ (p=1.000) |
| postgres/floor_no_tracing | 15.04 µs ±18% | 14.15 µs ±29% | ~ (p=0.818) |
| postgres/batched | 31.70 µs ±13% | 29.16 µs ±17% | ~ (p=0.240) |

Trace-attributable cost (arm − floor), the figure §3.2 of the Phase 0/1 record quotes:
SQLite 13.35 → 14.64 µs, PostgreSQL 16.66 → 15.01 µs. Both differences are far inside the
propagated spread and neither is separable.

Work-conservation metrics are unchanged, which is the stronger statement:

| metric | before | after | verdict |
| --- | --- | --- | --- |
| `pg_wal_B/op` (batched) | 737.3 ± 0% | 736.3 ± 0% | ~ (p=0.387) |
| `total_stmts/op` (batched) | 2.500m | 2.500m | identical |
| `rows_written` | 2,000 of 2,000 | 2,000 of 2,000 | identical |
| `trace_loss_pct` | 0 | 0 | identical |

**The bounds changed no durable work and lost no traces.** `postgres/batched`'s
`total_ns/op` (which includes the full drain) reads 71.91k → 59.33k, −17.49% (p=0.009);
that is reported as observed and **is not claimed as an improvement** — a 6-sample paired
comparison across two container-warmup states is not sufficient to attribute a 17% drain
improvement to these changes.

### 10.2 In-memory recorder — a real, small regression

`BenchmarkRecorderRequestLifecycle` and `BenchmarkNewTraceRow`, `-count=8`, cross-tree:

| benchmark | before | after | delta |
| --- | --- | --- | --- |
| `RecorderRequestLifecycle` sec/op | **631.4 ns ±1%** | **664.9 ns ±2%** | **+5.29% (p=0.007, n=8)** |
| `RecorderRequestLifecycle` B/op | 288.0 | 304.0 | +5.56% (p=0.000) |
| `RecorderRequestLifecycle` allocs/op | 8 | 8 | ~ |
| `NewTraceRow` sec/op | 1.636 µs ±3% | 1.657 µs ±3% | ~ (p=0.328) |

**+33.5 ns and +16 B per request**, statistically significant. The 16 bytes are the new
`admitted`, `retainedBytes` and `truncated` fields; the nanoseconds are the admission
counter, the byte accounting and the `clipString` length check. Allocation count is
unchanged, so nothing new allocates.

For scale: 33.5 ns against a relay request measured at 28–32 µs of gateway-attributable
wall time in §10.1, and against a real upstream call measured in hundreds of milliseconds.
At 10,000 RPS it is 0.34 ms of CPU per wall-clock second. It is a regression and it is
reported as one; it is not a throughput problem.

Isolated admission-counter cost, 16-way `RunParallel` (reciprocal throughput):

| arm | ns/op | allocs |
| --- | --- | --- |
| `TRACE_MAX_ACTIVE_RECORDERS=0` (unlimited: `Add(1)` + gauge) | **27.5–27.7** | 0 |
| `TRACE_MAX_ACTIVE_RECORDERS=200000` (`Load` + CAS loop) | **88.8–90.0** | 0 |

The bounded path is **3.25x** the unlimited one under maximum contention, because 16
threads contend on one `atomic.Int64` compare-and-swap. In the full single-goroutine
lifecycle benchmark the three configuration arms are indistinguishable (658–668 ns median
for unlimited, bounded-200000 and bounded-scaled alike), so this contention does not
surface at that scale. It is recorded because a single global counter is a real scalability
ceiling and a future measurement at much higher core counts should re-check it.

### 10.3 DEFECT (minor, since FIXED): the parameter ceiling read a process-global flag

`maxRowsPerStatement()` branches on `common.UsingSQLite`, a process-global set by
`model.InitDB` for the **primary** handle. Traces are written through the primary handle
(`model.traceDBWithContext` uses `model.DB`), so production is correct today. But §9.1 of
the proposal requires using "the owning handle's dialect", and this does not: the first
draft of the §3.1 measurement opened a SQLite handle without setting the flag and the sink
computed the 65,535-parameter MySQL/PostgreSQL ceiling, producing `too many SQL variables`
on every write. Any future change that moves `traces` onto `LOG_DB` would reintroduce
that failure silently. Recorded as a latent hazard, not a live bug.

**Fixed after this measurement.** The ceiling now comes from
`model.TraceMaxRowsPerStatement(parametersPerRow)`, which reads
`dialectName(traceDBWithContext(nil))` -- the dialect of the handle that actually writes
traces -- and falls back to the SMALLER (SQLite) ceiling when the handle is not yet
initialized, since a batch that is too small merely costs an extra statement while one
that is too large fails to prepare at all. The two placeholder ceilings now live in
`model` as `MaxStatementParametersSQLite` / `MaxStatementParametersDefault` and are no
longer duplicated in `common/tracing`, so the two copies cannot drift apart.

---

## 11. What none of this establishes

1. **No capacity claim follows from any of it.** Every number here is a component
   measurement. §9.2 of the proposal requires an open-arrival-rate full-relay harness with
   a controlled upstream simulator, offered/achieved/failed RPS reported separately,
   seeded retained data at production scale, and a one-hour run at target after warmup.
   None of that was run. **G5 remains open and is not advanced by this record.**
2. **`-benchmem` and `RunParallel` figures are not p99 latency.** Under
   `b.RunParallel` with 16 goroutines, `ns/op` is wall clock per operation, i.e. the
   reciprocal of throughput; mean service time is roughly 16x larger, and no percentile
   of anything is reported anywhere in this document.
3. **The trace-attributable p99 goal is untested.** The proposal's initial goal is
   trace-attributable p99 overhead ≤ 2 ms against the no-tracing arm. §10.1 measures mean
   reciprocal throughput at 2,000 requests per iteration; it says nothing about a tail.
4. **Database numbers are warm-cache, untuned, single-node and unloaded.** §5 ran with no
   concurrent foreground traffic against the swept table, on 200,000 rows that fit in any
   default buffer pool, on default `postgres:17-alpine` and `mysql:8.4` settings. The
   Phase 0/1 record's caveat still applies verbatim: delete throughput on a cached fixture
   is an upper bound on steady-state capacity, not an estimate of it.
5. **Retention catch-up is not measured.** §8.3 of the proposal requires sustained sweep
   throughput greater than eligible arrivals under concurrent load. §5 measures a single
   drain of a static fixture. The shipped sweep reports `backlog=0 passes=1` in every cell,
   which means it never exercised the multi-pass catch-up path at all.
   `TestChunkedDeleteCatchesUpWithConcurrentInserts` covers that path functionally.
6. **Memory projections are arithmetic.** Every "at 600,000 active" figure in §4 is a
   measured per-recorder cost multiplied by a count. No process here held 600,000
   recorders, ran a GC at that heap size, or measured pause time or RSS.
7. **The OTLP export-volume figure is a proxy.** No serialized OTLP byte count was
   obtained (§9), and the event-count difference is a relocation, not a reduction.
8. **The storm's "before" rate is offered, not achieved.** §8's 257 MB/s went to a
   counting writer, not a filesystem.
9. **SQLite rows-examined is absent, not zero.** §5's `rows_examined=0` for SQLite means
   the engine exposes no counter; the plans and wall times are the SQLite evidence.
10. **Nothing here tests compatibility, the config matrix, mixed-schema operation, or
    shutdown ordering.** Those are separate G1 requirements covered by the correctness
    suites named beside each section, not by this record.

---

## 12. G1 gate status

| G1 requirement (§9 of the proposal) | Status after this record |
| --- | --- |
| W0 disk limits: active-file cap, fast pressure cadence, bounded emergency policy | **Measured** — §7, §8. Cadence 24 h/1 h → 5 s; storm 257 MB/s → 0.85 MB/s |
| W0 cache limits: coalescing and budget, including Redis-unavailable | **Measured** — §6. 256 → 1 computation with and without Redis |
| W0.8 bounded candidate location | **Measured** — §5. 32.3x / 18.5x rows examined in steady state; 4% slower on backlog sweeps |
| W1 recorder memory bounded | **Measured** — §4. Entry counts, record bytes and retained string bytes all bounded; the §4.2 defect is fixed and re-measured (1,056,944 B → 8,373 B, flat in input) |
| W1 active-recorder admission | **Measured as a ceiling** — §4, arithmetic; behaviour in `TestRecorderAdmissionBudgetIsEnforced` |
| W1 SQL batching bounded by rows, bytes and parameters | **Measured** — §3. 50,000 → 500 rows; parameter ceiling now splits instead of failing |
| W1 Flush reports failed persistence | Behavioural only (`TestSQLSinkFlushReportsSustainedDatabaseOutage`, `TestSQLSinkCloseReportsUnfinishedWork`); nothing quantitative to measure |
| W1 OTLP: one request SERVER span, correct parentage | **Established** — cited tests in §9, plus 2 → 1 span measured |
| W1 shutdown ordering and retention worker join | Behavioural only (`TestWaitForRetentionWorkersJoinsStartedWorkers`, `retention_shutdown_test.go`) |
| Default compatibility, mixed schemas/binaries, config matrix | **Not covered by this record** |
| No target-load claim | Honoured — see §11 |

**G1 is not closed by this document.** Every W0/W1 resource bound it set out to measure is
now supported, and the two defects it surfaced are fixed and re-measured. What remains
outside it is the compatibility and config-matrix evidence (covered by the correctness
suites, not by this record) and, above all, the full-relay load evidence G5 requires. No
capacity claim follows from anything here.

### 12.1 Repository checks

```
$ cd /home/laisky/repo/laisky/one-api && go build ./... && go vet ./... 2>&1 | tail -20
EXIT=0
```

Both commands produced **no output at all**; the `EXIT=0` line is the harness's own echo.

```
$ go test ./... -timeout 40m
exit=0
FAIL count: 0
ok  packages: 103

$ go test -race ./common/tracing/ ./common/logger/ ./controller/ ./middleware/ ./common/config/ -timeout 40m
exit=0
FAIL / DATA RACE count: 0
ok  github.com/Laisky/one-api/common/tracing   8.956s
ok  github.com/Laisky/one-api/common/logger    3.580s
ok  github.com/Laisky/one-api/controller     246.956s
ok  github.com/Laisky/one-api/middleware      (cached)
ok  github.com/Laisky/one-api/common/config    1.073s
```

`FAIL` was grepped for explicitly in every log, including every benchmark log, and matched
nothing. Neither `make build-frontend-modern` nor any frontend check was run: this change
set touches no frontend file.

---

## 13. Measurement code added

All of it lives in the packages it measures, named `Measure` so a reader can tell it apart
from the acceptance tests.

| File | Measures |
| --- | --- |
| `common/tracing/measure_drain_test.go` | §3 flush-local drain, per-row heap, parameter ceiling |
| `common/tracing/measure_recorder_memory_test.go` | §4 active-recorder bytes, admission ceiling, string retention; §10.2 lifecycle and admission benchmarks |
| `common/tracing/measure_otlp_span_test.go` | §9 spans and attribute payload per request |
| `model/measure_retention_scan_test.go` | §5 rows examined, plans, statements, wall time, per engine and mix |
| `controller/measure_dashboard_coalesce_test.go` | §6 computations per concurrent herd, budget concurrency |
| `common/logger/measure_log_storm_test.go` | §8 storm bytes; §7 guard sample cost and cadence arithmetic; emergency hot-path benchmark |

---

## 14. Still unmeasured

1. **Retention steady-state catch-up under concurrent load** (proposal §8.3), and the
   multi-pass path, which no arm in §5 reached.
2. **A cold-cache and larger-than-buffer-pool retention run.** Every §5 figure is warm.
3. **Real six-query dashboard aggregates behind the coalescer.** §6 uses a 25 ms stub;
   the database-side effect of 256 → 1 is inferred from the Phase 0/1 record, not
   re-measured.
4. **Active-file size rotation (W0.2) has no quantitative record.** Its behaviour is
   covered by `rotation_size_test.go` (8 tests) but nothing here measures rotation latency,
   the cost of `highestSequence` on a directory with many files, or write throughput
   across a size rotation.
5. **Flush-barrier and shutdown timing.** `Flush` now reports failed persistence and
   `Close` joins writers; neither has a latency or throughput measurement.
6. **OTLP serialized byte volume**, blocked by the exporter's internal transform package.
7. **GC pause and RSS behaviour** at any of the projected heap sizes in §4.
8. **The full-relay G5 harness**, which does not exist.
