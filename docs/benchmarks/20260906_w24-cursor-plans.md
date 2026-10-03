# W2.4 evidence: keyset log pagination, bounded counts, and response budgets

- Date: 2026-09-06
- Proposal: [Observability Data Tiering](../proposals/20260905_observability-data-tiering.md), §W2.4
- Gate: **G2**, the cursor/count and response-budget portion. This bundle does **not**
  discharge the projection portion of G2 (W2.1–W2.3) or the index-migration gate (W2.5).
- Work item status: W2.4 implemented; W2.5 informed by, but not closed by, the plans below.

## 1. What this bundle claims, and what it does not

**Claimed.** With the shipped code and the plans below:

1. Keyset page cost is independent of how deep the reader has walked — **given a supporting
   index**. §4.2 and §4.3 show what happens without one, which is why the capability ships
   disabled.
2. Legacy offset page cost grows with depth, and an unbounded exact `total` scans the table.
3. The bounded count probe terminates at its limit instead of scanning to the end.
4. The additive routes cannot be reached with a tampered, expired, or out-of-scope cursor.
5. A page is bounded in rows *and* in encoded bytes, without truncating any field.
6. Modern uses the capability explicitly and falls back to the legacy route when it is absent.
7. The legacy routes, their envelope, filters, sorts, default sort and exact `total` are
   untouched.

**Verified alongside.** `go test ./...` passes (100 packages, 0 failures) and the modern
frontend suite passes (76 files, 419 tests, 1 skipped). The new backend code additionally
passes under `-race`.

**Not claimed.** These are single-execution plans on one machine, not a load benchmark.
The wall-clock figures indicate order of magnitude and plan shape; they are not a latency
certification, and they say nothing about behaviour at the target relay load, which is G5's
job. No index change is shipped here: §4 measures the candidates so W2.5 can decide, and
the deployed index set is unchanged.

## 2. How to reproduce

```sh
docker run -d --name oneapi-w24-pg -e POSTGRES_PASSWORD=... -e POSTGRES_DB=oneapi \
  -p 127.0.0.1:55432:5432 postgres:17-alpine \
  -c shared_buffers=256MB -c max_connections=100 -c random_page_cost=1.1
docker run -d --name oneapi-w24-mysql -e MYSQL_ROOT_PASSWORD=... -e MYSQL_DATABASE=oneapi \
  -p 127.0.0.1:53306:3306 mysql:8.4 --innodb-buffer-pool-size=256M

ONEAPI_W24_PLANS=1 ONEAPI_W24_PLAN_OUT=/tmp/w24 \
ONEAPI_BENCH_PG_DSN='host=127.0.0.1 port=55432 user=... password=... dbname=oneapi sslmode=disable' \
ONEAPI_BENCH_MYSQL_DSN='...:...@tcp(127.0.0.1:53306)/oneapi?charset=utf8mb4&parseTime=True&loc=Local' \
go test ./model/ -run TestCollectW24CursorPlans -timeout 180m -v

# Re-collect against a corpus a previous run already seeded (server engines only;
# SQLite lives in a per-test temp dir). It drops the candidate indexes first, so
# the "before" plans really are before.
ONEAPI_W24_PLAN_REUSE=1 ONEAPI_W24_PLANS=1 ONEAPI_W24_PLAN_OUT=/tmp/w24b ... \
go test ./model/ -run 'TestCollectW24CursorPlans/(postgres|mysql)' -timeout 180m -v
```

Run it on an otherwise idle machine. Seeding takes 3–7 minutes per engine, and a collection
taken while the test suites are running has visibly inflated absolute values (§4.3 shows one).

The collector is `model/log_cursor_plan_test.go`. It explains the statements the request
path actually emits: every cursor and count statement in §3 comes from
`buildLogCursorStatement` and `buildLogCountProbeStatement`, the same builders the handlers
call. A plan for a rewritten query would be evidence about a query that does not ship.

**Corpus.** 2,000,000 usage rows, deterministic seed `20260906`, spanning 2026-01-01 onward
at 25 rows per second so the tie-breaker arm is exercised rather than trivially empty. The
distribution is skewed: user 1 owns ~70% of rows and a tail of 5,000 users owns the rest,
because a uniform distribution hides exactly the plans that matter. Types are ~94% consume,
2% top-up, 2% provisional, 2% manage. Statistics are refreshed (`VACUUM ANALYZE`,
`ANALYZE TABLE`, `ANALYZE`) before any plan is taken.

