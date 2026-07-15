# Automatic Compact UUID Storage Implementation Handbook

- Status: Ready for implementation and acceptance
- Date: 2026-07-15
- Owners: Backend, database operations, release engineering, and QA
- Area: Automatic UUID shadow storage, compatibility, indexes, and migration
- Migration generation: `compact_uuid_storage_v1`
- Related: [External UUIDv7 Resource Identifiers](./20260703_external-uuid-identifiers.md),
  [Incremental External UUID Backfill Remediation](./20260715_incremental-uuid-backfill.md)
  (work item UUID-051)

## 1. Purpose and Non-Negotiable Contract

This document is the implementation and acceptance handbook for adding compact
UUID storage without requiring an operator-run migration and without breaking a
supported pre-migration binary before, during, or after completion.

The implementation must satisfy both guarantees:

1. **Zero-touch completion.** Ordinary application startup detects the database
   state and automatically expands, synchronizes, backfills, indexes,
   validates, retries, and records completion. No migration command, finalizer
   flag, writer barrier, read-mode change, or maintenance deployment is needed.
2. **Permanent backward compatibility.** A supported pre-migration binary may
   start, run its existing `AutoMigrate`, read, write, search, report, and use
   caches after compact completion with the same observable behavior it had
   before expansion.

These guarantees make the legacy text representation permanent. The migration
must never drop, rename, retype, change the nullability or collation of, or
repurpose a legacy UUID column or its existing index. Compact values are
additive, database-derived shadows used by new optimized predicates.

This means the project cannot claim lower total database storage while the
compatibility contract is active. Compact indexes must be smaller than their
text equivalents, but both representations and both index families remain.
Physical reclamation is mutually exclusive with permanent old-binary
compatibility and requires a separate, future proposal that explicitly retires
this contract.

## 2. Compatibility Boundary and Supported Source State

“Fully backward compatible” means fully compatible with named, immutable
application releases, not with arbitrary SQL clients. The implementation issue
must record:

- the oldest supported rollback build containing the v3 external-UUID writer
  contract;
- the immediately preceding production build;
- artifact checksums, Go version, embedded SQLite version, schema baseline, and
  cache version for both; and
- the UTC qualification date and the compatibility-retirement proposal, if any.

Both real artifacts must run in qualification. Raw SQL emulation is not an
acceptable substitute. Neither artifact leaves the regression matrix merely
because time passes; removal requires the separately approved proposal that
retires this compatibility contract.

The contract does not cover clients that use `INSERT INTO table VALUES (...)`
without a column list, fixed-width positional `SELECT *` scanners, unsupported
SQLite engines, or schema tools that intentionally drop unknown objects. The
project's pinned GORM binaries qualify as supported only after exact catalog
hashes prove their startup and `AutoMigrate` preserve additive columns,
triggers, indexes, checks, manifests, and markers in every migration state.

### 2.1 Source prerequisites

All applicable current-generation external UUID backfill markers must exist
before compact completion:

- unified topology: `external_uuid_backfill_v3_primary`;
- split topology: `external_uuid_backfill_v3_primary` and
  `external_uuid_backfill_v3_log`; and
- v2 markers never satisfy the prerequisite.

The existing v3 coordinator already defaults to automatic catch-up and
finalization. Bootstrap schedules that coordinator first and then starts the
compact worker. The compact worker may expand, synchronize, and backfill while
v3 is still running, but it must keep reads on legacy text and must not write a
compact completion marker until the v3 markers and its own validation pass.
Consequently, an empty or populated valid supported source with neither marker
generation still reaches both sets of markers automatically. Disabling v3
automatic finalization is an emergency override and is outside zero-touch
qualification; it must never cause a false compact marker.

Qualification covers every supported source shape:

| Source shape | Required default behavior |
| --- | --- |
| No application tables | Legacy `AutoMigrate`, v3, and compact migration complete automatically |
| Complete but empty legacy tables | Both marker generations complete automatically |
| Partially created compatible legacy schema | Legacy `AutoMigrate` completes it, then both generations complete automatically |
| Populated valid schema without v3 markers | V3 completes automatically first; compact waits safely and then completes |
| Populated valid schema with v3 markers | Compact migration completes automatically |
| Unsupported schema/dialect/topology | Compact enters `blocked_validation`; legacy readiness and behavior are unchanged |

### 2.2 Invalid data boundary

Automatic completion is guaranteed for a supported schema containing valid
legacy UUID data. Malformed owned or FK UUIDs, missing owned UUIDs, and owned
UUIDs that become duplicates after case normalization cannot be repaired
without changing user data. They therefore:

- do not block service readiness or legacy reads/writes;
- place compact migration health in `blocked_validation`;
- prevent any missing completion marker from being written;
- emit bounded aggregate diagnostics without UUID values; and
- are rechecked automatically after operator data correction, with no restart,
  command, or mode change.

Nullable FK `NULL` and empty text remain distinct in the authoritative legacy
column for exact compatibility. Both derive to compact `NULL`; APIs continue to
render the legacy value exactly as before.

## 3. Background and Reviewed Current State

The compile-time registry in `model/uuid_migration_registry.go` is the source of
truth for 12 owned UUID columns and 15 denormalized FK UUID columns. The model
tags currently store them as `CHAR(36)` and existing owned/FK indexes serve
legacy queries.

Integer primary and foreign keys remain the internal relationship mechanism.
External APIs, URLs, DTOs, caches, Gin context values, reports, and Go model
fields remain lowercase canonical UUID strings. The compact project changes
only an additive database shadow and selected lookup predicates.

The current bootstrap already provides the right lifecycle pattern:

- `InitDatabases` constructs explicit primary/log topology;
- master-only work runs outside the readiness-critical path;
- catch-up uses bounded context-aware cycles; and
- shutdown cancels and joins the worker before handles are replaced.

The compact implementation extends that pattern. It must not reuse the current
completed-marker behavior that stops the worker forever; compact markers record
historical installation, while periodic health audit and repair continue.

## 4. Locked Design Decisions

