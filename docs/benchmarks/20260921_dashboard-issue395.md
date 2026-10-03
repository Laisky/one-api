# Dashboard cold-path performance: issue #395

## Scope and result

[Issue #395](https://github.com/Laisky/one-api/issues/395) reports approximately
5 seconds for 290,000 logs over 7 days and 20 seconds for 1 million logs over
30 days, including nginx 504 failures. The reporter's database, hardware, log
cardinality, proxy configuration, and production deployment were not available.
This report therefore reproduces the repeated aggregate-query work, **not the
reporter's exact HTTP latency or a production 504**.

The baseline is `7cfbc6ebaf6505cadc5c28a69d37676ca2db50da`. Its cold dashboard
collector executes six independent queries from `model/log.go`. Redis caching
and in-process request coalescing already exist; they do not remove this work
on a cache miss. The change in [PR #415](https://github.com/Laisky/one-api/pull/415)
preaggregates the selected window once and derives all six views from that
query-local intermediate result. The real collector is wired to this path.

The measured SQL is `model/dashboard_aggregate.sql`, SHA-256:

```text
f084eb6df9a0e2b1e59eb854e63426e498d09521c8d22e7acb003b0308a17842
```

The change preserves authorized user scope, half-open timestamps, UTC days,
raw nullable grouping keys, database collations, cache-hit accounting, and
64-bit tool quotas. It does not persist rollups of mutable provisional logs,
fetch raw audit payloads into Go, change cache TTLs, raise timeouts, or introduce
a migration, external service, or additional CI job. MySQL 5.x, MariaDB, and
unrecognized MySQL versions retain the original six-query path.

## Measured results

These are **SQL execution plus Python row-fetch times**, in milliseconds.
Each value is the median of five paired samples on the same seeded database.
The application result cache is absent; database and OS caches are warm.

| Fixture | Scope | Six-query baseline | Shared preaggregation | Baseline / new |
| --- | --- | ---: | ---: | ---: |
| 290,000 rows / 7 days | Site-wide | 1,777.54 | 845.40 | 2.10x |
| 290,000 rows / 7 days | Hot tenant | 1,835.56 | 621.94 | 2.95x |
| 1,000,000 rows / 30 days | Site-wide | 6,358.60 | 2,837.77 | 2.24x |
| 1,000,000 rows / 30 days | Hot tenant | 6,423.84 | 2,117.71 | 3.03x |
| 100,000 unique tokens / 30 days | Site-wide | 808.71 | 899.21 | **0.90x** |
| 100,000 unique tokens / 30 days | Hot tenant | 894.54 | 834.86 | 1.07x |

**Known tradeoff: the unique-token site-wide stress case is 11.2% slower.**
Preaggregation cannot substantially compress a workload with one token per
request, yet still pays materialization and regrouping costs. This fixture and
its unfavourable result are retained. The change is not a universal speedup or
a constant-time dashboard implementation. High-cardinality deployments need
native-engine measurements before rollout.

[Raw measurement evidence](20260921_dashboard-issue395-results.json) contains
all 60 samples, including outliers; source hash; fixture and output row counts;
independent expected request/quota totals; and all optimized SQLite plans.
No sample was dropped. Five samples are not sufficient to claim a production
P95, and no Go allocation, MySQL/PostgreSQL speedup, concurrent-load throughput,
or HTTP/nginx result is inferred from this table.

## Reproduction and controls

The standalone [SQL reproducer](../../scripts/dashboard_395_sql.py) reads the
exact production SQL template. Its control arm mirrors the six baseline SQL
shapes. The Go benchmark instead invokes the unchanged production functions
directly for its control arm.

Both benchmark arms use the same exact-size fixture and must return equivalent
results before timing. Fixtures have a 1 KiB audit payload per row, the existing
`(created_at, type)` and `user_id` indexes, 17 model/tool names, and 11 token names
except in the unique-token stress case. User 7 owns 90% of requests; the rest
are distributed over 100 users. Every 23rd request is a tool call and every
third request has cached prompt tokens. Seeds use independent frequencies
rather than assigning each user a single fixed model/token combination.

The file-backed SQLite fixtures occupy 405,123,072 bytes, 1,397,215,232 bytes,
and 139,653,120 bytes respectively. Seeding, equality checks, and query-plan
inspection are outside timed regions. Arms alternate execution order. Timed
queries fully fetch their result sets; they do not return lazy cursors.

Environment: Python 3.13.5, SQLite 3.46.1, Linux x86_64, Intel Xeon Platinum
8375C. The container exposes five affinity CPUs but has a four-CPU cgroup quota
and a 4 GiB memory limit. SQLite uses file-backed WAL, `synchronous=NORMAL`,
`cache_size=-2000`, and `temp_store=FILE`. These container timings are not a
hardware-independent latency promise.

The deterministic SQLite plan check requires six physical accesses to `logs`
in the baseline versus one in the new query. It does **not** claim zero scans
of the materialized intermediate result. That result is reused by the six
output views.

## Correctness and regression tests

The executed standalone differential test passed 70 window/scope/collation
combinations, covering 420 result-set comparisons. It uses 2,000 adversarial
rows per BINARY/NOCASE fixture, including negative epochs, empty/reversed
windows, NULL/empty identities, negative/large quotas, provisional exclusion,
and case-sensitive tool-name semantics.

The Go suite adds native SQLite, PostgreSQL, and MySQL differential arms using
the existing CI database services. Required CI backends cannot silently skip.
It compares every DTO field and duplicate group against the old query functions,
checks non-UTC sessions and split-engine globals, and includes cancellation,
injected SQL failure, empty JSON shape, 64-bit tool quotas, and version fallback.
Sorted canonical rows keep large-fixture verification O(n log n).

`TestDashboardPreaggregationOneScan` pins the six-versus-one statement count
and the SQLite physical scan count. The controller test
`TestDashboardCacheMissUsesOneLogScan` exercises the actual resolver and
collector with Redis disabled, asserting all six views and owner isolation.
This prevents an unused optimized helper from passing acceptance.

An initial native run exposed MySQL's temporary-table reuse restriction in
the test fixture. The MySQL fixture now uses a cryptographically unique regular
table, a handle-local source-name rewrite for both control and new queries,
and cleanup restricted to that table. It never truncates or drops persistent
`logs`. SQLite and PostgreSQL retain connection-local temporary tables. A
native test failure must be fixed, not converted into a skipped backend.

Go 1.27.1 is required by the repository; the local container only has Go 1.23.2.
Full Go race/coverage verification runs in the repository's existing CI, not
through a downgraded local module. Current run links and acceptance status are
maintained in [PR #415](https://github.com/Laisky/one-api/pull/415). The SQL table
above is independent evidence and must not be relabelled as a Go benchmark run.

## Commands

From the repository root, with Python 3.10+ and SQLite 3.35+:

```sh
python3 scripts/dashboard_395_sql.py --verify-only
python3 scripts/dashboard_395_sql.py --samples 5 --storage file \
  --cases 290k 1m high-cardinality --output /tmp/dashboard-395-results.json
```

With the repository's Go toolchain:

```sh
go test ./model -run '^TestDashboardPreaggregation' -race -count=1
go test ./controller -run '^TestDashboardCacheMissUsesOneLogScan$' -race -count=1
go test ./model -run '^$' -bench '^BenchmarkDashboardAggregate395$' \
  -benchmem -benchtime=3x -count=3
```

`PG_DSN` and `MYSQL_DSN` enable the native correctness arms. The Go benchmark
uses `ONEAPI_BENCH_PG_DSN` and `ONEAPI_BENCH_MYSQL_DSN`. Use disposable benchmark
databases with permission to create and drop isolated fixture tables. The
benchmark reports `ns/op`, `B/op`, `allocs/op`, and `queries/op`; no invented
values for these metrics are supplied here. Large benchmarks are opt-in,
not additional mandatory CI jobs.

## Release boundary

Require the PR's native correctness/race checks to pass. Evaluate the explicit
high-cardinality tradeoff against a representative deployment before merging.
A deployment behind a proxy still needs a real endpoint check to establish
whether its 504 is resolved. No database migration or rollback data repair is
needed; reverting the code restores the old collector.

Implementation references: [SQLite materialization](https://sqlite.org/lang_with.html),
[PostgreSQL CTE materialization](https://www.postgresql.org/docs/17/queries-with.html),
[MySQL CTE optimization](https://dev.mysql.com/doc/refman/8.4/en/derived-table-optimization.html),
and [MySQL temporary-table limitations](https://dev.mysql.com/doc/refman/8.0/en/temporary-table-problems.html).