**Machine.** AMD Ryzen 7 5700G (16 threads), 27 GB RAM, Linux 6.8, Go 1.27.1.
PostgreSQL 17-alpine and MySQL 8.4 in Docker on loopback; SQLite file-backed with
`WAL` + `synchronous=NORMAL`, matching `model.openSQLite`.

Credentials are redacted above. The raw collector output contains no request content,
customer identifiers or DSNs beyond the generated fixture.

## 3. Correctness evidence

All tests are in the repository and run in the ordinary suite. `go test ./...` passes; the
new code additionally passes under `-race`.

### 3.1 Row selection is identical to the legacy list

The cursor route must select exactly the rows the legacy route selects — otherwise it is a
different report, not a faster one.

| Test | Establishes |
| --- | --- |
| `model.TestLogListPredicateMatchesLegacyRowSelection` | Across 15 filter combinations, `BuildLogListPredicate` selects exactly the rows `GetAllLogs`/`GetUserLogs` select |
| `model.TestLogCursorTraversalMatchesOrderedScan` | At page sizes 1, 2, 5, 10, 57 and 100, the traversal equals a single ordered scan — no gaps, no repeats |
| `model.TestLogCursorTraversalWithHeavyTies` | 250 rows sharing one second still traverse exactly once each; the tie-breaker arm is load-bearing |
| `model.TestLogCursorRespectsScopeAndProvisionalExclusion` | Self scope never sees another user's rows; provisional (type 6) rows are excluded unconditionally |
| `controller.TestCursorProvisionalRowsAreNeverListed` | An explicit `type=6` request selects nothing, rather than surfacing unfinalized billing rows |
| `controller.TestCursorTraversalCoversEveryRowExactlyOnce` | The property holds end-to-end over HTTP, not only at the model layer |

### 3.2 Live-traversal semantics are the documented ones

The cursor walks live data. The proposal states what that does and does not promise, and
these tests hold the implementation to that contract rather than to a stronger one.

| Test | Establishes |
| --- | --- |
| `model.TestCursorIgnoresRowsInsertedAheadOfTheAnchor` | Rows inserted newer than the anchor mid-walk neither appear nor shift the pages that follow |
| `model.TestCursorToleratesRetentionDeletesDuringTraversal` | Retention deleting ahead of the reader shortens the result without duplicating or stalling it; every surviving row is still visited |
| `model.TestCursorNeverDuplicatesUnderConcurrentWriters` | Under a continuous concurrent writer, no row is returned twice and no row newer than the start appears |

This is deliberately **not** a stable audit snapshot, and the proposal says so. Snapshot-
complete work uses the separately pinned export, which still walks the legacy offset route.

### 3.3 A cursor is not an authorization grant

| Test | Establishes |
| --- | --- |
| `logcursor.TestCursorRejectsEveryMutation` | An exhaustive single-bit-flip sweep over the sealed token: every mutation is rejected |
| `logcursor.TestCursorCannotCrossScope` | Scope kind, subject user and endpoint are bound as associated data; a cursor moved between scopes fails to open |
| `logcursor.TestCursorRejectsChangedQuery` | The normalized-filter digest is bound; changing any filter invalidates the cursor |
| `logcursor.TestCursorExpiry` | Expiry is enforced, with bounded tolerance for clock skew |
| `logcursor.TestCursorDoesNotLeakRowID` | The wire token reveals neither the row id nor the timestamp |
| `logcursor.TestCursorRejectsMalformedInput` | Truncated, oversized and non-token inputs are refused without allocation growth |
| `controller.TestCursorRejectsTamperingAndScopeChange` | Over HTTP: a flipped character, a truncation, replay by another user, a move to the admin route, and a changed filter each return a restart instruction and **no rows** |
| `controller.TestCursorRouteRequiresAdminForSiteWide` | The site-wide handler re-checks privilege itself, so it does not depend on the router still wiring a guard in front of it |
| `router.TestApiRouterRegistersCursorRoutesBesideLegacyOnes` | The real API router registers both additive paths and all seven legacy log paths without conflict |

Every rejection path returns before any SQL is built.

### 3.4 Counts state what they establish