| Decision | Required implementation |
| --- | --- |
| Logical authority | The legacy text column is permanently authoritative |
| Compact write ownership | Applications never write compact columns; database synchronization and repair derive them from text |
| External representation | Existing string fields and exact API/cache payloads remain unchanged |
| Byte representation | RFC/network-order UUID bytes; no timestamp-field swapping |
| PostgreSQL storage | Nullable native `uuid` shadow |
| MySQL storage | Nullable `BINARY(16)` shadow |
| SQLite storage | Plain nullable `BLOB` shadow; triggers and audits enforce type/length |
| Shadow naming | Append `_compact` to the legacy column name; names never change |
| Schema migration | Additive automatic worker; compact fields are excluded from ordinary `AutoMigrate` DDL |
| Synchronization | Versioned persistent database triggers on every authoritative table |
| New writes | Continue writing legacy text only; triggers derive compact in the same transaction |
| New exact reads | Verified compact lookup with authoritative text fallback; never compact-only |
| Old reads/writes | Continue using text columns and legacy indexes indefinitely |
| Completion | Automatic after two clean passes plus schema/data/trigger/index validation |
| Post-completion behavior | Worker continues audit and repair; markers never suppress health checks |
| Destructive cleanup | Prohibited while this compatibility contract is active |
| Topology | SQLite unified, MySQL unified/split, and PostgreSQL unified/split; mixed-dialect split is rejected in v1 |

## 5. Exact Physical Inventory

The compact registry must be derived from `uuidOwnedRegistry` and
`uuidFKRegistry`, not duplicated as an unrelated list. Registry tests fail when
any target lacks schema, trigger, backfill, validation, index, runtime or
operational predicate, and compatibility coverage.

| Role | Table | Authoritative text | Derived compact | Compact index |
| --- | --- | --- | --- | --- |
| Primary | `users` | `uuid` | `uuid_compact` | Unique |
| Primary | `users` | `inviter_uuid` | `inviter_uuid_compact` | Non-unique |
| Primary | `tokens` | `uuid` | `uuid_compact` | Unique |
| Primary | `tokens` | `user_uuid` | `user_uuid_compact` | Non-unique |
| Primary | `channels` | `uuid` | `uuid_compact` | Unique |
| Primary | `redemptions` | `uuid` | `uuid_compact` | Unique |
| Primary | `redemptions` | `user_uuid` | `user_uuid_compact` | Non-unique |
| Primary | `token_transactions` | `uuid` | `uuid_compact` | Unique |
| Primary | `token_transactions` | `token_uuid` | `token_uuid_compact` | Non-unique |
| Primary | `token_transactions` | `user_uuid` | `user_uuid_compact` | Non-unique |
| Primary | `token_transactions` | `log_uuid` | `log_uuid_compact` | Non-unique |
| Primary | `user_request_costs` | `uuid` | `uuid_compact` | Unique |
| Primary | `user_request_costs` | `user_uuid` | `user_uuid_compact` | Non-unique |
| Primary | `traces` | `uuid` | `uuid_compact` | Unique |
| Primary | `async_task_bindings` | `uuid` | `uuid_compact` | Unique |
| Primary | `async_task_bindings` | `user_uuid` | `user_uuid_compact` | Non-unique |
| Primary | `async_task_bindings` | `token_uuid` | `token_uuid_compact` | Non-unique |
| Primary | `async_task_bindings` | `channel_uuid` | `channel_uuid_compact` | Non-unique |
| Primary | `mcp_servers` | `uuid` | `uuid_compact` | Unique |
| Primary | `mcp_tools` | `uuid` | `uuid_compact` | Unique |
| Primary | `mcp_tools` | `server_uuid` | `server_uuid_compact` | Non-unique |
| Primary | `passkey_credentials` | `uuid` | `uuid_compact` | Unique |
| Primary | `passkey_credentials` | `user_uuid` | `user_uuid_compact` | Non-unique |
| Log | `logs` | `uuid` | `uuid_compact` | Unique |
| Log | `logs` | `user_uuid` | `user_uuid_compact` | Non-unique |
| Log | `logs` | `token_uuid` | `token_uuid_compact` | Non-unique |
| Log | `logs` | `channel_uuid` | `channel_uuid_compact` | Non-unique |

In unified mode the primary database owns all 27 targets. In split mode the
primary owns the 23 non-log targets and LOG_DB exclusively owns the four log
targets. A stale primary `logs` table is never expanded, triggered, scanned,
indexed, validated, or marked.

Before the first compact DDL, normalized metadata for every pre-migration
UUID-related index is stored in a versioned, checksummed manifest in the
authoritative database. Its name, columns/order, uniqueness, predicate/prefix,
collation, visibility, and validity must remain unchanged after completion.
The worker verifies the durable manifest before every later DDL or repair; an
absent or mismatched manifest blocks compact mutation. Compact index names are:

- owned: `idx_<table>_uuid_compact_unique`; and
- FK: `idx_<table>_<legacy_column>_compact`.

## 6. Database Synchronization and Codec Contract

### 6.1 Source normalization

The shared codec accepts exactly 36 ASCII characters in case-insensitive
`8-4-4-4-12` hexadecimal form, with no surrounding whitespace; requires UUID
version 7 and RFC variant `[89ab]`; normalizes accepted input to lowercase; and
emits 16 RFC-order bytes. It never rewrites the authoritative text. Unit and
trigger parity vectors cover lowercase, uppercase, mixed case,
whitespace-padded, zero, non-v7, wrong-variant, wrong-hyphen, wrong-length, and
malformed-hex inputs.

Derivation rules are identical on every dialect:

| Legacy value | Compact result | Completion effect |
| --- | --- | --- |
| Valid owned UUIDv7 | 16-byte UUID | Valid |
| Owned `NULL`, empty, malformed, non-v7, or wrong variant | `NULL` | Blocks completion |
| Nullable FK `NULL` or empty | `NULL` | Valid terminal state |
| Valid populated FK UUIDv7 | 16-byte UUID | Valid |
| Malformed/non-v7 populated FK | `NULL` | Blocks completion |

Triggers must not make a previously accepted legacy write fail solely because
its UUID is invalid. Invalid input remains in the legacy column, derives compact
`NULL`, and degrades migration health. Current new binaries continue validating
their own normal writes before database submission.

The golden vector is fixed:

```text
canonical: 018f0000-0000-7000-8000-000000000001
hex bytes: 018f0000000070008000000000000001
```

### 6.2 Go representation

Add a compact UUID codec that:

- parses and formats the golden RFC-order representation;
- copies scan/value byte slices to avoid driver-buffer aliasing;
- scans PostgreSQL native UUID and MySQL/SQLite 16-byte values;
- binds PostgreSQL through `pgtype.UUID`/`UUIDValuer` or canonical text accepted
  as native `uuid`, and binds copied 16-byte slices for MySQL/SQLite, without a
  column-side cast or function;
