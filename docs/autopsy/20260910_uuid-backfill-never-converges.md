# Autopsy: The External UUID Backfill That Could Never Finish

- Status: Fixed
- Date: 2026-09-10
- Area: background data migration / scheduler design / worker convergence
- Audience: backend engineers
- Baseline (defective) commit: `b3c18cb878aaedfe4ba33e5aec531dca6f5d5819`
- Related files: `model/uuid_migration.go`, `model/uuid_migration_fk.go`,
  `model/uuid_migration_progress.go`, `model/database_bootstrap.go`,
  `model/compact_uuid_worker.go`, `model/uuid_migration_convergence_test.go`
- Related docs: [External UUID Backfill runbook](../manuals/external_uuid_backfill.md),
  [Incremental External UUID Backfill Remediation (archived)](../proposals/archive/20260715_incremental-uuid-backfill.md)

## 1. Summary

The external UUID backfill is designed to run once. A background worker fills the UUID columns
that predate the UUID-aware release, and once it has observed that there is nothing left to
reconcile it finalizes: it promotes indexes, validates globally, writes completion markers, and
exits. Every later startup then does one marker lookup and nothing else.

In production it never got there. On the `b1` deployment the worker had run a full cycle every
five seconds since the release started, each cycle reading the same 10,000 `logs` rows,
updating none of them, and reporting that work remained. It would have kept doing that forever.

The rows it kept reading were not backlog. They were 15,030 log rows whose token had been
hard-deleted years ago, so their `token_uuid` can never be resolved. The finalizer already knew
that — its validator explicitly tolerates permanent orphans — but the scheduler never let the
finalizer run, because it counted "rows examined" as "work remaining". With more unresolvable
rows than one cycle's row budget, the worker was structurally incapable of reporting quiescence.

Two things were wrong, and each alone would have been enough to prevent completion:

1. Every cycle restarted its keyset cursor at id 0, so a backlog larger than one cycle was
   never traversed past its first budget-worth of rows.
2. Quiescence was counted per cycle rather than per traversal, so a cycle that spent its whole
   budget on unresolvable rows always looked like backlog.

The fix makes catch-up incremental across cycles and makes quiescence a property of a complete
traversal. A first pass reads the backlog once; later passes are cheap; after three quiet passes
the worker finalizes and exits for good.

## 2. Impact

No data was lost or corrupted, and no request was served incorrectly. Completion of the
migration was blocked, and the cost of not completing it was continuous.

Measured on `b1` (PostgreSQL 17.6, `shared_buffers` 128 MB, 2 GB host) over the 49 minutes
after the release started:

| Observation | Value |
| --- | --- |
| Catch-up cycles executed | 550, one every 5 to 6 seconds |
| Result of every one of them | `updated_rows: 0`, `budget_exhausted: true` |
| `logs` rows examined per cycle | 10,000, always starting from id 0 |
| Shared buffers touched per 1,000-row page | 3,140 (2,835 heap, 305 index) |
| Catalog statements per cycle | about 150 |
| Statements in one no-work cycle (SQLite equivalent) | 205, of which 150 were schema probes |
| Compact-worker prerequisite polls | 591 lock-key statements per 50 minutes |
| Application log lines produced by the two workers | roughly 100,000 per 50 minutes |
| PostgreSQL CPU attributable to the loop | about 5 % of one core |

The investigation started from the wrong end, which is worth recording. The reported symptom was
that the three largest-RSS PostgreSQL backends were idle `oneapi` connections. That turned out to
be a red herring: their 146 MB of RSS was 131 MB of `RssShmem` and 6 MB of private memory. They
had simply touched the whole shared buffer pool, because they were the connections serving this
loop. The whole PostgreSQL cgroup held 20 MB of anonymous memory. There was no memory leak; there
was a scan that never stopped.

## 3. How to reproduce

The production shape is: a table holding more permanently unresolvable rows than
`EXTERNAL_UUID_BACKFILL_MAX_ROWS_PER_CYCLE` allows one cycle to examine.

On a live PostgreSQL deployment, count them:

    SELECT count(*) FROM logs l
    LEFT JOIN tokens t ON t.user_id = l.user_id AND t.name = l.token_name
    WHERE l.user_id > 0 AND l.token_name <> '' AND l.token_uuid IS NULL AND t.id IS NULL;

If that number exceeds the row budget, the worker on the defective binary cannot finish.

In the repository, `model/uuid_migration_convergence_test.go` reproduces it in about a second:
seed 1,500 log rows whose `token_name` names a token that does not exist, set the row budget to
1,000, and run bounded catch-up cycles. On the defective code the loop never terminates; every
cycle returns `{updated: 0, budgetExhausted: true}`.