| Test | Establishes |
| --- | --- |
| `model.TestProbeLogCountMatchesLegacyCount` | Below the bound, the probe equals the legacy exact count |
| `model.TestProbeLogCountQualityAtBoundary` | At and above the bound, the probe reports `lower_bound`, never a total |
| `model.TestProbeLogCountReportsUnavailableOnBudgetExhaustion` | An exhausted budget yields `unavailable` with a null value, classified from the probe's own context so a client disconnect is not misreported |
| `model.TestLogCountProbeStatementIsBounded` | The emitted statement contains `LIMIT N+1`; there is no unbounded count anywhere on the path |
| `controller.TestCursorCountQuality` | Over HTTP, the first count is computed and the second is served from cache and **labelled** `cached` with the instant it was taken |
| `admission.*` (4 tests) | The concurrency gate admits to capacity, releases idempotently, honours cancellation, and is safe under concurrency |

An unavailable count is never rendered as zero, and a lower bound is never rendered as a
total or as `N+`.

### 3.5 Response budget

| Test | Establishes |
| --- | --- |
| `controller.TestCursorByteCapKeepsTraversalComplete` | Under a tight byte budget the listing splits across more pages, every row still appears exactly once, and no field is truncated |
| `controller.TestCursorOversizedRecordIsFlaggedNotTruncated` | A record larger than the whole budget is returned **whole**, alone, and flagged — silently shortening it would be indistinguishable from the value being short |

The byte cap and the cursor anchor are kept aligned by construction: `encodeCursorItems`
returns how many source rows the emitted items account for, and the next cursor is derived
from that row. Skipping a row rather than stopping at it would put a hole in a traversal
that promises to visit every row exactly once.

### 3.6 Frontend

| Test | Establishes |
| --- | --- |
| `lib/logCursor.test.ts` (17 cases) | Query construction withholds site-wide filters from a non-admin; a disabled capability, an old server, and a malformed body are all read as *unsupported* rather than as an empty page; an unavailable count never carries a number; the trail places each page from rows actually delivered, so a short page does not mislabel every page after it |
| `pages/logs/__tests__/useLogCursorPagination.test.tsx` (7 cases) | Forward/back navigation reuses recorded anchors; a disabled capability or transport failure falls back once and stops re-asking; an expired cursor restarts in place exactly once; two consecutive refusals give up instead of looping |
| `pages/logs/__tests__/LogsPage.cursor.test.tsx` (4 cases) | End-to-end: the page calls `/api/log/cursor` with `v=1` and never a page number, renders the keyset pager, states a bounded count as "at least N", labels a reused count, surfaces the byte-cap notice, and falls back to `/api/log/` — showing rows, not an empty list — when the capability is absent |

Berry, Air and the default frontend are untouched and continue to use the legacy offset
routes. Translations for the new strings are present in all five locales (`en`, `zh`, `ja`,
`fr`, `es`); the locale diffs are additive.

## 4. Execution plans

Raw per-engine reports: `plans-sqlite.txt`, `plans-postgres.txt`, `plans-mysql.txt` from the
command in §2. The excerpts below are quoted verbatim.

**Read the plan shapes, not the milliseconds.** Each figure is a single execution including
client round-trip, parse and result transfer; run-to-run variation on the same engine is
tens of percent, and PostgreSQL's own `Execution Time` is frequently an order of magnitude
below the measured wall-clock for these small result sets. What is durable is the *work the
plan does*, which is why the row counts and scan types are quoted alongside.

### 4.1 The structural result: offset scans what it skips, keyset does not

PostgreSQL, legacy offset at page 5001, current index set:

```
Limit  (cost=5100.81..5101.83 rows=20 width=658) (actual time=40.197..40.210 rows=20 loops=1)
  Buffers: shared hit=3564
  ->  Index Scan Backward using logs_pkey on logs  (actual time=0.048..36.541 rows=100020 loops=1)
        Filter: (type <> '6'::bigint)
Execution Time: 40.276 ms
```

The scan reads **100,020 rows to return 20**. That is the definition of the problem: the
cost is proportional to the offset, so page 5001 costs 5001 pages of work and page 50,001
costs ten times that.

The keyset page reads a bounded number of rows regardless of depth. At the same position,
PostgreSQL's site-wide deep-anchor page reports:

```
Limit  (cost=29.19..30.43 rows=21 width=658) (actual time=0.287..0.295 rows=21 loops=1)
  Buffers: shared hit=10
```

