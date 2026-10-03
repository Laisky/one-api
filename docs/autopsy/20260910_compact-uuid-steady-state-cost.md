# Autopsy: The Completed Compact UUID Migration That Kept Re-Proving Itself

- Status: Fixed in the working tree, not yet deployed
- Date: 2026-09-10
- Area: background data migration / post-completion audit / query planning
- Audience: backend engineers
- Baseline (defective) commit: `e000b8eb2791`
- Related files: `model/compact_uuid_steady_state.go`, `model/compact_uuid_migration.go`,
  `model/compact_uuid_backfill.go`, `model/compact_uuid_coordinator_state.go`,
  `model/compact_uuid_steady_state_test.go`, `model/compact_uuid_steady_state_live_test.go`
- Related docs: [The External UUID Backfill That Could Never Finish](./20260910_uuid-backfill-never-converges.md),
  [Automatic Compact UUID Storage](../manuals/compact_uuid_storage.md),
  [Compact UUID Storage proposal (archived)](../proposals/archive/20260715_compact-uuid-storage.md)

## 1. Summary

This is the second half of one investigation. The first autopsy fixed the external UUID backfill,
which could never finish. Once that fix shipped, both migrations completed within an hour: the
external UUID markers at 05:14 and the compact storage markers at 05:56. One fat PostgreSQL
backend remained.

It was the compact UUID worker. Completion had worked; what came after completion was wrong. The
compact migration deliberately keeps running once done, because its shadow columns are derived
data that can drift if someone drops a trigger. The specification says what that continuing work
is: verify object metadata, probe for NULL backlog "in one indexed read", and advance a *bounded*
rolling scan. The code did all three, and then on every idle interval it also:

1. re-ran the full validation traversal of every row of all 27 targets — the completion gate,
   although the markers it gates already existed; and
2. issued a NULL-backlog probe that PostgreSQL answered by walking the primary key through the
   whole table, because of how the query was ordered.

Every five minutes, forever, on every deploy's master. Each sweep pulled about 1.5 million buffer
pages through a 128 MB pool, so the backend serving it touched all of shared memory in about one
second. That was the idle process in `ps`.

The fix went through two designs. The first made the completed worker do what the specification
said: catalog audit, NULL probes on all 27 targets, and the bounded rolling sweep. That cut the cost
about 125-fold, and it was still wrong for the product. The owner's review: external UUIDs are a
one-time migration; once the historical rows are repaired, every new write derives its own shadow;
periodic scanning buys nothing, and a deployment with millions of rows cannot afford it; anything
that must be checked should be checked through an index at negligible cost.

The reviewed final design follows that while retaining the proposal's bounded rolling audit. After
completion the worker reads the catalog, seeks the empty NULL range of each owned UUID index,
repairs rows the local read path proved wrong by primary key, and advances bounded equality pages
with rotating target priority. A full traversal happens only while recovering from observed drift.

## 2. Impact

No data was wrong and no request failed. The read path verifies every compact candidate against
its authoritative text, so the worker's extra work protected nothing the application relies on.
The cost was pure I/O, plus a buffer pool flushed every five minutes, evicting the working set of
real traffic on a 2 GB host.

Measured on `b1`, PostgreSQL 17.6, `shared_buffers` 128 MB, after both markers existed:

| Observation | Value |
| --- | --- |
| Backend growing from 7 MB to 134 MB `RssShmem` | within one second, about five minutes after boot |
| Buffer accesses in a four-minute window holding one cycle | 1,496,905 (about 11.7 GB of page visits) |
| Pages actually read into the pool in that window | 180,606 (about 1.4 GB) |
| `logs` heap-block visits in that window | 551,937, about 20 times the table's 28,285 pages |
| One NULL probe on `logs.channel_uuid` | 68,617 buffers, 211 ms, to return zero rows |
| NULL probes, all 27 targets, one cycle | 344,049 buffers, 1.07 s |
| Full validation traversal, one cycle | about 1.15 million buffers |

## 3. How to reproduce

After both markers exist, sample the backends and the active queries for one idle interval:

    # per-backend shared memory, once a second
    for p in $(pgrep -f "postgres: .* oneapi"); do
      awk -v p=$p '/^(RssAnon|RssShmem)/{printf "%s %s %s ", p, $1, $2} END{print ""}' /proc/$p/status
    done