- distinguishes SQL `NULL` from all-zero bytes; and
- returns wrapped errors with no UUID value in error text.

Public/shared model fields remain legacy strings, and compact columns are not
added to those structs. Exact compact lookups use private projection structs
whose backing fields use:

```text
json:"-" gorm:"column:<name>_compact;->;-:migration"
```

The read-only tag prevents application writes and prevents `AutoMigrate` from
owning compact DDL. A private projection may be used only after a capability
audit proves that its shadow column exists; legacy model queries never mention
compact columns.

### 6.3 Trigger contract

One versioned synchronization trigger set per table derives every compact field
from the final legacy text on every supported insert and update. Supported
applications always omit compact columns. A type-valid direct compact value is
overwritten where the engine permits, but malformed or type-invalid direct
compact SQL may fail before a trigger executes and is outside the compatibility
contract. Text is never derived from compact, avoiding bidirectional
last-writer ambiguity.

PostgreSQL uses trigger `cuuid_v1_<table>_sync` and function
`cuuid_v1_<table>_sync_fn`; MySQL and SQLite use
`cuuid_v1_<table>_insert` and `cuuid_v1_<table>_update`. Metadata verification
checks timing, event, table, normalized function/body hash, definer/security
properties, enabled state, and every source/target pair. A matching name alone
is insufficient.

Body canonicalization removes engine-added outer delimiters and comments,
normalizes line endings to LF, trims edges, collapses whitespace outside quoted
tokens to one byte, and lowercases unquoted SQL keywords/function names while
preserving quoted identifiers and literals byte-for-byte. Compile-time expected
hashes are versioned by dialect; semantic catalog fields are compared
separately. Restore under a different approved MySQL definer changes the
verified definer-policy result, not the canonical body hash.

Before conversion, every trigger implements the same shape check: exactly 36
characters, hyphens at positions 9, 14, 19, and 24, hexadecimal characters in
all other positions, lowercase position 15 equal to `7`, and lowercase position
20 in `8`, `9`, `a`, or `b`. Conversion functions are evaluated only after this
check succeeds. Trigger SQL is generated from compile-time identifiers and
golden-tested as normalized DDL.

| Dialect | Required synchronization and DDL |
| --- | --- |
| PostgreSQL 17 | Add nullable native `uuid` columns without defaults. Install one `SECURITY INVOKER` `BEFORE INSERT OR UPDATE` function/trigger per table with a pinned safe `search_path`. Regex-check before casting so invalid text derives NULL rather than aborting. Create compact indexes with `CREATE INDEX CONCURRENTLY` outside the expansion transaction. Bound `lock_timeout` and statement timeout. |
| MySQL 8.4 | Add nullable `BINARY(16)` columns with `ALGORITHM=INSTANT`, or `INPLACE, LOCK=NONE` when verified. Install deterministic `BEFORE INSERT` and `BEFORE UPDATE` triggers under the approved migration definer using validated `UUID_TO_BIN(value, 0)` semantics; swap flag `1` is forbidden. Create indexes with `ALGORITHM=INPLACE, LOCK=NONE` and never fall back to blocking `COPY`. |
| SQLite 3.53.2 | Add plain nullable `BLOB` columns without a `CHECK`, because checked `ADD COLUMN` can scan the table. Install persistent main-schema `AFTER INSERT` and `AFTER UPDATE` triggers using guarded core `unhex(replace(...))` plus a null-safe mismatch predicate. Trigger output and audits enforce BLOB type/length. The inner update must terminate with `recursive_triggers` both ON and OFF. Index DDL uses bounded busy retry outside readiness. |

MySQL auto-commits DDL, so a safe interval exists between column and trigger
creation. Compact reads remain disabled, triggers are installed before
backfill, and the backfill covers every row written in that interval.

SQLite compatibility requires every supported archived binary to report SQLite
3.41.0 or newer and pass a golden `hex(unhex(...))` probe. Persistent triggers
must not use Go connection-local functions because an old binary would not have
them. If a supported artifact fails the probe, implementation is blocked until
a pure core-SQL alternative is approved.

Trigger installation requires the necessary PostgreSQL/MySQL privileges and
must survive dump/restore and replication qualification. A missing privilege
blocks compact completion but not application readiness.

PostgreSQL invalid same-name indexes left by failed concurrent builds are
dropped only with verified `DROP INDEX CONCURRENTLY` and then recreated;
`IF NOT EXISTS` never proves validity. MySQL verification includes trigger
`sql_mode`, action order, and an allowed durable definer or approved restore-time
definer rewrite. Supported restore cases are pre-migration full dump to a fresh
legacy schema, completed full dump to a provisioned fresh server, and approved
data-only/trigger-disabled restore followed by automatic repair. Positional
data-only restore into an expanded schema is unsupported.

PostgreSQL physical streaming replication and MySQL row-based replication are
required modes. Compact reads on a replica remain disabled until a replica-local
equality audit passes. Any future PostgreSQL logical-replication deployment must
qualify publication of compact columns or an explicit subscriber-trigger policy
in a separate matrix before compact reads are enabled there.

## 7. Safe Runtime Lookup Contract

Legacy text remains the response and correctness source. Free-text searches,
reports, relationship rendering, caches, and ordinary model reads continue to
use legacy columns. Compact storage initially optimizes exact UUID-to-row
resolution only.

Every new exact lookup follows this algorithm:

1. Trim and parse request input, validate UUIDv7/variant, and canonicalize it for
   comparison. Invalid input returns the existing `idresolve.ErrInvalidRef`.
2. Use the compact predicate only when all applicable storage markers exist and
   the process has a fresh healthy audit.
3. Load the candidate ID through the compact index, then verify the row's
   authoritative legacy text canonicalizes to the requested UUID.
4. If the compact query returns no row or a candidate whose text disagrees,
   query the legacy text index before returning. A capability-race error may
   also fall back; inside a PostgreSQL caller transaction, bracket the compact
   probe with a savepoint and roll back to it first.
5. Never return an unverified compact candidate. Signal the background worker
   and emit a bounded gap/mismatch metric when fallback was necessary.
6. A canonical unknown UUID returns the existing `idresolve.ErrNotFound`.