## 4. Root cause

### 4.1 The work queue had no memory

Each backfill phase opened with `lastID := 0` and paged forward with a keyset cursor
(`model/uuid_migration.go`, `model/uuid_migration_fk.go`). The cursor was a local variable, so it
lived exactly as long as one cycle. A cycle is bounded by a row budget precisely because the
backlog may be larger than one cycle should handle — and then the next cycle threw away the only
record of where the previous one had got to.

For resolvable rows this was invisible: a filled row stops matching `uuid IS NULL`, so the work
queue shrinks on its own and restarting from id 0 still makes progress. The design relied on
that, and the archived proposal says so explicitly: "Missing owned UUIDs are always fillable, so
the UUID column itself is the durable work queue."

That sentence is true for owned UUIDs and false for denormalized foreign-key UUIDs. A log row
whose token was deleted keeps matching the candidate query forever. It is not a work queue entry;
it is a permanent fixture at the front of the queue. Restarting from id 0 meant re-reading the
same fixtures every five seconds and never reaching anything behind them.

### 4.2 Quiescence was counted in the wrong unit

The worker decided what to do next from one cycle's result:

    case result.updated > 0 || result.budgetExhausted:
        idlePasses = 0
        delay = uuidCatchUpActiveInterval()

`budgetExhausted` came from `budget.spent()`, which is true whenever `examined >= maxRows`. It
says "this cycle looked at a lot of rows". The scheduler read it as "there is more work to do".
Those are different claims, and for unresolvable rows they are opposites: examining 10,000 rows
that can never be filled is not evidence of backlog, it is evidence of quiescence.

So `idlePasses` was reset on every cycle, never reached the threshold of three, and
`runAutoFinalize` — the only path to completion in a default deployment — was unreachable code in
production.

The two causes reinforce each other, which is why neither showed up in testing. Fix only the
cursor and a cycle still reports backlog whenever it fills its budget. Fix only the counting and
a settled cycle would be declared while rows behind the orphans were never examined, which is the
false-completion failure the archived proposal was written to prevent.

### 4.3 Why the existing tests did not catch it

The suite had good coverage of orphan handling and of scale, but the two never met.

- `TestOrphansAndAmbiguityDoNotBlockCompletion` proves orphans do not block the **finalizer**. It
  uses four log rows. Four is less than the 1,000-row minimum budget, so the pathological case
  cannot arise.
- `uuid_scale_test.go` drives large backlogs through `drainCatchUp`, but every row in the fixture
  is resolvable, so the column-as-work-queue assumption holds and it converges.

The missing test is the intersection: *more unresolvable rows than one cycle's budget*. That is
now the first test in the regression file.

### 4.4 Two multipliers found on the way

Neither of these blocked completion, but both made the loop expensive, and one of them could have
blocked completion on a slower database.

**Per-cycle schema probing.** `validateSchema`, `ensureUUIDCandidateIndexes`, and the per-target
`HasTable`/`HasColumn` checks ran on every cycle. A schema does not change under a running
process, so these were 150 catalog statements per cycle, about 73 % of everything a no-work cycle
issued.

**A deadline inside a transaction.** When a cycle's wall-clock budget expires mid-transaction,
`database/sql` rolls that transaction back on its own and the next statement returns
`sql.ErrTxDone`, not `context.DeadlineExceeded`. `cycleWindowExpired` matched only the latter, so
the normal end of a time-boxed cycle surfaced as a hard error, logged at ERROR and resetting the
quiescence streak. On a database slow enough for that to happen regularly, this alone would have
prevented finalization — the same defect wearing a different error. It was found by a test written
for the fix, not by inspection.

**Compact-worker polling.** The compact UUID worker waits for these same markers. Its
`waiting_prerequisite` state fell through to the active interval, so while the external migration
was stuck it re-acquired an advisory lock and re-read both marker sets every five seconds, with
nothing it was permitted to do.

## 5. Fix

### 5.1 A pass is now a first-class thing

`model/uuid_migration_progress.go` introduces `uuidCatchUpProgress`, owned by the topology, which
is exactly one database installation for one process generation. It holds a keyset cursor per
scan — a scan being one table, column, and missing-value predicate on one database role — plus
which scans have reached the end of their candidate set.

A **cycle** stays what it was: one bounded run. A **pass** is a complete traversal of every scan,
and may span several cycles. Restarting the process legitimately costs one pass; nothing is
persisted to the database, which was a deliberate non-goal of the original design and still is.

### 5.2 Backlog now means "not yet traversed"

