# Automatic Compact UUID Storage

- Status: Implemented
- Migration generation: `compact_uuid_storage_v1`
- Proposal: [Automatic Compact UUID Storage Implementation Handbook](../proposals/20260715_compact-uuid-storage.md)
- Related: [External UUID Backfill](./external_uuid_backfill.md)

## 1. What this is

Compact UUID storage adds a **database-derived shadow column** beside every external UUID
column, so exact UUID lookups can use a 16-byte index instead of a 36-character text index.

Two things are worth being precise about, because they are easy to misread:

- **The legacy text column stays authoritative, permanently.** Compact columns are additive
  shadows. Nothing drops, renames, retypes, or repurposes a legacy UUID column or its index.
- **This project does not reduce total database storage.** Both representations and both index
  families remain, because a supported pre-migration binary must keep working. Compact indexes
  are smaller than their text equivalents; the total is larger. Reclaiming the legacy storage
  is mutually exclusive with that compatibility promise and needs a separate proposal.

## 2. Operating it

There is nothing to run.

Deploy the binary. Ordinary startup detects the database state and automatically expands the
schema, installs synchronization triggers, backfills, indexes, validates, retries on transient
failure, and records completion. There is no migration command, no finalizer flag, no writer
barrier, no read-mode switch, and no maintenance window.

Deployment is therefore:

1. Deploy the new binary with default settings.
2. Confirm readiness stays healthy and `oneapi_compact_uuid_state` appears in metrics.
3. Wait for the state to reach `ready`.

Readiness never waits for migration history or DDL. If compact never completes, the
application keeps serving exactly as it did before — that is the point of the design, not a
consolation.

## 3. Rollback

Rollback is an ordinary binary deployment, at any migration percentage:

1. Stop the new binary.
2. Start a pinned pre-migration artifact. Change nothing — not the schema, indexes, triggers,
   markers, or configuration.
3. Redeploy the new binary whenever you want; it resumes from database state.

Rollback never deletes compact objects, and the old binary's `AutoMigrate` leaves them alone.
The database triggers keep deriving compact values while only the old binary runs, so an
upgrade does not inherit a backlog.

## 4. Reading the state

`oneapi_compact_uuid_state` reports exactly one state per role. In precedence order:

| State | Meaning | What to do |
| --- | --- | --- |
| `blocked_validation` | Invalid source data, or a privilege/version/topology blocker | Act — see below |
| `degraded` | A marker exists but drift was detected | Nothing; it repairs itself |
| `retry_wait` | A transient database, lock, or DDL failure is backing off | Nothing unless it persists |
| `waiting_prerequisite` | The v3 external-UUID markers are not present yet | Nothing; v3 is still running |
| `expanding` / `indexing` / `backfilling` / `validating` | Normal progress | Nothing |
| `ready` | Markers exist and the current audit is healthy | Nothing |
| `passive_legacy` | Incomplete with no eligible master, or paused | Start/promote a master |

Only two states ever need a human.

**`blocked_validation`** means something cannot be fixed without a decision. Most often it is
invalid legacy data — a NULL, empty, malformed, or non-v7 owned UUID — which cannot be repaired
without changing user data, so the migration refuses to guess. Diagnostics name the target
(`users.uuid`) and a bounded count, never the value. Correct the data and the next audit leaves
the blocked state on its own; no restart, flag, or command. Other causes are a missing trigger
privilege, a SQLite engine older than 3.41, or an unsupported topology (mixed-dialect split or
SQLite split).

**`passive_legacy` with no master** means nobody is eligible to do the work. Start or promote a
master and progress resumes from database state.

`degraded` is self-healing and deliberately not an alert-by-default: compact predicates are
disabled process-wide the moment drift is detected, the application serves authoritative text,
and the worker repairs the objects and data automatically.

## 5. Why a marker does not mean "done forever"

Unlike the v3 backfill, the compact worker **does not stop when it completes**. A marker records
that the historical installation finished; it never suppresses the audit, and it never
authorizes an unverified read.

This matters because compact values are derived data. If someone drops a trigger, restores a
dump with triggers disabled, or writes a compact column directly, the shadow can drift away
from the text. When that happens:

- requests stay correct, because every compact lookup verifies its candidate against the row's
  authoritative text and falls back to the text index on any disagreement;
- compact predicates are disabled process-wide until a fresh audit passes;
- the worker recreates the objects and repairs the data automatically; and
- the completion marker's timestamp never moves.

A compact read is only ever used by a process with its own fresh healthy audit, which expires
after twice `COMPACT_UUID_IDLE_INTERVAL`. A process that has not audited recently serves legacy
text. This is why a non-master, or a replica, is safe by default rather than by configuration.

## 6. Configuration

Every setting has a default that satisfies automatic completion. Invalid values fail startup
before the worker is created, rather than silently running on an unintended budget.

| Setting | Default | Range |
| --- | --- | --- |
| `COMPACT_UUID_AUTO_MIGRATE` | `true` | Strict boolean |
| `COMPACT_UUID_BATCH_SIZE` | `1000` | 1..1000 |
| `COMPACT_UUID_MAX_ROWS_PER_CYCLE` | `10000` | 1000..1000000 |
| `COMPACT_UUID_MAX_CYCLE_DURATION` | `30s` | 1s..30m |
| `COMPACT_UUID_ACTIVE_INTERVAL` | `5s` | 1s..5m |
| `COMPACT_UUID_IDLE_INTERVAL` | `5m` | 5s..1h |
| `COMPACT_UUID_RETRY_INTERVAL` | `30s` | 1s..30m |
| `COMPACT_UUID_LOCK_TIMEOUT` | `5s` | 1s..5s |
| `COMPACT_UUID_DDL_TIMEOUT` | `30m` | 1m..24h |
| `COMPACT_UUID_VALIDATION_TIMEOUT` | `2h` | 1m..24h |

`COMPACT_UUID_AUTO_MIGRATE=false` is an **emergency pause only**. It mutates no schema, data, or
markers, and legacy service stays fully functional — but a paused deployment cannot complete,
and is not a supported steady state.

There is no `COMPACT_UUID_STORAGE_MODE`, no compact-only reader or writer, and no manual
cutover. The runtime switch is derived from database state and this process's own audit.

## 7. Physical layout

27 targets: 12 owned UUID columns and 15 denormalized FK UUID columns, derived from the
registry in `model/uuid_migration_registry.go` rather than listed separately, so a new UUID
column cannot miss coverage.

| Dialect | Shadow type | Synchronization |
| --- | --- | --- |
| PostgreSQL 17 | Nullable native `uuid` | One `SECURITY INVOKER` `BEFORE INSERT OR UPDATE` trigger per table, pinned `search_path` |
| MySQL 8.4 | Nullable `BINARY(16)` | `BEFORE INSERT` and `BEFORE UPDATE` triggers, `UUID_TO_BIN(value, 0)` |
| SQLite 3.41+ | Nullable `BLOB` | Persistent `AFTER INSERT`/`AFTER UPDATE` triggers using core `unhex(replace(...))` |

Shadow columns are named `<legacy>_compact`. Indexes are `idx_<table>_uuid_compact_unique`
(owned, unique) and `idx_<table>_<legacy_column>_compact` (FK, non-unique).

In split mode the primary owns 23 non-log targets and LOG_DB exclusively owns the four log
targets. A stale primary `logs` table is never expanded, triggered, scanned, indexed, or marked.

SQLite requires 3.41.0 or newer because the persistent triggers use core `unhex()`. That
constraint exists because the trigger runs inside whichever SQLite each supported binary links,
including a pinned old one — a Go connection-local function would not exist in that process.

## 8. Invariants worth not breaking

- Applications never write a compact column. The database derives it from text.
- Text is never derived from compact, so there is no last-writer ambiguity.
- Invalid legacy text never makes a write fail. It stays exactly as written, derives compact
  NULL, and degrades migration health.
- Nullable FK `NULL` and empty text stay distinct in the authoritative column. Both derive
  compact NULL, and the API renders the legacy value exactly as before.
- A compact candidate is never returned unverified.
- No production path can drop, rename, or retype a legacy UUID column or index. This is enforced
  twice: a static assertion over the source and a runtime guard that fails before DDL.