This prevents both stale-not-found and stale-row results if a trigger is missing
or a compact shadow is corrupted. A committed old-binary write is immediately
visible through its text column even before repair. Cancellation, connection,
serialization, and other general database errors retain existing behavior and
are not relabeled as misses. A handled capability error is reported once by
bounded diagnostics and fallback; it is not both logged and returned.

The runtime compact-read switch is automatic and database-state-derived. There
is no `COMPACT_UUID_STORAGE_MODE`, compact-only reader, compact-only writer, or
manual cutover.

## 8. Automatic Migration Coordinator

### 8.1 Bootstrap lifecycle

`InitDatabases` remains the supported application bootstrap:

1. Initialize and `AutoMigrate` the legacy primary/log schemas.
2. Construct explicit `databaseTopology`.
3. Complete normal readiness without waiting for compact history or DDL.
4. On every process, start a lightweight read-only compact health monitor and
   default runtime lookups to legacy-safe behavior until its first audit passes.
5. On the master, start one context-bound compact worker after the v3 worker has
   been scheduled or completed.
6. Supervise v3, compact mutation, and per-process health loops with distinct
   cancel/done handles; starting one must never stop another.
7. On shutdown or handle replacement, cancel all loops, join mutation workers
   before health monitors, and only then close either database.

Non-master nodes perform no compact DDL, backfill, repair, index creation, or
marker write. They may use compact predicates only after their own fresh audit.
Every process repeats the read-only audit at the idle interval; health expires
after twice that interval, and an expired or failed audit immediately forces
legacy predicates for the affected database role through one atomic gate. This
process-local monitor is canceled and joined with the database handles.

At least one eligible master is a deployment prerequisite for progress. While
compact is incomplete, an all-non-master deployment keeps readiness and legacy
traffic in `passive_legacy`, disables compact predicates, and mutates no
migration state. Starting or promoting a master resumes automatic progress from
database state. After completion, a non-master may report/use `ready` only from
its own fresh audit.

There is no write-capable CLI. Optional status tooling is read-only and cannot
advance migration state.

### 8.2 Database-derived state machine

State is inferred from schema, trigger, data, index, marker, and current audit
health. No operator changes it.

| State | Meaning | Automatic action |
| --- | --- | --- |
| `waiting_prerequisite` | Applicable v3 marker is absent | Continue legacy service while v3 automatic work runs; recheck |
| `expanding` | Compact columns or synchronization objects are missing | Create/verify one bounded additive step |
| `backfilling` | Actionable compact gaps or mismatches remain | Repair bounded batches from authoritative text |
| `indexing` | An expanded target lacks its compact index or has a verified invalid build | Create/repair and verify the dialect-safe index before fill |
| `validating` | Schema/data/indexes/triggers appear complete | Run two clean passes and global validation |
| `ready` | Markers exist and current audit is healthy | Enable verified compact predicates and continue periodic audit |
| `degraded` | Marker exists but drift/gap/mismatch is detected | Disable compact predicates immediately, serve text, repair automatically |
| `blocked_validation` | Invalid authoritative data or privilege/version blocker | Serve text, expose aggregate blocker, and recheck automatically |
| `retry_wait` | Transient database/lock/DDL failure | Serve text and retry with bounded backoff |
| `passive_legacy` | Migration is incomplete with no eligible local owner, or automatic work is paused | Serve text; read-only audit only |

Mixed-dialect split and SQLite split enter `blocked_validation` for compact
work; they do not fail otherwise-supported legacy readiness.

### 8.3 Single-owner election and correctness

Each cycle attempts an ownership lock whose key is derived from a compile-time
namespace plus normalized database/topology identity, so unrelated one-api
databases on one server do not contend:

- PostgreSQL: session advisory lock on a pinned primary connection;
- MySQL: `GET_LOCK` on a pinned primary connection; and
- file-backed SQLite: a non-blocking OS advisory lock on a sidecar path derived
  from the canonical database path; in-memory SQLite uses a process mutex.

At most one owner per topology may start a mutating side effect. PostgreSQL and
MySQL ownership is checked before and after each side effect; connection/lock
loss cancels the cycle before another side effect. SQLite's kernel lock is
released on process exit and does not hold a database writer lock. Locking
reduces duplicate work
but is not a correctness dependency: DDL is verify-before/create/verify-after,
updates are conditional, indexes/triggers are body-verified, and marker inserts
are idempotent after duplicate classification and reread.

### 8.4 One bounded cycle

The worker performs, in order:

1. Validate topology and determine dialect per handle; reject mixed dialects.
2. Read applicable compact and v3 marker state.
3. Persist or verify the legacy-index manifest before any compact DDL.
4. Logically expand one table: add all its shadows, verify them, then install
   and verify its complete trigger set before that table is eligible for fill.
5. Create or verify one compact index for an expanded target before historical
   fill; all supported engines permit multiple `NULL` entries.
6. Backfill/repair indexed targets up to the row/time budget.
7. Run bounded schema, trigger, index, and data validation.
8. After two consecutive clean full passes, run global fingerprints and
   write missing completion markers.
9. Publish aggregate state and schedule the next cycle.

Row reconciliation uses `COMPACT_UUID_MAX_CYCLE_DURATION`. DDL and full
validation are not silently truncated by that budget: each runs outside the
row-cycle context under its own DDL or validation timeout. Lock acquisition is
still independently capped at five seconds, and no DDL may fall back to a
foreground-blocking algorithm prohibited by the dialect contract.

The worker derives each compact value from its own legacy source. It never
re-resolves relationships across databases.

Candidate reads use a per-target durable primary-key cursor, keyset order, and
at most 1,000 materialized rows. The cursor wraps after its recorded high-water
mark so an early range is never rescanned from zero while later rows starve.
Compact NULL probes and captured plans must demonstrate use of the applicable
compact index.

Rows are classified exactly once per observation:

- valid UUID plus unequal compact: set the derived bytes;
- nullable FK `NULL`/empty plus non-null compact: clear compact;
- nullable FK `NULL`/empty plus compact NULL: valid terminal state; and
- invalid/missing owned or malformed populated FK: validation blocker; clear a
  non-null compact once, but never repeat a no-op repair.

Updates recheck ID, exact observed text, and observed compact state before
replacing the derived compact value. Text is never updated.

