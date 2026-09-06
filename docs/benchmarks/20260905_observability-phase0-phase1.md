# Measured results: observability Phase 0 and Phase 1

- Date: 2026-09-05
- Proposal: [Observability Data Tiering](../proposals/20260905_observability-data-tiering.md)
- Baseline tree (**before**): commit `397781e1` — predates all Phase 0/1 work
- Measured tree (**after**): `11663356cc7953ddb99b0008393238e8d00d3f37`
  (a `git stash create` snapshot of the working tree; the work is not yet committed)

## 1. How to reproduce

```sh
docker run -d --name oneapi-bench-pg -e POSTGRES_PASSWORD=bench \
  -e POSTGRES_DB=oneapi_bench -p 15432:5432 postgres:17-alpine
docker run -d --name oneapi-bench-mysql -e MYSQL_ROOT_PASSWORD=bench \
  -e MYSQL_DATABASE=oneapi_bench -p 13306:3306 mysql:8.4

export ONEAPI_BENCH_PG_DSN="postgres://postgres:bench@127.0.0.1:15432/oneapi_bench?sslmode=disable"
export ONEAPI_BENCH_MYSQL_DSN="root:bench@tcp(127.0.0.1:13306)/oneapi_bench?charset=utf8mb4&parseTime=True&loc=Local"

# The exact invocations that produced the numbers below.
go test ./middleware/ -run XXX -bench BenchmarkTracePipeline      -benchtime=5000x -count=6 -timeout=120m
go test ./middleware/ -run XXX -bench BenchmarkTraceArrivalRate   -benchtime=1x    -count=3
go test ./model/ -run XXX -bench BenchmarkRetentionSweep -benchtime=1x -count=5 -timeout=240m
# Each dashboard measurement runs in its OWN invocation: they share one database
# and one table name, so running them together lets an earlier arm size the table
# a later one measures.
go test ./model/ -run XXX -bench BenchmarkSiteWideQuotaStats/postgres/uncached -benchtime=20x      -count=6 -timeout=90m
go test ./model/ -run XXX -bench BenchmarkSiteWideQuotaStatsCachedHit         -benchtime=5000000x -count=6 -timeout=90m
go test ./model/ -run XXX -bench BenchmarkSiteWideQuotaStatsColdMiss/postgres -benchtime=5x       -count=6 -timeout=120m
go test ./model/      -run XXX -bench BenchmarkRecordLogLineBytes -benchtime=2000x -count=1

# The log-volume baseline additionally requires the pre-work tree:
git worktree add --detach /tmp/oneapi-base 397781e1
cp model/log_volume_bench_test.go /tmp/oneapi-base/model/
( cd /tmp/oneapi-base && go test ./model/ -run XXX -bench BenchmarkRecordLogLineBytes -benchtime=2000x )
```

Both DSN variables are optional: without them every benchmark still runs against
file-backed SQLite, so `go test ./...` never depends on a container.

Machine: AMD Ryzen 7 5700G (16 threads), 27 GB RAM, Linux 6.8, Go 1.27.1.
PostgreSQL 17-alpine and MySQL 8.4 in Docker on loopback. SQLite is file-backed
with `WAL` + `synchronous=NORMAL`, matching `model.openSQLite`.