A fresh backend jumps to the size of `shared_buffers` while running statements shaped like:

    SELECT "id" AS id, "uuid" AS legacy_value, "uuid_compact" AS compact_value
    FROM "logs" WHERE "id" > $1 AND "id" <= $2 ORDER BY "id" ASC LIMIT $3

That is the validation traversal. Its companion probe shows the planner defect directly:

    EXPLAIN (ANALYZE, BUFFERS)
    SELECT "id" AS id, "channel_uuid" AS legacy_value, "channel_uuid_compact" AS compact_value
    FROM "logs" WHERE "channel_uuid_compact" IS NULL AND "channel_uuid" IS NOT NULL
      AND "channel_uuid" <> '' AND "id" > 0 ORDER BY "id" ASC LIMIT 200;
    -- Index Scan using logs_pkey ... Rows Removed by Filter: 273892

In the repository, `model/compact_uuid_steady_state_test.go` reproduces the traversal half on
SQLite, and `TestCompactGapProbeCostLive` reproduces the planner half on real MySQL 8.4 and
PostgreSQL 17.

## 4. Root cause

### 4.1 The completion gate was also the routine audit

`runCompactReconciliation` ended every cycle the same way. If the cycle repaired nothing, it ran
`runCompactValidationPass`, a complete traversal of all 27 targets up to their high-water marks,
and handed the report to `applyCompactValidation`. That function is the completion gate: two clean
passes in the same epoch, fingerprints, then markers. With markers present it simply answered
`ready` — after paying for the whole traversal to reach that answer.

Nothing in the specification asks for that. Section 8.5 defines the clean pass as the completion
gate; section 8.6 defines the post-completion cycle as object metadata, NULL-backlog probe, and a
bounded rolling scan, with "a fresh full audit" reserved for restoring `ready` after drift. The
code had one path for both, so the gate ran as routine.

A restart made it worse. The clean-pass epoch resets on restart, so a fresh master needed two full
traversals before it would report `ready` again, on every deploy. Non-master processes, meanwhile,
became ready for compact reads on their object audit alone. The master was holding itself to a
standard the read path had never needed.

### 4.2 `ORDER BY id LIMIT n` persuaded the planner to walk the table

The NULL-backlog probe was written as a keyset page:

    WHERE <compact> IS NULL AND <legacy> IS NOT NULL AND <legacy> <> ''
      AND id > ? ORDER BY id ASC LIMIT ?

Its comment says it goes "through the compact index… in one indexed read". On production it did
not. PostgreSQL estimates the predicate by multiplying selectivities as if the columns were
independent: about 7.8 % of shadows are NULL and about 92 % of legacy values are not, so it
expected 22,640 matches. At LIMIT 200 that meant finding its rows in the first 1 % of id order,
which made walking `logs_pkey` look far cheaper than fetching 22,000 rows through the compact
index and sorting them.

The columns are not independent. A shadow is NULL precisely when its legacy value is NULL; that is
how the trigger derives it. So there were no matches, and the walk read all 273,892 rows to prove
it — per column, per target, per cycle.

The trap has a clean shape. The planner walks the primary key when
`nullShadowRows² >> LIMIT × tableRows`. `b1` sits about nine times past that line.

### 4.3 Why the existing tests did not catch it

- The compact suite's SQLite fixtures are small, and SQLite's planner is not PostgreSQL's. The
  traversal ran there too, but no test asked what a *completed* worker's cycle should cost.
- The live suite drove every engine to `ready` and checked correctness. It never measured the
  cycle after `ready`, and its fixtures sat far below the planner's crossover.
- "Continues after completion" was tested as a property: drift is detected and repaired, and it
  is. What it costs to continue was never stated, so never tested.

## 5. Fix

### 5.1 The decision: completed means completed

Correctness never depended on the post-completion audit. Every compact read verifies its candidate
against the authoritative text and falls back to the text index when they disagree. Every write
derives its shadow in the database, through the trigger, in the same statement. What can still go
wrong is that the machinery stops working, or that something bypasses it. Both can be detected
without reading rows.

### 5.2 The steady state

`runCompactReconciliation` sends a completed installation — compact markers present, external UUID
markers still present, no drift observed by this process — to `runCompactSteadyState`:

1. **`validateCompactObjects`.** Columns, triggers, indexes, and the legacy index manifest, from
   the catalog. PostgreSQL's check includes `tgenabled`, so a disabled trigger counts as drift,
   not only a dropped one.