Compact is derived data, so a verified mismatch is repaired from authoritative
text when its correct value is not occupied. A compact uniqueness permutation
that cannot be corrected row-by-row enters `blocked_validation`; it never
causes text mutation, trigger disabling, or a false marker. The worker may
overwrite an otherwise repairable compact shadow but never legacy text or a
compact value already equal to its source.

Each update statement stays at or below 900 binds, with rows calculated as
`min(200, floor((900 - fixed_binds) / binds_per_row))`.

### 8.5 Validation, fingerprints, and markers

One clean pass is a complete traversal of all 27 authoritative targets up to
per-target primary-key high-water marks captured at pass start. It records exact
examined, actionable, blocker, and updated counts and verifies trigger metadata
both before and after. The clean-pass epoch resets on restart, owner change,
topology/marker/object-version change, retry, repair, or validation error; two
passes from different epochs cannot combine. Trigger-atomic writes above or
below the high-water marks remain valid during the scans.

Global validation requires:

- every owned legacy value is a valid UUIDv7 and its compact value is equal;
- nullable FK null/empty semantics and every valid populated FK are equal;
- malformed populated values are reported and block completion;
- every compact value has the exact physical type and 16-byte representation;
- all 12 compact owned indexes are valid/unique;
- all 15 compact FK indexes are valid/non-unique;
- every synchronization object matches its versioned body contract;
- every legacy index matches its pre-expansion snapshot; and
- two consecutive passes separated by the active interval examine/update zero
  actionable rows.

Fingerprints run in a bounded repeatable-read snapshot per database role and
select legacy and compact in the same row scan. Equality streams share the
`table | id | logical-column` prefix and compare the derived semantic value:
nullable FK text `NULL` and empty both encode as derived NULL, while valid text
and decoded compact encode as the same canonical UUID. A separate raw-source
stream tags SQL NULL, empty text, and non-empty text distinctly to prove the
legacy states were observed without conflation. The equality SHA-256 digests
must match; raw-source counts/digest are retained as evidence. The
implementation never materializes a table or exposes values or digest bytes in
logs.

Final markers are:

- unified: `compact_uuid_storage_v1_primary`; and
- split: `compact_uuid_storage_v1_log` first, then
  `compact_uuid_storage_v1_primary` last.

Partial split marker state causes full global revalidation and insertion of
only the missing marker. Existing timestamps remain unchanged. Markers record
that the historical installation completed; they never stop audit or authorize
an unverified compact read. Later drift changes health to `degraded` but does
not delete or rewrite markers.

### 8.6 Continuous post-completion health

The completed worker continues at the audit interval. It verifies object
metadata, probes actionable NULL backlog, and advances a bounded rolling
equality scan. A runtime lookup fallback signals an immediate repair cycle.

If a trigger/index is dropped or an old/direct writer creates a gap:

- new requests remain correct through authoritative text fallback;
- compact predicates are disabled process-wide after detected unhealthy state;
- the worker recreates safe objects and repairs derived data when uniqueness
  permits; unsafe compact collisions enter `blocked_validation`;
- a fresh full audit restores `ready`; and
- completion marker timestamps remain stable.

A lookup fallback signal is consumed within one second and an eligible owner
starts work within one active interval. One repairable row completes within one
active interval plus one row-cycle duration; object recreation may additionally
use one DDL timeout. After invalid source data is corrected, the next audit must
leave `blocked_validation`, and completion must meet the fixture deadline in
Section 12.

There is one attempt per side effect per cycle. A transient failure schedules
the next cycle with full jitter over
`[0, COMPACT_UUID_RETRY_INTERVAL × 2^min(consecutive_failures, 5)]`; any successful durable
progress resets the counter. Retries continue across cycles until cancellation,
an explicit emergency pause, or classification as a permanent blocker. Context
cancellation interrupts waits and never starts another side effect.

### 8.7 Configuration

Automatic migration is enabled by default. Unset default configuration must
pass zero-touch acceptance.

| Setting | Default | Accepted range or values |
| --- | --- | --- |
| `COMPACT_UUID_AUTO_MIGRATE` | `true` | Strict Boolean; `false` is emergency pause only |
| `COMPACT_UUID_BATCH_SIZE` | `1000` | 1 to 1,000 |
| `COMPACT_UUID_MAX_ROWS_PER_CYCLE` | `10000` | 1,000 to 1,000,000 |
| `COMPACT_UUID_MAX_CYCLE_DURATION` | `30s` | 1 second to 30 minutes |
| `COMPACT_UUID_ACTIVE_INTERVAL` | `5s` | 1 second to 5 minutes |
| `COMPACT_UUID_IDLE_INTERVAL` | `5m` | 5 seconds to 1 hour |
| `COMPACT_UUID_RETRY_INTERVAL` | `30s` | 1 second to 30 minutes |
| `COMPACT_UUID_LOCK_TIMEOUT` | `5s` | 1 second to 5 seconds |
| `COMPACT_UUID_DDL_TIMEOUT` | `30m` | 1 minute to 24 hours |
| `COMPACT_UUID_VALIDATION_TIMEOUT` | `2h` | 1 minute to 24 hours |

Invalid configuration fails startup before worker creation. Emergency pause
does not alter schema or markers and leaves legacy service functional, but a
paused deployment cannot satisfy automatic-completion acceptance.

## 9. Development Change Checklist

### 9.1 Registry, codec, and database objects

- [ ] AUTO-001: Add `model/compact_uuid_registry.go`, derived from the existing
  owned/FK registries, with source, shadow, role, model, nullability, trigger,
  index, legacy-manifest, and expected semantic-hash metadata.
- [ ] AUTO-002: Add `model/compact_uuid.go` with source normalization, RFC-order
  codec, driver/pgx scan/bind behavior, and golden-vector tests.
- [ ] AUTO-003: Add private read-only `gorm:"->;-:migration"` compact projection
  types for exact lookups; keep all shared/public model structs legacy-only.
- [ ] AUTO-004: Add per-dialect additive column and exact trigger generation in
  `model/compact_uuid_schema.go` and `model/compact_uuid_trigger.go` using only
  registry identifiers.
- [ ] AUTO-005: Extend `model/uuid_index_ddl.go` verification/timeout/online-DDL
  policy for additive compact indexes while preserving every legacy index.

### 9.2 Automatic coordinator and runtime safety