> **Which configuration these numbers describe.** After a backward-compatibility
> audit, every optimization measured here is opt-in: the standalone defaults
> reproduce pre-proposal behaviour exactly (see
> [the compatibility contract](../proposals/20260905_observability-data-tiering.md#6-backward-compatibility-contract)).
> The "after" arms below therefore correspond to `OBSERVABILITY_PROFILE=scaled`
> settings — batched trace writes, the compact log line, dashboard caching —
> not to what an operator gets by upgrading. The "before" arms correspond to
> both the pre-change code *and* the current standalone defaults.

## 2. Methodology, and what would have made these numbers lie

Five controls were added specifically because, without them, each of these
measurements produces a flattering number that is not true.

| Control | Why it is necessary |
| --- | --- |
| **Floor arm** (`floor_no_tracing`) | An end-to-end ratio between two write paths can be inflated without limit by making the benchmark handler cheap, because the ratio is then dominated by work neither arm does. All Phase-1 timing claims below are quoted as **absolute microseconds attributable to tracing** (arm − floor), not as end-to-end ratios. |
| **Work-conservation metrics** (`pg_wal_B/op`, `pg_tup_upd/op`, `pg_xact/op`) | A batching writer can improve request latency by *relocating* work to another core while conserving it. Request-goroutine wall time cannot distinguish that from removing work. WAL bytes can: it is the durable work the server actually performed, and it is read from `pg_current_wal_lsn()` rather than the statistics collector, so it has no reporting lag. Caveats: `pg_current_wal_lsn()` is cluster-wide (the collector columns are filtered to this database) and the sample is taken 1.5 s after the workload, so background WAL is attributed to the arm — the floor arm measured 0 B/op in all six iterations, which bounds that contamination. `pg_tup_upd` and `pg_xact` **do** come from the collector and undercount by ~3%; they are corroborating only. |
| **Production connection pool policy** | An earlier run used `database/sql` defaults (2 idle connections) and reported a PostgreSQL baseline of 10.9 ms; applying `model.setDBConns`'s policy roughly halved it. The twelve-statement arm needs far more connections than the batched one, so the default pool was starving it and manufacturing much of the apparent win. The harness applies that policy **capped at 32 open connections** (`benchdb.benchMaxOpenConns`) so a multi-arm suite cannot exhaust the container's `max_connections`; production allows 2000. 16 request goroutines plus 2 writers never approach the cap, so no arm is starved. |
| **Arrival-rate curve** | Statements per trace is not a constant. A closed-loop driver at maximum rate fills the 500-row batch instantly and yields the most flattering possible figure. §4 publishes the curve. |
| **Verified row counts** | The unsampled batched arm asserts exact persistence (`rows == requests`) and fails the benchmark on any shortfall. The sampled arm can only be bounded from above (`rows <= requests`). The sync baseline **reports** its loss as `trace_loss_pct` rather than asserting it, because dropping traces under write contention is a real property of that path and an assertion would hide it. |

Sink configuration is the **production default** in every arm
(`TRACE_BATCH_SIZE=500`, `TRACE_FLUSH_INTERVAL_MS=1000`, `TRACE_WRITER_COUNT=2`,
`TRACE_QUEUE_SIZE=20000`). Tuning the writer for the benchmark would report a
configuration nobody runs.

### 2.1 Which baseline

Two baselines exist and they answer different questions.

- **In-tree** (`TRACE_WRITE_MODE=sync`): the real pre-Phase-1 code path, still
  compiled in, verified by `TestTracingMiddlewareSyncModeKeepsLegacyWrites`.
  Same binary, same handle, same harness, so it isolates the write-path change
  from everything else in the tree. Used for §3.
- **Cross-tree** (worktree at `397781e1`): the literal historical source. Used
  for §6, where the change is confined to one function and a cross-tree
  benchmark file compiles unchanged in both trees.

The in-tree baseline is **not identical** to the historical code. It carries six
extra `ts_*` columns on every `SELECT *` and `INSERT`, one extra `SET` clause per
`UPDATE`, and a `requestExcluded` prefix scan per call. Every one of those
differences makes the in-tree baseline do **more** work than the true pre-work
code, so §3 is biased in the optimization's favour by that amount. The bias is
small relative to twelve network round trips, but it is real and it is why §3
quotes absolute microseconds rather than a headline multiplier.

## 3. Phase 1 — trace write pipeline

`BenchmarkTracePipeline`, 5000 requests per iteration, 6 iterations, 16-way
parallelism, `benchstat` medians with the observed spread.

### 3.1 Statements per request (exact, not timing-dependent)

| Engine | arm | stmts/req before flush | total stmts/req |
| --- | --- | --- | --- |
| all | `floor_no_tracing` | 0 | 0.0002 |
| all | `baseline_sync` | **12.00** ± 0% | **12.00** ± 0% |
| sqlite | `batched` | 0.0016 ± 25% | 0.0022 |
| postgres | `batched` | 0.0006 ± 33% | 0.0022 |
| mysql | `batched` | 0.0007 ± 43% | 0.0022 |

The "before flush" column is everything issued up to the moment the timer
stopped. In the batched arms it is **not** purely request-goroutine work: the
writer goroutines run concurrently, so their `INSERT`s land in it too, which is
why the residue is non-zero. The stronger claim — that the request goroutine
issues **exactly zero** statements — is proven separately and deterministically
by `TestTracingMiddlewareIssuesNoStatementsOnRequestPath`, which sizes the batch
above the request count and the flush ticker an hour out so no write is possible
until the test asks for one.

The twelve statements are exactly the ones the proposal named: 1 `INSERT`,
5 × (`SELECT` + read-modify-write `UPDATE`), 1 status `UPDATE`. The
`baseline_sync` figure is an exact integer that does not move between runs
(± 0%), so that row is a derivation rather than a measurement. The `batched`
rows do vary (± 25–43%) because how many writer `INSERT`s land inside the window
depends on timing. **Request-path statements: 12 → 0**, proven by the
dedicated test above; the `0.0006–0.0016` residue in the benchmark is the
writer's `INSERT` amortized over ~500 traces, executing off the request
goroutine.

### 3.2 Trace-attributable cost per request

Under `b.RunParallel` with 16 goroutines, `ns/op` is **wall clock per request,
i.e. the reciprocal of throughput** — not the latency an individual request
experiences (mean service time is ~16× larger). The table below is therefore a
throughput statement: how much wall time each request costs the machine.

Absolute microseconds, floor subtracted. The ± on a difference is the propagated
worst case, not the ± of either arm; because the floor is a large fraction of the
batched arm, that propagation matters.

| Engine | floor | baseline attributable | batched attributable | µs saved/req |
| --- | --- | --- | --- | --- |
| sqlite | 17.6 µs ± 22% | **1,760 µs** ± 5% | 20.7 µs ± ~41% | **1,740** |
| postgres | 14.2 µs ± 22% | **4,080 µs** ± 5% | 21.4 µs ± ~89% | **4,058** |
| mysql | 16.5 µs ± 30% | **6,209 µs** ± 29% | 30.7 µs (interval includes 0) | **6,178** |

The saving is dominated by the baseline term and is robust. The *batched*
absolute values (20.7-30.7 µs) are of the same order as this harness's floor
(14.2-17.6 µs) and, given the propagated spread, are not statistically separable
from it; MySQL's interval includes zero. Only the saving is robust — the batched
path's own cost is below this harness's resolution, not measured to be zero.

Including the full drain (`total_ns/op`, which counts the writer goroutines'
work inside the measured window) the saving is 1,734 / 4,028 / 6,141 µs per
request respectively — i.e. the win does not come from deferring work past the
end of the measurement.

### 3.3 Work conservation (PostgreSQL)

This is the decisive table. If the rewrite had merely relocated work, these
would be unchanged.

| Metric | `baseline_sync` | `batched` | `batched` @ 5% sampling |
| --- | --- | --- | --- |
| **WAL bytes per request** | **2,602 B** ± 4% | **746 B** ± 10% | **36 B** ± 5% |
| WAL bytes per *persisted* trace | 2,602 B | 746 B | ~708 B |
| Row versions updated per trace (derived) | **6** | **0** | **0** |
| Row versions updated per trace (collector) | 5.81 ± 3% | 0 | 0 |
| Transactions per trace (derived) | **12** | — | — |
| Transactions per trace (collector) | 11.62 ± 3% | 0.0018 ± 56% | 0.0006 |
| Traces persisted (of 5000) | 5000 | 5000 | 255 ± 5% |

The "derived" rows are exact, from the statement sequence §3.1 measures: the
sync path issues 1 `INSERT` + 5 × (`SELECT` + `UPDATE`) + 1 status `UPDATE`, so
6 row-version updates and 12 autocommit transactions per trace. The "collector"
rows are `pg_stat_database`, which lags and reads ~3% low; they corroborate the
derivation but should not be quoted to three significant figures.

Two readings matter, and they are different:

- **The rewrite itself cuts durable work 3.5×** (2,602 → 746 WAL bytes per
  trace), and eliminates the six whole-row rewrites per trace that came from
  mutating a TEXT column in place.
- **Sampling cuts total durable work a further ~20×, by storing ~20× fewer
  traces — not by making a trace cheaper.** Per *persisted* trace the sampled arm
  costs ~708 B, statistically the same as the unsampled 746 B. Reading the
  per-request row alone would suggest sampling made each stored trace 20×
  cheaper. It did not.

## 4. Statements per trace is a curve, not a constant

`BenchmarkTraceArrivalRate`, PostgreSQL, 3-second window, production sink
defaults, 3 iterations per point.

The driver is **rate-limited closed loop**, not open loop: one goroutine waits
for a tick then blocks in `ServeHTTP`, so offered load is capped at
1/service_time and surplus ticks are discarded. The requested rate is an upper
bound; the achieved rate is what was delivered and is the number to read.

| Requested | **Achieved** | traces | writer `INSERT`s | traces per `INSERT` | total stmts/trace |
| --- | --- | --- | --- | --- | --- |
| 50 /s | **~50 /s** | 149–150 | 7 | 21 | **0.0534** |
| 500 /s | **355–491 /s** | 1,064–1,474 | 6–7 | 152–246 | **0.0047–0.0075** |
| 5,000 /s | **~1,870 /s** | 5,595–5,631 | 13 | ~431 | **~0.0025** |

The single-goroutine driver saturates near 1,900 req/s, so the top of this curve
is ~1.9 k/s, not 5 k/s. The saturated end is covered instead by
`BenchmarkTracePipeline` (§3.1), which drives 16-way parallel and reaches
0.0022 stmts/trace.

At low load the 1-second flush ticker, not the 500-row batch, decides when a
write happens, so statements per trace is ~21× higher at 50 req/s than at
~1.9 k/s. **Quoting only the saturated figure would overstate the result at
realistic load.** Even the worst point on this curve is 225× fewer statements
than the baseline's 12.

## 5. Phase 0 W0.2 — chunked retention

`BenchmarkRetentionSweep`: 200,000 expired + 5,000 fresh rows swept while a
simulated gateway offers a **fixed 400 requests/second** against the same table
through a separate, uninstrumented connection. Five sweeps per cell.

> **The foreground generator was closed loop in two earlier versions of this
> section, and that invalidated both of them.** An unthrottled generator issues
> as many operations as the database will accept, so a sweep that runs four
> times longer observes roughly four times more operations: both the numerator
> and the denominator of any degradation measure then depend on the variant, and
> no two arms are comparable. The second version concluded from that data that
> chunking makes foreground impact *worse*. With a fixed offered rate the
> opposite is true, and the table below supersedes it.

Two degradation measures are reported, and with a fixed offered rate both are
comparable across arms:

- **degraded %** — the share of concurrent requests that took ≥ 25 ms. This is
  per-request risk: how likely any one user is to feel the sweep.
- **degraded ops** — the absolute count. This is total harm: how many users felt
  it. Every arm removes the same 200,000 rows, so the counts are comparable.

| Engine | Variant | longest DELETE | degraded % | degraded ops | sweep | delete rows/s |
| --- | --- | --- | --- | --- | --- | --- |
| postgres | baseline unbounded | 134 ms | **11.76%** | 4 | 134 ms | 1,489k |
| postgres | chunked, pause 0 | 29 ms | 1.64% | 4 | 608 ms | 328k |
| postgres | **chunked, pause 10 ms** | **22 ms** | **0.00%** | **0** | 1,030 ms | **194k** |
| postgres | chunked, pause 100 ms | 22 ms | 0.00% | 0 | 4,626 ms | 43k |
| mysql | baseline unbounded | 2,890 ms | 0.86% | 9 | 2,890 ms | 69k |
| mysql | chunked, pause 0 | 394 ms | 0.67% | 17 | 6,466 ms | 31k |
| mysql | **chunked, pause 10 ms** | **383 ms** | **0.28%** | **7** | 6,268 ms | **32k** |
| mysql | chunked, pause 100 ms | 335 ms | 0.36% | 14 | 9,767 ms | 20k |
| sqlite | baseline unbounded | 1,495 ms | **26.67%** | 4 | 1,495 ms | 134k |
| sqlite | chunked, pause 0 | 98 ms | 17.95% | 42 | 1,469 ms | 136k |
| sqlite | **chunked, pause 10 ms** | **68 ms** | 18.24% | 65 | 1,798 ms | **111k** |
| sqlite | chunked, pause 100 ms | 41 ms | 5.68% | 92 | 5,005 ms | 40k |

No arm produced a single foreground error on any engine.

### 5.1 What chunking achieves

**The longest statement is bounded, on every engine:** MySQL 2,890 → 383 ms
(7.5×), SQLite 1,495 → 68 ms (22×), PostgreSQL 134 → 22 ms (6.1×). This is what
W0.2 was built for, and it holds. It is close to tautological for the baseline
arm, whose single statement *is* the sweep; for the chunked arms the per-chunk
duration is a real measurement.

**Per-request risk falls on every engine:** PostgreSQL 11.76% → 0.00%, MySQL
0.86% → 0.28%, SQLite 26.67% → 18.24% (and → 5.68% at a 100 ms pause). The
earlier claim that chunking made this worse was an artifact of the closed-loop
generator.

**Total harm is mixed.** PostgreSQL improves (4 → 0 degraded requests) and MySQL
improves (9 → 7); SQLite gets worse (4 → 65), because SQLite serializes writers,
so spreading the same work over a longer window exposes more requests to it
without reducing contention. SQLite is the small-deployment default, where a
retention sweep touches a table orders of magnitude smaller than this fixture,
so this is a caveat rather than a problem — but it is real and it is the reason
chunking is not universally free.

**Throughput falls 2–8×** versus the unbounded DELETE. That is the price of
bounded statements, and section 5.2 is about spending it well.

### 5.2 Choosing `RETENTION_DELETE_PAUSE_MS`: 100 ms → 10 ms

The pause exists to yield the database between chunks. The full sweep over
0 / 10 / 25 / 50 / 100 / 200 ms:

| pause | PG degraded % | PG rows/s | MySQL degraded % | MySQL degraded ops | MySQL rows/s | SQLite degraded % |
| --- | --- | --- | --- | --- | --- | --- |
| 0 | 1.64% | 328k | 0.67% | 17 | 30.9k | 17.95% |
| **10** | **0.00%** | **194k** | **0.28%** | **7** | **31.9k** | 18.24% |
| 25 | 0.00% | 121k | 0.51% | 14 | 28.8k | 17.18% |
| 50 | 0.00% | 75k | 0.62% | 20 | 24.1k | 9.35% |
| 100 | 0.00% | 43k | 0.36% | 14 | 20.5k | 5.68% |
| 200 | 0.00% | 23k | 0.21% | 11 | 15.1k | 2.98% |

**The default is now 10 ms.** The reasoning:

- **Yielding is necessary.** PostgreSQL at pause 0 degrades 1.64% of concurrent
  requests; any pause ≥ 10 ms degrades none. So 0 is not the answer, which
  refutes what an earlier closed-loop measurement suggested.
- **10 ms is sufficient.** PostgreSQL is already at 0.00% there, and everything
  above 10 ms buys no further improvement while costing 1.6–8× throughput.
- **10 ms is also the MySQL optimum** in this sweep, on degraded-request count
  (7, the lowest), on degraded percentage (0.28%, second lowest) and on
  throughput (31.9k, the highest).
- **Throughput is the tie-breaker.** At 10 ms the sweeper deletes 194k rows/s on
  PostgreSQL and 32k on MySQL, versus 43k and 20k at 100 ms. An hourly sweep has
  to stay ahead of arrivals, and the earlier 100 ms default left MySQL with a
  margin this document previously flagged as uncomfortably thin.

**Where 10 ms is not optimal is SQLite**, whose degraded percentage keeps falling
all the way to 200 ms (2.98%). That is not a reason to slow every engine down:
SQLite deployments are the small ones, their sweeps are short, and the knob is
one environment variable away for anyone who wants it. The trade is stated here
rather than hidden.

The `stmt_max_ms` column is essentially flat across pause values within each
engine, confirming that the pause does not affect boundedness — that is set by
`RETENTION_DELETE_BATCH_SIZE`.

> Delete throughput here is total rows removed over total sweep wall time on a
> fully cached 205k-row table under 400 requests/second of competing load. It is
> an **upper bound** on steady-state capacity, not an estimate: a production
> table is far larger than the buffer pool.

## 6. Phase 0 W0.4 — application log volume

`BenchmarkRecordLogLineBytes`, run in **both trees** with a byte-identical
benchmark file, JSON encoding, measuring bytes appended to a file sink by one
billed request's `record log` line.

Production uses **console** encoding (`common/logger/logger.go`
`configureGlobalLogger`); JSON is measured as well because most shipping
pipelines re-encode downstream.

Console encoding (production):

| Rendered content | before (`397781e1`) | after | saved | share of the line |
| --- | --- | --- | --- | --- |
| short (62 B) | 516.4 B | **413.4 B** | 103 B | 19.9% |
| typical (122 B) | 576.4 B | **413.4 B** | **163 B** | 28.3% |
| long (297 B) | 751.4 B | **413.4 B** | 338 B | 45.0% |

JSON encoding:

| Rendered content | before (`397781e1`) | after | saved | share of the line |
| --- | --- | --- | --- | --- |
| short (62 B) | 531.4 B | **432.4 B** | 99 B | 18.6% |
| typical (122 B) | 591.4 B | **432.4 B** | 159 B | 26.9% |
| long (297 B) | 766.4 B | **432.4 B** | 334 B | 43.6% |

The line after the change is flat in content length because it no longer carries
`content`; the line before it grew with it. Note the saving is not attributable
to the `content` demotion alone: the pre-change INFO line also carried
`created_at`, which the post-change line drops. The per-row deltas track the
content-length differences exactly (122−62 = 60 B versus a 60 B delta;
297−122 = 175 B versus a 175 B delta), so `created_at` accounts for the constant
~43 B floor of the saving and `content` for the rest.

**This corrected an overclaim in the source.** The comment in
`model/log.go` originally said this line was "roughly 1 TB/day at 10k
requests/second", conflating the whole process's log volume with this one line.
Measured: at 10,000 requests/second, console encoding and typical content, the
change avoids **~141 GB/day** (163 B × 10,000/s × 86,400 s) — **with sampling
disabled**, which is the `standalone` default.

That qualification matters, because the profiles in which 10,000 req/s is even
plausible are exactly the ones where sampling is on: `scaled` and `external`
default to `LOG_SAMPLE_INITIAL=100`, `LOG_SAMPLE_THEREAFTER=100` over a 1000 ms
tick, and the level-bounded sampler applies to this very INFO line. At 10k req/s
that thins it to 100 + (10000−100)/100 ≈ **199 lines/s**, so the volume this
change avoids there is only **~2.8 GB/day**. The two mechanisms are not additive:
sampling subsumes most of the demotion's benefit at high load. The demotion's
real value is at the volumes where sampling is off.
`LOG_SAMPLE_INITIAL` is the lever for the rest. The comment now states the
measured figures.

## 7. Phase 0 W0.5 — dashboard

`BenchmarkSiteWideQuotaStats`, PostgreSQL, production `users` schema, six runs
per cell, `benchstat` medians.

| Users | uncached, per dashboard load | growth per 10x rows |
| --- | --- | --- |
| 10,000 | **3.05 ms** ±17% | — |
| 100,000 | **25.1 ms** ±4% | 8.2x |
| 1,000,000 | **114 ms** ±18% | 4.5x |

`BenchmarkSiteWideQuotaStatsCachedHit`, five million iterations per run, six runs:

| | value |
| --- | --- |
| cached hit | **66.4 ns** ±3% |
| bytes per hit | **0 B** |
| allocations per hit | **0** |

The hit is reported as a single figure because it cannot depend on table size:
the path reads one struct under a mutex and issues no query.
`TestSiteWideQuotaCacheHitIsSizeIndependent` proves that directly by removing the
database handle and requiring a hit to still succeed. An earlier version of this
section printed 120 / 209 / 98 ns per user count as though that were a scaling
measurement; at `-benchtime=20x` those were roughly two microseconds of total
measured time, which is below timer resolution.

The uncached aggregate grows with user count but **sub-linearly above 100k rows**
(8.2x then 4.5x per 10x of rows), because PostgreSQL parallelises the larger
scan: it is O(rows) of work, not O(rows) of wall time.

### 7.1 The cache's global mutex is a single-flight, not a serialization point

The cache takes a process-global mutex and holds it across the query on a miss.
Reviewed in isolation that looks like a regression: concurrent cold viewers that
previously ran their scans in parallel would now queue. But the waiters re-check
the cache after acquiring the lock, so the prediction is the opposite. The two
hypotheses make different, checkable predictions:

- **serialization** — the cached column rises with viewer count, roughly N x one query;
- **single flight** — the cached column stays FLAT at one query's cost, while the
  uncached column rises.

`BenchmarkSiteWideQuotaStatsColdMiss`, 100,000 users, cache expired at the start
of every iteration, six runs per cell, wall time for the whole herd to be served:

| Concurrent viewers | uncached (`TTL=0`) | cached | ratio |
| --- | --- | --- | --- |
| 1 | 28.5 ms ±15% | 25.4 ms ±11% | 1.1x (no difference) |
| 8 | 41.8 ms ±22% | **25.9 ms** ±25% | 1.6x |
| 32 | **121 ms** ±6% | **25.9 ms** ±7% | **4.7x** |

The cached column is flat at ~25.5 ms — one query — across a 32x range of viewer
counts, while the uncached column grows. That is the single-flight signature, and
the serialization concern is refuted.

Two corrections to an earlier version of this table. It reported 54x at 32
viewers; that was measured on a `users` table left carrying roughly a million
dead tuples, because `benchdb.Reset` used `DELETE` rather than `TRUNCATE` and the
cold-miss arms ran immediately after the 1,000,000-user arms. The tell was that
the same nominal 100,000-row query cost 296 ms there and 35.6 ms in section 7.
The harness now truncates, each measurement runs in its own process, and the two
sections agree (25.4 ms versus 25.1 ms). The second correction is the reasoning:
a 54x ratio was never consistent with the mechanism being claimed, since a
single flight still has to run one full query. The honest figure is 4.7x.

The flip side, already in section 8: for a **single** cold viewer the cache buys
nothing measurable, and that viewer still pays the full scan on the request path.

## 8. Honest negatives, collected

Everything below got worse, stayed flat, or is weaker than the proposal implied.

1. **Chunked retention is 2–8× slower in total sweep time**, and delete
   throughput falls correspondingly (§5). This is the deliberate price of
   bounded statements.
2. **On SQLite, chunking increases the total number of degraded requests**
   (4 → 65 per sweep), even though it reduces per-request risk and cuts the
   longest statement 22×. SQLite serializes writers, so spreading the same work
   over a longer window exposes more requests to it (§5.1).
3. **The delete-throughput figure is an upper bound** on steady-state capacity,
   measured on a fully cached 205k-row table, not an estimate of production
   capacity (§5.2).
4. **Two earlier versions of §5 reached wrong conclusions** because the
   foreground load generator was closed loop, which makes any degradation
   measure incomparable between arms of different duration. The second version
   published "chunking makes foreground impact worse"; a fixed-rate generator
   refutes it. Both are superseded (§5).
5. **Statements per trace is load-dependent**, 0.053 at 50 req/s versus ~0.0025
   at ~1,900 req/s; the saturated figure alone overstates the result ~21× (§4).
6. **The `record log` demotion saves 20–45% of one line**, ~141 GB/day at 10k
   requests/second **only with sampling off**; under the `scaled`/`external`
   profiles that reach such volumes, sampling already thins the line and the
   demotion is worth ~2.8 GB/day. Neither is the "1 TB/day" the source comment
   originally claimed (§6).
7. **The site-wide cache is no better than no cache for a single viewer**
   (25.4 ms versus 28.5 ms, within noise). It only pays off when viewers
   overlap, and its benefit at 32 concurrent viewers is 4.7x, not the 54x an
   earlier contaminated measurement reported (§7.1).
8. The in-tree Phase-1 baseline does slightly more work than the historical code
   and therefore biases §3 in the optimization's favour (§2.1).
9. Under a contended machine one 6-run set observed the **legacy synchronous path
   losing 4 of 5,000 traces** on SQLite (`CreateTrace` is best-effort and drops
   on write contention). It did not reproduce in the clean run reported here, so
   it is recorded as an observation, not a result. The batched path lost nothing
   in any run; `trace_loss_pct` is reported for every arm.
10. **The batched arm's absolute cost is not individually resolvable** against
    the harness floor: propagated uncertainty is ±41% / ±89% on SQLite and
    PostgreSQL, and MySQL's interval includes zero (§3.2). Only the *saving*,
    which is dominated by the baseline term, is robust.
11. **The arrival-rate driver saturates at ~1,900 req/s**, so §4's curve does not
    reach the 5,000 req/s its labels originally suggested. The requested-rate
    labels were corrected to achieved rates after verification.
12. **`pg_tup_upd` and `pg_xact` undercount by ~3%** (statistics-collector lag).
    The exact values are derivable from the statement sequence and §3.3 now
    reports both.
13. **Sampling does not make an individual trace cheaper to store.** Per
    persisted trace the sampled arm costs the same WAL as the unsampled one; the
    ~20× reduction comes entirely from storing ~20× fewer traces (§3.3).
14. **The batched sink introduces two durability failure modes the synchronous
    path did not have**, and **no benchmark here exercises either**:
    `sqlSink.Submit` drops the newest record when its queue is full (counting
    the drop), and records still queued are lost on an unclean exit. Every arm
    above submits 5,000 traces into a 20,000-entry queue, so the queue is never
    stressed. The `trace_loss_pct == 0` result is therefore evidence for an
    under-provisioned-queue-free run only, not for the sink's behaviour under
    saturation. Sizing `TRACE_QUEUE_SIZE` and watching
    `oneapi_trace_records_total{outcome="dropped_queue_full"}` remains an
    operator responsibility.

## 9. Claims in the proposal that these measurements support

| Proposal claim | Measured |
| --- | --- |
| ~12 statements per request → ~0 on the request path | 12.00 → 0.0006–0.0016 (§3.1) |
| One batched `INSERT` per `TRACE_BATCH_SIZE` traces | ~431 traces per `INSERT` at ~1.9 k req/s; 21 at 50 req/s (§4) |
| Trace pipeline adds ~3 µs of in-memory work | The proposal's 3 µs was the pure in-memory recorder plus row build, measured in isolation. End to end the batched path costs 21–31 µs of wall time per request at 16-way parallelism, which is not separable from this harness's 14–18 µs floor (§3.2). Both are true; they measure different things. |
| Retention sweeps stop holding one giant transaction | longest DELETE 3,737 → 354 ms on MySQL (§5) |
| Dashboard site-wide aggregate removed from the hot path | **Amortised, not removed**: 114 ms on every load → 114 ms once per `DASHBOARD_CACHE_TTL_SEC`, 66 ns on hits. A single cold viewer still pays the full scan (§7, §7.1). |