2. **`repairCompactRowsFromLookups`.** When a compact lookup falls back and the text index then
   finds the row, that row's shadow is proven missing or wrong. On a mismatch, the row the shadow
   wrongly nominated is proven wrong too. The lookup queues those ids, bounded at 256 and never
   blocking the request, and the worker repairs each through one primary-key read and at most one
   conditional update. It makes two passes, so that a row whose repair collides with another row's
   stale shadow is retried after that row is fixed.
3. **`compactTargetHasGap` on the 12 owned targets only.** An owned UUID is never legitimately
   NULL, so on a clean table the probe reads an empty index range. It catches the one drift the
   catalog cannot. `pg_dump --data-only --disable-triggers` is a supported restore, and it enables
   the triggers again when it finishes. Rows it loaded leave their owned shadow NULL along with the
   rest, so this probe sees them.
4. **Bounded rolling equality pages with rotating target priority.** This restores eventual
   detection for wrong non-NULL values, foreign-key-only drift, and mismatches observed on another
   process while keeping each cycle within the configured row and time budgets.

Foreign-key shadows are never probed after completion. They are NULL wherever the reference is,
so an indexed probe costs as many rows as there are NULL references; the rolling equality audit
covers them instead.

Audit-required state is stored as a namespaced control row in `compact_uuid_manifests`, separately
from immutable completion markers. It is written before recovery mutation and cleared only after
two clean full passes, so restarts and ownership handovers cannot lose evidence of drift.

The external UUID markers are part of the steady-state condition on purpose. Rolling back that
migration deletes them, and the compact worker has always answered by leaving `ready` for
`waiting_prerequisite`, which also disables compact reads.

### 5.3 A probe the index can answer

The owned probe asks only whether a gap exists, ordered by the compact column:

    SELECT id FROM t WHERE uuid_compact IS NULL AND uuid IS NOT NULL AND uuid <> ''
    ORDER BY uuid_compact ASC LIMIT 1

Only the compact index can supply that order, and it also serves the `IS NULL` predicate, so every
engine reads from it and stops at the first hit.

The recovery path keeps its id-ordered keyset read. Paging repairs by `(compact, id)` would force
PostgreSQL, whose single-column index carries heap order rather than id order, to sort every gap
row on every batch of an initial backfill. "Is there any?" and "give me the next page" are
different questions, and each query is shaped for the case it runs in.

### 5.4 The request path stops waking the worker for nothing

Before this change every compact miss signalled the worker, including lookups for identifiers that
exist nowhere, and every signal woke it into a full traversal. So a stream of requests for unknown
UUIDs could keep the database scanning back to back. Now a lookup signals only when it has queued a
row that exists. An unknown identifier falls back, fails in the text index, and costs nothing else.

## 6. Verification

Every defect was reproduced behaviorally before any code changed. The reproduction is now the
regression suite.

| Test | Engine | Pre-fix | Fixed |
| --- | --- | --- | --- |
| `TestCompactReadySteadyStateCycleIsIndependentOfTableSize` | SQLite | FAIL: 4 traversal pages at 300 users, 16 at 1,500 | PASS, 0 and 0 |
| `TestCompactRestartWithMarkersReachesReadyWithoutFullTraversal` | SQLite | FAIL: `validating`, not `ready` | PASS |
| `TestCompactSteadyStateStillDetectsAndRepairsDrift` | SQLite | FAIL only at the last step, because detection and repair already worked | PASS |
| `TestCompactSteadyStateUsesOnlyBoundedRollingScans` | SQLite | FAIL: full traversal in every ready cycle | PASS: bounded pages only |
| `TestCompactSteadyStateDetectsTriggerBypassedRows` | SQLite | new guard | PASS |
| `TestCompactLookupMismatchRepairsExactlyThoseRows` | SQLite | FAIL: repair only via full traversal | PASS: 2 rows, by primary key |
| `TestCompactUnknownIdentifierLookupsCostNothing` | SQLite | FAIL: every miss woke the worker | PASS |
| `TestCompactSteadyStateHonorsExternalUUIDRollback` | SQLite | new guard | PASS |
| `TestCompactGapProbeCostLive` | MySQL 8.4 | FAIL: foreign-key probe examined 20,001 rows of 20,000 | PASS: no foreign-key probe; owned probe a seek |
| `TestCompactGapProbeCostLive` | PostgreSQL 17 | FAIL: foreign-key probe examined 20,000 rows of 20,000 | PASS: no foreign-key probe; owned probe examined 0 rows |

### 6.1 The restore contracts, on real engines