- [ ] AUTO-006: Add bounded conditional fill/repair in
  `model/compact_uuid_backfill.go` and global validation/fingerprints in
  `model/compact_uuid_validation.go`, including durable cursors, pass epochs,
  and collision classification.
- [ ] AUTO-007: Add automatic marker state and partial recovery in
  `model/compact_uuid_markers.go` using existing `DataMigration` helpers.
- [ ] AUTO-008: Add election, state inference, scheduling, retry, and continuous
  audit in `model/compact_uuid_migration.go`; correctness must not depend on the
  election lock.
- [ ] AUTO-009: Integrate compact worker start/stop with
  `model/database_bootstrap.go` after explicit topology construction. Preserve
  default v3 automatic finalization and give v3, compact, and health loops
  independent lifecycle handles; keep all history/DDL outside readiness.
- [ ] AUTO-010: Add verified compact exact lookup plus authoritative fallback in
  `model/uuid_lookup.go`; keep public values, free-text search, reports, and
  caches on legacy text.
- [ ] AUTO-011: Add strict bounded settings in
  `common/config/compact_uuid.go`; remove manual mode/finalizer concepts.
- [ ] AUTO-012: Extend `common/metrics/interface.go`, Prometheus/OTEL recorders,
  and controller exposure with bounded state, backlog, repair, fallback,
  mismatch, retry, DDL, and duration metrics.

### 9.3 Compatibility and qualification

- [ ] AUTO-013: Pin and archive the minimum supported and immediately preceding
  pre-migration binaries with checksums and SQLite capability metadata.
- [ ] AUTO-014: Add real mixed-version process harnesses for startup,
  `AutoMigrate`, CRUD, lookup, search, report, cache, upsert, bulk, dump/restore,
  replication, and rollback testing against both pinned artifacts.
- [ ] AUTO-015: Add `.github/workflows/compact-uuid-qualification.yml` with
  mandatory SQLite, MySQL 8.4, and PostgreSQL 17 suites, unified/split services,
  no-skip guards, fault injection, plans, size evidence, and 90-day artifacts.
- [ ] AUTO-016: Add a forbidden-DDL assertion proving no production path can
  drop/rename/retype legacy UUID columns or indexes while compatibility is
  active.

New functions and interfaces require project-style comments, context
propagation, wrapped errors, and Go files at or below 600 lines.

## 10. Automatic Rollout and Rollback Runbook

### 10.1 Deployment

1. Record the supported legacy artifacts and baseline schema/index metadata.
2. Deploy the new binary with default compact settings; do not run a command.
3. Confirm readiness remains healthy and migration state appears in metrics.
4. Allow the worker to reach `ready`; investigate only a persistent blocked
   state or repeated transient failure beyond the deployment alert threshold.
5. Verify applicable markers, current healthy audit, old-binary smoke tests,
   query plans, and required evidence.

DDL lock/statement timeouts, database outages, process exits, and election loss
are automatically retried. They do not fail readiness or require a phase
restart. Permanent invalid data and missing privileges require correction, but
the worker resumes on its own afterward.

### 10.2 Rollback

Rollback is a normal binary deployment at any migration percentage:

1. Stop the new binary safely.
2. Start a pinned supported pre-migration artifact without changing schema,
   indexes, triggers, markers, or configuration.
3. Run CRUD, exact lookup, search, reports, and cache smoke tests.
4. Redeploy the new binary when desired; automatic work resumes from database
   state and reconverges.

Rollback never deletes compact objects. Old `AutoMigrate` must leave them
unchanged. Database triggers continue deriving compact values while only the old
binary runs, so a later upgrade does not inherit a new backlog.

### 10.3 Prohibited cleanup

No command, environment value, hidden phase, or automatic branch may drop or
alter legacy UUID columns/indexes. An attempted cleanup must fail before DDL.
Compatibility retirement requires a separately approved proposal, new migration
generation, new test matrix, and explicit user authorization.

## 11. Security, Logging, and Performance Controls

- Build SQL identifiers only from the compile-time registry.
- Never use request input to construct DDL, SQL identifiers, batches, or memory
  allocation.
- Use per-handle dialect detection; process-global database flags are forbidden
  in compact migration and lookup code.
- Keep target materialization at or below 1,000 rows and binds at or below 900.
- Use UTC for markers, audit state, and evidence.
- Use context-aware structured Zap logging; process each error exactly once.
- Log only bounded example row IDs and aggregate counts, never UUID values,
  DSNs, credentials, token names, row content, or fingerprints.
- Use only fixed registry/state/result metric labels; IDs and UUIDs are not
  labels.
- Cap lock acquisition at five seconds. DDL has its separate timeout, but the
  qualification workload must prove no foreground operation blocks for five
  seconds and p95 remains within its threshold.
- Sample heap at least every 100 milliseconds during scale qualification.

The following observability schema is normative; qualification uses a
five-second scrape interval:

| Metric | Type and fixed labels | Meaning |
| --- | --- | --- |
| `oneapi_compact_uuid_state` | Gauge; `role`, `state` | Exactly one current state per role/process |
| `oneapi_compact_uuid_backlog_rows` | Gauge; `role`, registry `target`, `kind` | Last bounded gap/mismatch/blocker observation, not a claimed global total |
| `oneapi_compact_uuid_actions_total` | Counter; `role`, `action`, `result` | DDL, fill, validation, marker, audit, and repair outcomes |
| `oneapi_compact_uuid_lookup_fallback_total` | Counter; `role`, `reason` | Missing, mismatch, expired-health, or capability fallback |
| `oneapi_compact_uuid_last_progress_unixtime` | Gauge; `role` | UTC timestamp of last durable progress |
| `oneapi_compact_uuid_duration_seconds` | Histogram; `role`, `operation` | Lock, DDL, fill, validation, and audit duration |

Allowed role, state, target, kind, action, result, reason, and operation values
are compile-time constants. State precedence is `blocked_validation`,
`degraded`, `retry_wait`, active phase, `ready`, then `passive_legacy`; lower
states cannot mask a higher-severity condition.

## 12. Required Test Matrix

Required live cases may not skip. Run SQLite unified, MySQL 8.4 unified/split,
and PostgreSQL 17 unified/split. Mixed-dialect and SQLite split configurations
must enter compact `blocked_validation` while legacy readiness remains healthy.
AUTO-T04 through AUTO-T10 run independently against both pinned artifacts on
every applicable dialect/topology.

