# External UUID Backfill: Operator Runbook

Procedures for the one-time external UUID backfill: enabling and disabling the
finalizer, retrying, handling a lock timeout, recovering partial marker state,
and repointing a split LOG_DB.

Implements UUID-R08 of
[Incremental External UUID Backfill Remediation](../proposals/20260715_incremental-uuid-backfill.md).

## 1. Concepts

Rows created before the UUID-aware release have missing UUID values. Two modes
reconcile them:

| Mode | What it does | When |
| --- | --- | --- |
| Catch-up | Fills resolvable UUIDs in a bounded background worker. Writes no completion marker, promotes no index. | Default. Runs automatically on the master node while markers are absent. |
| Automatic finalization | After the worker observes sustained quiescence (no reconcilable work for the configured number of idle passes), it runs the full finalizer on its own. | Default (`EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE=true`). No operator action required. |
| Synchronous finalizer | Runs the full ordered reconciliation, promotes unique indexes, validates globally, then writes completion markers at startup, failing startup on any error. | Optional operator override, in an approved window, after every writer is UUID-aware. |

The default lifecycle is therefore fully automatic: deploy the release, catch-up
drains the backlog in the background, and finalization happens once no cycle has
found work for the quiescence window. Completion is backward compatible — every
UUID column stays nullable and unique indexes treat NULLs as distinct, so even a
pre-UUID binary keeps working after completion — and idempotent: every later
invocation is a marker-only no-op.

Completion is recorded by marker rows in `data_migrations`:

- `external_uuid_backfill_v3_primary` — always required.
- `external_uuid_backfill_v3_log` — required only when a dedicated `LOG_SQL_DSN`
  is configured.

The older `external_uuid_backfill_v2_*` rows are historical only. They never
suppress reconciliation and must never be treated as completion.

Once both applicable markers exist, startup performs only the marker lookups:
one query in unified mode, two in split mode, and no UUID target, reference, or
DDL work at all.

## 2. Configuration

| Setting | Default | Range | Purpose |
| --- | --- | --- | --- |
| `EXTERNAL_UUID_BACKFILL_FINALIZER` | `false` | bool | Selects finalizer mode. This flag is the operator's attestation that the writer barrier is complete. |
| `EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE` | `true` | bool | Lets the background worker finalize automatically after sustained quiescence. Disable to require the manual finalizer flag. |
| `EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE_IDLE_PASSES` | `3` | 1..1000 | Consecutive no-work passes required before an automatic attempt (about 15 minutes at the default idle interval). |
| `EXTERNAL_UUID_BACKFILL_ALLOW_BLOCKING_DDL` | `false` | bool | Permits the blocking MySQL fallback when `LOCK=NONE` is unavailable. SQLite does not need it. |
| `EXTERNAL_UUID_BACKFILL_MAX_ROWS_PER_CYCLE` | `10000` | 1000..1000000 | Rows one catch-up cycle may examine, counted across all phases. |
| `EXTERNAL_UUID_BACKFILL_MAX_CYCLE_DURATION` | `30s` | 1s..30m | Wall-clock ceiling for one catch-up cycle. |
| `EXTERNAL_UUID_BACKFILL_ACTIVE_INTERVAL` | `5s` | 0s..5m | Delay before rescheduling a cycle that still has backlog. |
| `EXTERNAL_UUID_BACKFILL_IDLE_INTERVAL` | `5m` | 5s..1h | Delay after a full no-work pass. |
| `EXTERNAL_UUID_BACKFILL_LOCK_TIMEOUT` | `5s` | 1s..5m | Bounded lock acquisition for migration DDL. |
| `EXTERNAL_UUID_BACKFILL_DDL_TIMEOUT` | `30m` | 1m..24h | Bounded statement timeout for migration DDL. |

An out-of-range value fails configuration loading at startup rather than being
silently clamped.

## 3. Metrics

| Metric | Labels | Use |
| --- | --- | --- |
| `oneapi_uuid_backfill_rows_total` | role, phase, target, result | Reconciliation throughput; `result="unresolved"` counts examined rows whose reference could not be resolved. |
| `oneapi_uuid_backfill_last_backlog` | role, target | `1` while catch-up still has work, `0` after a full no-work pass. |
| `oneapi_uuid_backfill_cycle_duration_seconds` | role, mode, result | Cycle duration and outcome. |
| `oneapi_uuid_backfill_finalizer_total` | role, result | Finalizer attempts and their result. |

## 4. Enabling the finalizer manually

The default deployment finalizes automatically and needs none of this section.
Follow it only when `EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE=false`, or when you
want completion at a chosen moment instead of after the quiescence window. Only
proceed after the writer barrier is genuinely complete; `IsMasterNode` is not
the barrier.

1. Confirm every active and rollback-capable writer generates owned and FK
   UUIDs. Record instance identity, version, start time, drain time, oldest
   permitted rollback version, the responsible operator, and the approval
   timestamp.
2. Drain old processes and their open transactions.
3. Confirm `oneapi_uuid_backfill_last_backlog` is `0` and no migration errors
   are being reported.