The replication-and-restore suite is gated behind `COMPACT_UUID_TEST_REPLICATION=1`, which no
workflow sets, so no CI run has exercised the restore contracts. Its two restore-mode arms were run
locally against PostgreSQL 17 and MySQL 8.4, with fresh servers started through docker as the suite
intends.

| Restore mode | PostgreSQL | MySQL |
| --- | --- | --- |
| Pre-migration dump into a fresh legacy schema | PASS | PASS |
| Completed dump into a fresh server, now `ready` on the first cycle | PASS | PASS |
| Trigger-disabled data-only restore, repaired automatically | PASS | PASS |

The completed-dump case previously asserted `validating` on the first cycle: a restored, finished
installation had to re-prove itself with two full traversals. It now asserts `ready`, with no
traversal and no rewrite. That is the owner's decision made explicit in a test.

The data-only case is the one the owned probe exists for. With the owned probes removed, the
PostgreSQL arm fails: `pg_dump --data-only --disable-triggers` re-enables the triggers, the catalog
verifies, and nothing else would notice. MySQL's arm passes either way, because MySQL has no
`DISABLE TRIGGER` and its restore shape leaves the triggers dropped, which the catalog audit sees.

### 6.2 Two false positives on the way

The first live test passed on the defective code, and so did the second. Both were false
positives, and the method caught them.

- **Narrow rows.** The fixture's rows were a few dozen bytes, so random heap fetches through the
  compact index looked cheap and both planners chose it. Production rows average 789 bytes.
- **The crossover.** With production-width rows, PostgreSQL still chose the index, because the
  fixture sat exactly on `nullShadowRows² ≈ LIMIT × tableRows` (2,000² against 200 × 20,000).
  Production sits nine times past it. Seeding enough rows to match would have taken 180,000 wide
  rows per engine. Pinning the probe's batch to 20 for the measured cycle moved the fixture ten
  times past the line instead, and both engines then walked the table exactly as `b1` does.
  `COMPACT_UUID_BATCH_SIZE` is an operator setting, so that regime is reachable in production at
  any table size.

One measurement also needed correcting in the other direction. MySQL's first reading, 6,001 rows,
looked like a failure but was an index-merge intersection counting each row about three times.
`EXPLAIN ANALYZE` showed an index range scan. The bound is therefore relative to the table: at most
four times the NULL-shadow rows, which the fixture keeps well under half the table.

### 6.3 Mutation checks

Each behavior was reverted on its own; each reversion turned at least one test red.

| Reverted change | Tests that turned red |
| --- | --- |
| Steady-state dispatch, the pre-fix worker | the steady-state, restart, drift, bounded-scan, and mismatch tests |
| `requireFullAudit` made a no-op | `TestCompactSteadyStateStillDetectsAndRepairsDrift` |
| Full audit forced on every new coordinator | `TestCompactRestartWithMarkersReachesReadyWithoutFullTraversal` |
| Owned probe ordered by `id` | `TestCompactGapProbeCostLive/postgres` |
| External UUID prerequisite dropped from the gate | `TestCompactSteadyStateHonorsExternalUUIDRollback` |
| Foreign-key targets use NULL probes | `TestCompactSteadyStateUsesOnlyBoundedRollingScans` |
| Owned probes removed, catalog-only | `TestCompactSteadyStateDetectsTriggerBypassedRows` |
| Lookups queue no repair | `TestCompactLookupMismatchRepairsExactlyThoseRows` |
| Every miss wakes the worker, the pre-fix request path | `TestCompactUnknownIdentifierLookupsCostNothing` |

The owned-probe ordering mutation stays green on MySQL, whose optimizer answers the `LIMIT 1` form
through the index either way. The PostgreSQL arm, where production runs, is what pins it.

### 6.4 Production estimate

The probes were run against `b1` through read-only `EXPLAIN (ANALYZE, BUFFERS)`:

| Per idle-interval cycle | Before | First design | Final design |
| --- | --- | --- | --- |
| Full validation traversal | about 1.15 million buffers | not run | not run |
| NULL probes | 344,049 buffers, 1.07 s, 27 targets | 9,256 buffers, 81 ms, 27 targets | 271 buffers, 10 ms, 12 owned targets |
| Rolling sweep | one batch per target | one batch per target | bounded pages with rotating target priority; remeasurement pending |
| Read-path repairs | none | none | one primary-key read per proven row |