Liveness deadlines start when a healthy master obtains ownership. Empty
fixtures must reach `ready` within 15 minutes, 100k-row fixtures within 60
minutes, and 1m-row fixtures within four hours. A single repairable row must
meet the Section 8.6 repair bound. These are absolute first-release limits;
matching later releases must also meet the relative regression limit below.

The compatibility workload uses the recorded fixture hash, eight concurrent
clients, at least 10 requests/second, and at least 1,000 successful operations
per held migration state: 30% creates/updates covering every writer target, 40%
exact reads covering every owned type, and 10% each search/report/cache paths.
Status, payload, row count, ordering, and acknowledged-write reconciliation are
compared with a migration-disabled legacy baseline.

| ID | Scenario | Binary pass condition |
| --- | --- | --- |
| AUTO-T01 | No tables and complete-but-empty schemas, one new binary, default configuration | Readiness succeeds; 27 shadows, 12 complete trigger sets, 27 compact indexes, and applicable v3/compact markers appear without a command and within 15 minutes; CRUD works throughout |
| AUTO-T02 | Populated valid 100k supported legacy schema with no v3 or compact markers, one new binary | Readiness is within `max(2s, 1.10 × legacy-startup p95)`; v3 then compact markers appear automatically within 60 minutes |
| AUTO-T03 | Start 2, 4, and 8 new instances concurrently; inject ownership loss during a side effect | At most one mutating side effect starts per topology at an instant; ownership keys do not couple separate databases; zero unhandled DDL races, divergence, duplicates, or false markers |
| AUTO-T04 | Real oldest supported binary remains live during migration | At least 1,000 CRUD/search/report/cache operations pass with zero schema/API errors while migration converges |
| AUTO-T05 | Real old binary starts after expansion and during backfill | Startup and `AutoMigrate` succeed; compact and legacy objects remain unchanged; compatibility corpus passes |
| AUTO-T06 | Real old binary starts after completion | Startup plus 1,000 compatibility operations pass; markers/shadows are unobservable to its contract |
| AUTO-T07 | Old binary writes every target after completion | Valid writes derive equal compact values atomically; commit exposes both and rollback exposes neither; new readers immediately return the correct current row |
| AUTO-T08 | New binary writes, only old binary reads | Legacy text is complete/canonical at commit; old readers return exact objects and relationships |
| AUTO-T09 | Hold barriers for at least 60 seconds and one audit interval in pre-expansion, expansion, partial backfill, indexing, validation, and marked states under the compatibility workload | Zero lost/duplicate IDs, stale resolutions, unexpected errors, deadlocks, or fingerprint mismatches; no foreground block reaches five seconds |
| AUTO-T10 | Old → new → old → new at 0%, partial, indexed, validated, and marked states | Every binary starts; full corpus passes; automatic work resumes and reconverges without a command |
| AUTO-T11 | Under continuous old/new traffic, kill owner immediately before/after every column, trigger, index, batch, validation, and marker side effect | Restart resumes automatically; every acknowledged operation reconciles exactly once; committed bytes and marker timestamps are stable; final state exact |
| AUTO-T12 | After healthy bootstrap, inject DB outage, lock/DDL timeout, election loss, and cancellation | Behavior matches a migration-disabled outage baseline; no migration-specific error, crash, corruption, or false marker; bounded retry and automatic recovery follow fault removal |
| AUTO-T13 | Valid legacy-only row or missing trigger after markers, all processes initially healthy | Every process returns correct text-backed results; signal is consumed within one second, repair meets Section 8.6, each process atomically observes role health expiry/recovery, and marker time is stable |
| AUTO-T14 | Inject repairable mismatch, wrong physical compact value, and an unrepairable unique-value permutation | No stale result on any process; repairable state is fixed; permutation enters `blocked_validation`; text and marker timestamps never change |
| AUTO-T15 | Malformed/missing/case-normalized duplicate legacy data and dialect-feasible wrong compact type/length | Readiness/legacy traffic continue; state blocks; no invalid text mutation or missing marker write |
| AUTO-T16 | Repair every T15 source blocker while service runs | Worker leaves blocked state on the next audit and reaches `ready` within the fixture deadline without restart, flag, or CLI |
| AUTO-T17 | Split partial markers, unavailable LOG_DB, stale primary logs, partial compatible schema, mixed dialect, and SQLite split | Only authoritative/supported targets mutate; partial schema auto-completes; unsupported compact topology blocks while legacy remains unchanged; missing marker recovery revalidates globally |
| AUTO-T18 | Exact lookup for every owned type plus NULL-backlog/repair probes for every FK target | Correct row/not-found only; compact candidate is text-verified; owned lookup and all 27 operational probe plans use their intended compact indexes when healthy |
| AUTO-T19 | API, free-text search, reports, relay metadata, old/new caches | Byte-for-byte legacy golden compatibility; UUIDs remain strings; no raw bytes/base64/cast errors |
| AUTO-T20 | V3 worker/finalizer on legacy, partial, ready, and post-old-write states | V3 behavior/markers remain correct; neither marker generation hides the other's drift |
| AUTO-T21 | New and both pinned old `AutoMigrate` runs in every schema state with captured SQL/catalog hashes | No drop, rename, retype, nullability/collation change, or rewrite; shadows, triggers, checks, index metadata, manifests, and markers are byte/semantically unchanged |
| AUTO-T22 | Trigger matrix for every source/target and both pinned writers | Insert/update/save/upsert/replace/bulk plus recursive SQLite ON/OFF preserve derivation; inside writer transaction text/compact agree, another connection sees both only after commit, and rollback leaves neither change; malformed direct compact SQL is explicitly outside support |
| AUTO-T23 | Lower/upper/mixed-case and every invalid grammar vector through old writers and dialect triggers | Accepted values derive identical RFC bytes; invalid legacy writes retain exact legacy behavior, derive NULL, degrade health, and never abort solely because of synchronization |
| AUTO-T24 | Observability for success/retry/block/degrade/repair/complete | Metrics change within one scrape; bounded labels/fields; no sensitive value leakage |
| AUTO-T25 | 100k and 1m fixtures, three runs per dialect | Absolute deadlines pass; ≤1,000 rows/query, ≤900 binds, 10× rows causes ≤2× heap, median time ≤125% matching baseline, foreground p95 regression ≤10% |
| AUTO-T26 | All 27 text/compact index pairs on identical 1m fixture; 1,000 warm-ups and 10,000 sampled probes per pair, three same-run comparisons | Each compact index and their aggregate bytes are ≤70% of its exact text comparator; median/p95 regression ≤10%; no total-storage claim |
| AUTO-T27 | SQLite archived binary capability | Every supported binary reports SQLite ≥3.41 and passes persistent-trigger `unhex` golden probe |
| AUTO-T28 | PostgreSQL/MySQL trigger metadata, privileges, three restore modes, PostgreSQL physical streaming, and MySQL row replication | Exact canonical body/timing/event/security/definer policy; triggers/markers survive approved restore; replica values converge and replica compact reads await local audit |
| AUTO-T29 | Configuration defaults/bounds/malformed values/emergency pause | Defaults migrate automatically; invalid fails before worker; pause mutates nothing and preserves service |
| AUTO-T30 | Attempt destructive cleanup | Operation is unavailable or fails before DDL; all legacy objects and markers remain unchanged |
| AUTO-T31 | Repository verification | Diff, format, vet, race, Modern build, and mandatory live workflow all pass |
| AUTO-T32 | Kill active master, promote an eligible instance, then run with all instances non-master | Promoted master resumes within one lock timeout plus active interval and meets the fixture deadline; all-non-master state is `passive_legacy`, mutation is zero, and legacy service remains healthy |
| AUTO-T33 | Raw source-state and equality fingerprints under traffic | A post-traffic quiescent snapshot reconciles every acknowledged operation; NULL and empty source states remain distinct while their derived NULL semantics compare equal |