**10 buffers against 3,564**, and 21 rows examined against 100,020, for the same page of
results.

This is the claim W2.4 rests on, and it is a property of the access pattern, not of any
particular timing.

### 4.2 MySQL 8.4 cannot serve the keyset order without a new index

This is the most consequential finding in the bundle. With the index set the schema ships
today, MySQL answers the *first* cursor page like this:

```
-> Limit: 21 row(s)  (cost=203960 rows=21) (actual time=6252..6252 rows=21 loops=1)
    -> Sort: `logs`.created_at DESC, `logs`.id DESC, limit input to 21 row(s) per chunk
        -> Filter: ((`logs`.`type` <> 6) and (`logs`.created_at is not null))  (actual time=0.0713..5581 rows=1.96e+6 loops=1)
            -> Table scan on logs  (actual time=0.0628..5318 rows=2e+6 loops=1)
```

A **full table scan of 2,000,000 rows and a filesort**, to return 21. Measured at 5.9 s and
9.1 s on two runs, against **0.8–6.1 ms** for the legacy offset page it would replace.

The cause is exactly what the proposal predicted: `idx_created_at_type` is
`(created_at, type)`, and `type <> 6` is not a range on the leading column, so it establishes
no usable path for `ORDER BY created_at DESC, id DESC`. The proposal's sentence — "a
`(type, created_at)` index alone does not establish an efficient unfiltered global cursor" —
is now measured rather than asserted. The *type-filtered* deep page is fine on the same index
set (5.7 ms), which is what makes the unfiltered case easy to miss.

Adding `(created_at, id)` removes it entirely:

| MySQL 8.4, 2M rows | Current indexes | With `(created_at, id)` + `(user_id, created_at, id)` |
| --- | --- | --- |
| `cursor/all/first-page` | 5,883 ms / 9,051 ms | 1.1 ms / 2.2 ms |
| `cursor/all/deep-anchor` | 5,742 ms / 9,456 ms | 4.4 ms / 2.1 ms |
| `cursor/all/deep-anchor+model` | 6,558 ms | 9.1 ms |
| `cursor/self/first-page` (heavy user, ~70% of rows) | 7,394 ms | 5.5 ms |
| `cursor/self/deep-anchor` (heavy user) | 7,584 ms | 218 ms |
| `cursor/self-tail/first-page` (tail user, ~0.01% of rows) | 49.5 ms | 3.8 ms |
| `legacy/all/offset-100000` (baseline) | 186 ms | 122 ms |
| `legacy/all/count` (baseline) | 1,211 ms | 443 ms |

One case remains unsolved: the **heavy user's deep page is still 218 ms** with both
candidates present, because MySQL picks ref access on `user_id` alone and then filters:

```
-> Index lookup on logs using idx_logs_user_created_at_id (user_id=1) (reverse)
   (cost=114247 rows=961122) (actual time=0.0274..158 rows=70102 loops=1)
   Filter: ((`logs`.`type` <> 6) and (`logs`.created_at < 1767301600))
```

It reads **70,102 rows** — every row that user owns newer than the anchor — instead of
seeking into the composite index on `(user_id, created_at)`. W2.5 owns resolving this;
it is recorded here as a known open item, not as a solved one.

Where two figures are given, they are two independent collections. They differ by up to 60%
in absolute terms — which is the point of the caveat at the head of §4 — and agree completely
on the finding: three to four orders of magnitude, not a tuning difference.

### 4.3 SQLite needs the per-user path

SQLite's site-wide cursor is fine on the current index set — the planner uses
`idx_created_at_type` and adds only a temp B-tree for the `id` tie-break. The
**authorized-user** cursor is not fine. Figures below are from a single collection taken on
an otherwise idle machine:

| SQLite, 2M rows | Current indexes | With candidates | |
| --- | --- | --- | --- |
| `cursor/all/first-page` | 270 µs | 190 µs | noise |
| `cursor/all/deep-anchor` | 291 µs | 391 µs | noise |
| `cursor/all/deep-anchor+type` | 305 µs | 483 µs | noise |
| `cursor/self/first-page` (heavy user) | **1,546 ms** | **185 µs** | ~8,400× |
| `cursor/self/deep-anchor` (heavy user) | **1,467 ms** | **594 µs** | ~2,500× |
| `cursor/self-tail/first-page` (tail user) | 1.22 ms | 28 µs | ~44× |
| `cursor/self-tail/deep-anchor` (tail user) | 644 µs | 191 µs | ~3× |
| `count/all/probe-10001` | 910 µs | 833 µs | noise |
| `legacy/all/offset-100000` (baseline) | 15.9 ms (`SCAN logs`) | 14.8 ms | |
| `legacy/all/count` (baseline) | 109 ms | 94 ms | |