Of the owned probes, eleven cost 1 to 5 buffers each. `traces.uuid` cost 241, all of it dead index
entries left by trace retention's deletes, which vacuum reclaims. The first design's remaining
9,256 buffers were almost all foreign-key probes reading legitimately NULL references, 8,985 in
total and 5,159 for `logs.channel_uuid` alone. On a table with a million rows and a tenth of
them without a token, that would have been a hundred thousand rows every five minutes. That is the
cost the owner's review removed.

The final NULL-probe measurements remain valid. The review follow-up restored bounded equality
pages to cover cross-process, non-NULL, and foreign-key drift, so the 271-buffer figure is not a
complete cycle total; live remeasurement of the combined cycle is still required.

### 6.5 A pre-existing break found on the way

Running the whole `model` package against live engines, as `.github/workflows/pr.yml` does, turned
up three failures that had nothing to do with this change and failed identically on the untouched
baseline `e000b8eb`: `TestCompactUUIDOldBinary`, `TestCompactUUIDOldBinaryDrift`, and all six rounds
of `TestCompactUUIDMixedVersionDrift`. Commit `32df60cf` had replaced
`idx_async_task_bindings_last_accessed_at` with a composite index, and the pinned rollback builds,
which still declare the old one, re-create it. It was fixed separately by stating the rollback
contract exactly; see
[A Better Index That Broke the Rollback Tests Nobody Ran](./20260910_rollback-contract-superseded-index.md).

### 6.6 One unexplained intermittent failure

In eleven runs of `TestCompactGapProbeCostLive` against MySQL, one failed inside
`driveCompactToReady` with an error from a worker cycle. That is the initial migration, before any
marker exists, where this change only sets a boolean and the steady state cannot run. The error
text was not captured, and the next ten runs passed. It is recorded here as unexplained rather than
attributed, since the full live suite drives MySQL through the same path many times.

## 7. Lessons

### 7.1 "Keeps running after completion" needs a cost, not only a behavior

The design was right to keep auditing derived data after completion. What it never stated was how
much that auditing may cost. A background loop that runs forever needs a per-iteration budget
written into its contract, and a test that measures it, because "forever" multiplies whatever it
costs.

### 7.2 Don't reuse a gate as a heartbeat

The completion gate proves a strong claim once. The heartbeat notices that the claim stopped being
true. They need different evidence at different prices, and one function serving both will
charge the gate's price on every heartbeat.

### 7.3 The planner does not know your invariant

`compact IS NULL` and `legacy IS NOT NULL` are nearly mutually exclusive by construction, and the
planner cannot know that. Any `ORDER BY pk LIMIT n` query whose filter is rare in reality but
common by independent estimate is a full-table walk waiting for a big enough table. Shape the query
so the only index that can serve the ordering is the selective one.

### 7.4 Ask what the check protects before making it cheaper

The first fix made the audit 125 times cheaper and kept every check the specification listed. The
better question was which of those checks protected anything. The read path verifies every
candidate, and the trigger derives every shadow, so row-level checks after completion guarded
nothing. Only the machinery itself needed watching, and the catalog shows that. Optimizing a check
is the second step. Deciding whether it should exist is the first.

### 7.5 A reproduction that passes is a finding

Both early passes of the live test were false positives, and each taught something true about the
defect: row width matters, and the crossover is quadratic in the NULL count. Had the first pass
been taken as "PostgreSQL is fine here", the fix would have shipped untested on the only engine
that fails.

## 8. Verification record

| Check | Command | Result |
| --- | --- | --- |
| Reproduction on the defective baseline | SQLite and live tests above, at `e000b8eb` | red for the stated reasons |
| Fix | same, on the working tree | all green |
| Mutation checks | nine targeted reversions | each turns at least one test red |
| Restore contracts | `COMPACT_UUID_TEST_REPLICATION=1 go test ./model -run 'TestCompactUUIDReplicationAndRestore/(postgres\|mysql)/restore-modes'` | 6 of 6 pass on PostgreSQL 17 and MySQL 8.4 |
| Production estimate | read-only `EXPLAIN (ANALYZE, BUFFERS)` of both probe shapes on `b1` | table in 6.4 |
| Whole repository, live MySQL 8.4 and PostgreSQL 17, race detector | `go test -race -count=1 ./...` in the environment of `.github/workflows/pr.yml` | 104 packages pass, no data races; in `model`, only the three pre-existing old-binary failures of 6.5 |
| Build and vet | `go build ./... && go vet ./...` | clean |