4. Back up both physical databases and complete a restore verification.
5. Record the current migration and index state.
6. In the approved DDL window, on exactly one master:

       EXTERNAL_UUID_BACKFILL_FINALIZER=true

   Restart that instance. Finalization is synchronous: startup fails if any
   phase, DDL, validation, or marker write fails.
7. Confirm both applicable markers exist, owned UUID unique indexes are present
   and valid, FK UUID indexes are present and non-unique, and there are zero
   missing owned UUIDs and zero fillable missing FK UUIDs.

## 5. Disabling the finalizer

After successful completion, remove `EXTERNAL_UUID_BACKFILL_FINALIZER` from
ordinary deployment configuration and restart. Startup is then marker-only.

Leaving the flag enabled is not harmful — repeated invocations are marker-only
no-ops once all applicable markers exist — but it should not remain in ordinary
configuration.

## 6. Retrying a failed finalization

A failed finalizer writes no marker and leaves the ordinary candidate index
intact, so the same finalizer can be rerun after the external condition is
corrected. It does not retry automatically.

| Symptom | Cause | Action |
| --- | --- | --- |
| `... is not promotable: ... duplicate owned uuid ...` | Two rows share one owned UUID. | Identify and correct the rows, then rerun. The migration never chooses which duplicate to rewrite. |
| `... malformed owned uuid requires operator remediation` | A populated owned UUID is not a canonical hyphenated UUIDv7. | Correct the rows, then rerun. Catch-up deliberately preserves the value. |
| `populated fk uuid disagrees with live owner` | A denormalized UUID does not match its owner. | Investigate the source, correct the rows, then rerun. Catch-up never silently repairs it. |
| `fillable missing fk uuid` | Reconciliation did not finish. | Rerun; if it persists, the referenced owner is being changed concurrently, which means the writer barrier is not complete. |

The validation error names the table, column, an aggregate count, and bounded
example row ids. It never logs row content.

## 7. Lock timeouts

Migration DDL runs with a bounded lock timeout (`5s` default) and statement
timeout (`30m` default) on a pinned session, and restores the session defaults
afterwards so ordinary traffic is unaffected.

A lock timeout is a retryable failure: no marker was written and the ordinary
index remains usable.

- Retry in a quieter window, or
- raise `EXTERNAL_UUID_BACKFILL_LOCK_TIMEOUT` within its range, or
- find and end the session holding the conflicting lock.

MySQL that cannot perform `ALGORITHM=INPLACE, LOCK=NONE` fails rather than
silently falling back to a blocking `ALTER`. To proceed, take a real maintenance
window and set `EXTERNAL_UUID_BACKFILL_ALLOW_BLOCKING_DDL=true`.

SQLite has no online DDL, but a single-process default deployment has no
maintenance-window concept either, so its DDL simply runs under the bounded
busy retry and context deadline. It needs no flag.

PostgreSQL builds indexes with `CREATE UNIQUE INDEX CONCURRENTLY`. A failed
concurrent build can leave an invalid same-name index behind; the next
finalizer detects it from `pg_index.indisvalid`, drops it, and rebuilds. No
dialect accepts a same-name index until metadata proves it is valid, unique, and
covers exactly the owned UUID column.

## 8. Partial marker recovery

Cross-database marker writes cannot be atomic, so partial state is expected and
recoverable. Normal split finalization writes the log marker first and the
primary marker last.

| State | Meaning | Action |
| --- | --- | --- |
| Both markers present | Complete. | None. Startup is marker-only. |
| Neither present | Not finalized. | Run the finalizer normally. |
| Log marker only | Crash between the two writes. | Rerun the finalizer. It reruns every phase and global validation, then writes the primary marker. The log marker's timestamp is preserved. |
| Primary marker only | Inconsistent: a crash, a manual deletion, or an earlier defect. | Rerun the finalizer. It logs a warning, reruns every phase and global validation, writes the log marker, and preserves the existing primary timestamp. |

An absent marker on either database always forces full global reconciliation and
validation. The other database's marker never permits skipping dependency
checks.

## 9. Repointing a split LOG_DB

A new `LOG_SQL_DSN` is a new database. Its completion is never inherited.

1. Treat the repoint as a new migration generation: the new log database must
   not carry a copied `external_uuid_backfill_v3_log` marker. If the marker was
   copied with the data, it asserts a completion that was never verified against
   this deployment — delete it before starting.
2. Start with the finalizer disabled and let catch-up reconcile the new log
   database.
3. Confirm `oneapi_uuid_backfill_last_backlog` reaches `0`.
4. Run the finalizer as in section 4, which revalidates both databases as one
   dependency graph before writing either marker.

Never copy a marker row between databases to skip this.

## 10. Rollback

During the documented rollback window, a rollback-capable writer must still be
UUID-aware. Rolling back to a writer that creates rows with missing UUID fields
after completion reintroduces gaps that the completed markers assert do not
exist.

If that happens, delete the applicable v3 markers and rerun catch-up, then
finalize again once the barrier is genuinely restored.