`uuidMigrationResult` gained `passComplete` and `passUpdated`, and `budgetExhausted` was redefined
as "this cycle stopped before the pass was complete". The worker schedules on passes:

| Cycle outcome | Delay | Quiescence streak |
| --- | --- | --- |
| pass not complete | active | unchanged |
| pass complete, wrote rows | active | reset |
| pass complete, wrote nothing | idle | +1, finalize at the threshold |
| error | idle | reset, and the pass is abandoned |

A pass that examines a million unresolvable rows and writes nothing is now one idle pass, which is
what it always should have been.

### 5.3 The cursor records committed progress

`advanceScan` is called after the batch's write succeeds, never before it. A cycle interrupted by
its deadline therefore re-reads that batch instead of stepping over rows it never wrote. Rows that
were written no longer match the candidate predicate, so the re-read is cheap.

### 5.4 The shape is read once

Schema validation, candidate-index assurance, and target presence are memoized for the worker's
lifetime. Any error clears the memo, so a retry still re-runs the failed step. The finalizer keeps
re-checking everything on every invocation: it is the completion authority and it runs rarely.

An idle cycle went from 205 statements (150 of them catalog probes) to 55 statements and no
catalog probes at all.

### 5.5 Waiting instead of polling

`waiting_prerequisite` now reschedules at the idle interval, and `signalCompactPrerequisite()`
wakes the compact worker within a second of the external UUID markers being written — from both
the automatic and the operator-driven finalizer.

### 5.6 `sql.ErrTxDone` is a deadline in disguise

`cycleWindowExpired` now accepts `sql.ErrTxDone` alongside `context.DeadlineExceeded`, but only
when this cycle's own context has actually expired. The existing guard that keeps a nested DDL
statement timeout surfacing as a real failure is untouched.

## 6. Verification

Every defect was reproduced behaviorally before it was fixed, and the reproduction is now the
regression suite (`model/uuid_migration_convergence_test.go`, 12 tests).

Predicted before running against the defective baseline: five red, one green. Observed: exactly
that, which is what rules out a misattributed cause.

| Test | Baseline | Fixed |
| --- | --- | --- |
| `TestCatchUpSettlesWhenOnlyOrphansRemain` | FAIL: five consecutive `{updated:0 budgetExhausted:true}` | PASS |
| `TestCatchUpWorkerFinalizesWhenOrphanBacklogExceedsCycleBudget` | FAIL: marker never written in 20 s | PASS |
| `TestCatchUpExaminesEachOrphanOncePerPass` | FAIL: 1,000 then 1,000 rows for 1,500 orphans | PASS |
| `TestCatchUpSteadyStateCycleIssuesNoSchemaProbes` | FAIL: 150 probes of 205 statements | PASS, 0 of 55 |
| `TestCompactWorkerWaitsForPrerequisiteAtIdleCadence` | FAIL: returned 5 s, not 5 m | PASS |
| `TestCatchUpNewPassPicksUpLateResolvableRows` | PASS (over-correction guard) | PASS |

Six more tests were added with the fix, covering the split log topology, cursor safety under an
aggressive time budget, pass restart after a failed cycle, a restarted worker starting fresh, the
backlog gauge, the prerequisite wake-up, and the budget-versus-quiescence distinction.

A separate check confirmed the diagnosis from the other direction: on the **defective** code, the
identical 1,500-orphan fixture with a 3,000-row budget settles after three cycles and finalizes.
The hang really is the budget-versus-orphan interaction and nothing else.

### 6.1 Mutation checks

A red test is only worth what it pins. Each behavioral change was reverted individually and the
suite re-run; a change whose reversion left everything green would mean the test suite was not
actually holding it.

| Reverted change | Tests that turned red |
| --- | --- |
| Worker's pass-based decision table | `TestCatchUpWorkerFinalizesWhenOrphanBacklogExceedsCycleBudget` |
| `stopScan` marking the pass incomplete | `TestCatchUpExaminesEachOrphanOncePerPass`, `TestCatchUpPassCompletionDistinguishesBudgetFromQuiescence` |
| Cross-cycle cursors | 5 tests |
| Index-assurance memo | `TestCatchUpSteadyStateCycleIssuesNoSchemaProbes` |
| Compact idle cadence | `TestCompactWorkerWaitsForPrerequisiteAtIdleCadence` |

The first two mutations were initially too weak to discriminate — both left the suite green — and
were rewritten until they did, which is the check working as intended.

### 6.2 Regression

The whole `model` package was run under the race detector with
`ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1`, so the compact UUID suite runs rather than skipping:
`ok github.com/Laisky/one-api/model 482.3s`, no failures and no data races. The convergence suite
was additionally run three times over under `-race` to check it is not timing-sensitive.