Two earlier collections taken under load reported the same finding with inflated absolutes
(`cursor/self/deep-anchor` at 2,162 ms and 1,638 ms before, 295 µs and 7.3 ms after).
Sub-millisecond differences are inside this method's noise and no conclusion is drawn from
them. Seconds-versus-microseconds is not noise.

Without `(user_id, created_at, id)` the per-user arm falls back to `idx_created_at_type` and
filters by user id across a huge range. With it, the arm becomes:

```
SEARCH logs USING INDEX idx_logs_user_created_at_id (user_id=? AND created_at=? AND id<?)
```

Note that the **tail** user is fine either way (1.2 ms), and the **heavy** user is not
(1,546 ms). A per-user access path is not a small-user optimization; it is what keeps the
users who generate the most traffic from paying a full-range scan to read their own logs.

### 4.4 PostgreSQL is already adequate

PostgreSQL 17 serves every cursor family in single-digit milliseconds on the **current**
index set, using `Incremental Sort` with `Presorted Key: created_at` over
`idx_created_at_type` and stopping after the page. Both candidates change little
(differences are within this method's run-to-run variation), including for the tail user.
On the evidence here, the candidate indexes earn their write and storage cost on MySQL and
SQLite but not on PostgreSQL — which is precisely the per-engine decision W2.5 has to make,
and precisely why the proposal forbids adding every possible composite index.

### 4.5 The bounded count is the count story, on every engine

| `count/all/probe-10001` vs `legacy/all/count` | SQLite | PostgreSQL | MySQL |
| --- | --- | --- | --- |
| Bounded probe (10,001 rows) | 0.91 ms | 6.3 ms | 8.7 ms |
| Legacy unbounded exact total | 109 ms | 109 ms | 1,211 ms |

The probe's emitted statement is
`SELECT count(*) FROM (SELECT 1 FROM logs WHERE <pred> LIMIT 10001) AS probe`, so its cost
is bounded by construction rather than by luck, and it is the same statement on all three
engines (MySQL additionally carries a `MAX_EXECUTION_TIME` hint). The legacy total is
unchanged on the legacy routes; it is simply not run during ordinary cursor navigation.

## 5. Decisions this evidence forced

### 5.1 `LOG_CURSOR_ENABLED` now defaults to **off**

It was drafted as default-on. §4.2 makes that indefensible: on MySQL 8.4 with the shipped
schema, enabling the capability replaces a 0.8 ms log page with a 5.9 s one. That is not a
performance improvement with a caveat, it is a severe regression for every existing MySQL
operator with a large `logs` table — precisely what the compatibility contract exists to
prevent.

The default is therefore `false`, pinned by `config.TestCursorCapabilityIsOffByDefault`, and
the reasoning is recorded at the declaration so a future reader does not "fix" it. Modern
probes the capability once, is told it is unavailable, and uses the legacy route for the rest
of the session — a path with its own tests
(`useLogCursorPagination` fallback cases, `LogsPage.cursor.test.tsx` fallback case). An
operator whose engine has the access paths enables it explicitly.

The cost of the default-off probe is one refused request on first load per session. That
buys zero-configuration capability detection with no new endpoint and no version handshake.

### 5.2 The response byte cap stops at a row, it never skips one

`encodeCursorItems` returns how many source rows its output accounts for, and the next cursor
is derived from that row rather than from `len(items)`. An earlier draft skipped a row it
could not encode, which would silently put a hole in a traversal that promises to visit every
row exactly once.

### 5.3 The pager places pages from rows delivered, not from `pageIndex * pageSize`

A page cut short by the byte cap is shorter than `pageSize`. Deriving "showing X–Y" from the
page index would mislabel that page and every page after it. The trail accumulates the rows
actually delivered instead (`currentRowsBefore`), which is what
`logCursor.test.ts › places each page from the rows actually delivered` pins.

### 5.4 Two routes, not a mode on the legacy route

`GET /api/log/cursor` and `GET /api/log/self/cursor` are siblings of the offset routes, not
a query-parameter mode of them. A conditional `total` on the legacy endpoint would be an
observable contract break by construction; two routes make the contract true by construction.
`router.TestApiRouterRegistersCursorRoutesBesideLegacyOnes` builds the **real** API router
and asserts both additive paths and all seven legacy log paths are registered — gin panics
on a conflicting route, so building it is the conflict assertion.

## 6. Open items this bundle hands to W2.5

1. **MySQL heavy-user deep page, 218 ms with both candidates** (§4.2). MySQL picks ref access
   on `user_id` and scans every newer row the user owns. Needs an index or predicate shape
   that gets a real range on `(user_id, created_at)`.
2. **Per-engine index decisions.** The candidates are load-bearing on MySQL and SQLite and
   near-neutral on PostgreSQL. W2.5 must weigh each against its write and storage cost per
   engine rather than adding both everywhere.
3. **Index build cost at scale.** On this 2M-row corpus the builds took 1.4–2.9 s. That says
   nothing about a multi-terabyte table; W2.5 owns the dry-run plan, lock budgets, PostgreSQL
   concurrent-build failure handling, and the `AutoMigrate` interaction.
4. **Cold caches.** Every plan here was taken warm. W2.5's validation must include cold-cache
   and concurrent-insert/retention conditions, as §W2.4 of the proposal requires.

## 7. Pre-existing defects fixed alongside

W2.4 could not be built correctly on top of these, so they are fixed in the same change set.
Each is a **correction**, so each changes observable behaviour — for the better, but a reader
comparing before and after should know where to look.

### 7.1 Dashboard day buckets were server-timezone dependent

`model.dayAggregationSelect` emitted `DATE(FROM_UNIXTIME(created_at))` on MySQL and
`TO_CHAR(TO_TIMESTAMP(created_at), 'YYYY-MM-DD')` on PostgreSQL. Both render in the
**session** timezone, so every dashboard day boundary shifted with the database server's
timezone while the rest of the system worked in UTC. On a `+09:00` server, epoch
`1767225540` bucketed as `2026-01-01` instead of `2025-12-31` — every day's totals were
attributed to the wrong day, and the error was invisible on a UTC server.

It now emits timezone-independent expressions on all three engines, and it takes the handle
it queries instead of reading process-global dialect flags while querying `LOG_DB` — which
was separately wrong in a split-database deployment. Covered by
`model.TestDayAggregationSelectIsTimezoneIndependent` and `model.TestDayAggregationBucketsAgreeAcrossEngines`, which run PostgreSQL at
`Asia/Tokyo` and MySQL at `+09:00`.

### 7.2 The log list was broken outright with a separate `LOG_SQL_DSN`

`fillLogChannelNames` queried `SELECT id, name FROM channels` on `LOG_DB`. `migrateLOGDB`
creates only `Log` and `DataMigration` there, so any deployment pointing `LOG_SQL_DSN` at a
separate database had no `channels` table on that handle and **every** log-list request
failed with `query channel names for logs`. It now resolves names on the primary handle,
where channels live. Covered by `model.TestFillLogChannelNamesWorksWithSeparateLogDatabase`.

This matters directly to the proposal's premise: separating telemetry onto its own database
is one of the recommended remedies, and it did not work.

### 7.3 CSV export silently dropped ~78% of rows

`fetchAllPaginatedResults` computed `totalPages` from the *requested* batch size. List
endpoints clamp `size` to `MAX_ITEMS_PER_PAGE` (100 by default), so a request for 1,000
rows per page returned 100 and the walker still divided the total by 1,000 — asking for a
tenth of the pages and exporting a tenth of the rows, with no error. It now derives the page
count from the size the server actually applied. The regression test fails on the old code
(100 rows exported instead of 450).

## 8. Compatibility statement

- The legacy routes, their envelope, `p`/`size`/`items_per_page`, filters, sorts, default
  `id DESC` sort, inclusive `end_timestamp` and exact `total` are unchanged. No handler on
  those paths was edited.
- The capability is off by default; an operator who upgrades and changes nothing observes no
  behavioural change at all.
- No schema change, no migration, and no index is shipped by this work item.
- Berry, Air and the default frontend are untouched.
- New locale keys are additive in all five locales.