Scale baselines are keyed by commit, engine/version, schema, fixture hash, and
hardware profile. The first accepted release records the baseline only after
meeting the absolute deadlines; later matching releases must meet both the
absolute deadlines and no more than 25% median migration-time regression.

## 13. Acceptance Criteria and Traceability

- [ ] AUTO-A01: Default ordinary startup reaches validated completion on every
  supported source topology without any compact migration command, mode, or
  finalizer action, including master failover within the absolute deadline.
  Evidence: AUTO-T01 through T03 and T32.
- [ ] AUTO-A02: Real pinned old releases start and pass the compatibility corpus
  before, during, and after completion. Evidence: AUTO-T04 through T10.
- [ ] AUTO-A03: All 27 targets preserve exact authoritative text/null semantics
  and database-derived compact equality across old/new writers. Evidence:
  AUTO-T07, T08, T19, T22, T23, and T33.
- [ ] AUTO-A04: Markers never authorize stale results or suppress detection and
  repair of later gaps, mismatches, or missing triggers. Evidence: AUTO-T13,
  T14, T17, T18, and T20.
- [ ] AUTO-A05: Every durable-side-effect fault resumes idempotently with stable
  committed data, legacy availability, and no false marker. Evidence: AUTO-T11
  T12, and T32.
- [ ] AUTO-A06: Invalid data is fail-safe and observable; correction causes
  automatic recovery without restart or command. Evidence: AUTO-T15, T16, and
  T24.
- [ ] AUTO-A07: Legacy schema definitions/indexes and every public API, search,
  report, relay, and cache contract are unchanged. Evidence: AUTO-T19 and T21.
- [ ] AUTO-A08: Synchronization is atomic for valid writes and portable across
  required engines, archived binaries, replication, and restore. Evidence:
  AUTO-T07, T22, T23, T27, and T28.
- [ ] AUTO-A09: Startup, traffic, memory, batch, bind, lock, index-size, and
  lookup-latency thresholds and absolute liveness deadlines pass. Evidence:
  AUTO-T01, T02, T09, T25, T26, and T32.
- [ ] AUTO-A10: Configuration and observability fail safely and contain no
  sensitive values. Evidence: AUTO-T24 and T29.
- [ ] AUTO-A11: Required dialect/topology suites run without skips and retain
  schemas, plans, metrics, fingerprints, fault results, and old-binary checksums
  for at least 90 days. Evidence: AUTO-T01 through T33 and workflow artifacts.
- [ ] AUTO-A12: Destructive cleanup is impossible while compatibility is active.
  Evidence: AUTO-T30 and forbidden-DDL static/runtime audit.

## 14. Required Verification Evidence

The implementation issue must contain or link:

1. Intended diff, changed-file inventory, and registry completeness output.
2. Supported old-binary artifacts, checksums, SQLite probes, and schema baseline.
3. Pre/post legacy and compact schema, trigger, index, privilege, and marker
   metadata for every supported topology.
4. Per-target/pass counts, raw-source/equality fingerprints, acknowledged-write
   reconciliation, and exact start/ready/recovery UTC timestamps.
5. Automatic lifecycle, restart, master ownership transitions, retry/backoff,
   before/after fault-hook IDs, blocked-data, drift-repair, and partial-marker
   results.
6. Old/new compatibility corpus, API/cache goldens, dump/restore, and replica
   results.
7. Per-phase operation counts/rates, query plans, per-index/aggregate sizes,
   migration heap/time, foreground latency, lock waits, query counts, and bind
   counts.
8. Logs/metrics audit proving bounded labels and absence of sensitive values.

Run at minimum:

```bash
git diff --check
gofmt -l <changed-go-files>
go vet ./...
go test -race ./...
make build-frontend-modern
```

`gofmt -l` must produce no output. The qualification workflow must fail when a
required DSN, old-binary artifact, SQLite probe, or live suite is absent or
skipped.

## 15. Definition of Done

The automatic compact UUID migration is complete only when:

1. AUTO-001 through AUTO-016 are implemented with focused tests.
2. AUTO-T01 through AUTO-T33 pass on every required environment without skips.
3. AUTO-A01 through AUTO-A12 have linked evidence.
4. A default deployment reaches `ready` from empty and populated supported
   legacy schemas within the absolute deadline and without a compact-specific
   operator action.
5. Real supported old binaries pass the complete corpus before, during, and
   after markers and can be deployed as rollback without schema changes.
6. Continuous audit proves marker presence never disables drift detection,
   authoritative fallback, safe automatic repair, or explicit blocking of an
   unsafe derived-data collision.
7. Compact indexes meet the size/latency targets, with no claim of lower total
   storage while legacy compatibility remains active.
8. No production code path can destructively clean up legacy UUID storage.

Future physical storage reclamation is explicitly outside this definition and
requires retirement of the backward-compatibility contract in a new proposal.