One test needed rewriting after that run. `TestCatchUpTimeBudgetNeverSkipsResolvableRows`
originally required an aggressively time-boxed catch-up to converge on its own, and under the race
detector a fifteen-millisecond cycle may not commit a single batch, so it never did. That is
correct behavior, not a defect — the configuration validator enforces a one-second floor on
`EXTERNAL_UUID_BACKFILL_MAX_CYCLE_DURATION` — but the test was asserting something
machine-dependent. It now asserts only what is not: interrupted cycles are not errors, and once
cycles are allowed to finish, every row is filled exactly once.

## 7. Operational note

A deployment already stuck on the defective binary is unblocked by the upgrade alone: the first
pass traverses the unresolvable rows once, three quiet passes follow, and the worker finalizes and
exits. No flag, no manual step, no data change.

Without upgrading, raising `EXTERNAL_UUID_BACKFILL_MAX_ROWS_PER_CYCLE` above the unresolvable-row
count lets a single cycle traverse them and reach quiescence; `EXTERNAL_UUID_BACKFILL_FINALIZER=true`
for one restart is the other option. Both are documented in the runbook.

Unrelated but worth acting on: `b1` runs with `DEBUG_SQL=true`, which emits every statement and was
the dominant contributor to the log volume in section 2.

## 8. Lessons

### 8.1 "The data is the work queue" is an assumption, not a fact

Using the target column as the queue is elegant and correct exactly when finishing a row removes
it from the queue. Any row that can never be finished breaks it, silently, by staying at the front
forever. If a queue can contain items that will never leave, the traversal needs its own memory of
where it has been.

### 8.2 A budget is not a backlog signal

"I stopped because I hit my limit" and "there is more work" are different statements. Conflating
them is easy because they coincide in the common case, and the failure only appears when the limit
is spent on work that will never complete. Name and compute the two separately.

### 8.3 A terminating condition needs its own test

The suite tested that the backfill fills rows, tolerates orphans, and scales. Nothing tested that
it *stops*. Any component whose contract includes "and then it finishes" needs a test that lets it
finish, sized so that finishing is not automatic.

### 8.4 Test the intersection, not the axes

Orphans were tested at small scale. Scale was tested without orphans. The bug lived at the
intersection, where the number of unresolvable rows crosses a configured bound. When a threshold
separates two tested behaviors, the case that straddles it is the one worth writing.

### 8.5 The first symptom is not the system

The investigation opened on "three PostgreSQL backends are using too much memory". The memory was
shared buffers counted once per process, and entirely normal. Splitting `RssAnon` from `RssShmem`
took one command and redirected the whole investigation. Check what a number means before
explaining why it is large.

### 8.6 One event can arrive as several different errors

A deadline that fires inside a transaction arrives as `sql.ErrTxDone`. Matching only the obvious
sentinel turned a routine cycle boundary into an ERROR that reset the very counter the system
needed to advance. When classifying an error by cause, ask what else that cause can look like by
the time it reaches you.

## 9. Verification record

| Check | Command | Result |
| --- | --- | --- |
| Reproduction on the defective baseline | `go test ./model -run 'TestCatchUp\|TestCompactWorkerWaits' -count=1 -v` at `b3c18cb8` | 5 fail, 1 pass, as predicted |
| Convergence suite on the fix | same, on this commit | 12 pass |
| Mutation checks | 5 targeted reversions | each turns at least one test red |
| Full package under the race detector | `ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1 go test ./model/ -race -timeout 25m -count=1` | `ok ... 482.3s`, no failures, no races |
| Convergence suite repeated | `go test ./model -race -count=3 -run 'TestCatchUp\|TestCompactWorker'` | `ok ... 31.0s` |
| Whole repository | `go vet ./...` and `go test -race -count=1 ./...` | clean, 104 packages |
| Build | `go build ./...` | clean |

### 9.1 What a fixed deployment looks like

After the upgrade, `b1` should stop logging `external uuid reconciliation started` within roughly
one pass plus three idle intervals, log `external uuid migration completed automatically` exactly
once, and carry the applicable `external_uuid_backfill_v3_*` rows in `data_migrations`. A restart
after that performs one marker lookup and starts no worker. Cycle cadence, before and after:

    docker logs vps_oneapi_1 --since 30m 2>&1 | grep -a "external uuid reconciliation started" \
      | awk '{print $1}' | tail -16 | while read t; do date -d "$t" +%s; done \
      | awk 'NR>1{print $1-p}{p=$1}' | tr '\n' ' '
