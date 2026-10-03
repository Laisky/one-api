# Autopsy: A Better Index That Broke the Rollback Tests Nobody Ran

- Status: Fixed in the working tree, not yet committed
- Date: 2026-09-10
- Area: schema evolution / rollback compatibility / test contracts
- Audience: backend engineers
- Introduced by: `32df60cf` ("feat(observability): harden pipelines and retention"), 2026-09-09
- Related files: `model/async_task.go`, `model/compact_uuid_rollback_contract_test.go`,
  `model/compact_uuid_oldbinary_test.go`, `model/compact_uuid_mixedversion_test.go`,
  `model/compact_uuid_prevbinary_helpers_test.go`
- Related docs: [Compact UUID acceptance status](../manuals/compact_uuid_acceptance_status.md),
  [The Completed Compact UUID Migration That Kept Re-Proving Itself](./20260910_compact-uuid-steady-state-cost.md)

## 1. Summary

Three live compatibility tests had been failing on `main` since 2026-09-09, and nobody knew:

- `TestCompactUUIDOldBinary`
- `TestCompactUUIDOldBinaryDrift`
- `TestCompactUUIDMixedVersionDrift`, all six stages

They run real, pinned rollback builds of one-api against a schema the current build created, and
check that the older build's startup does not disturb it. Both pinned builds re-created one index:
`idx_async_task_bindings_last_accessed_at`.

The cause was a deliberate improvement. `32df60cf` replaced that single-column index with the
composite `idx_async_task_retention (last_accessed_at, created_at)`, which the retention sweep
plans against and which serves every lookup the old one did. It intentionally left the old index
in place on existing deployments. But older builds still declare the old index, and GORM's
AutoMigrate creates every declared index that is missing. A rollback onto a freshly created schema
therefore puts it back.

Nothing was broken in production. The tests were right that the catalog changed, but wrong to call
it a compatibility failure. The fix states the rollback contract exactly — nothing may be dropped
or rewritten, and a rollback build may re-add only indexes the current build deliberately
superseded, listed by name and exact definition — and makes that list self-checking so it cannot
become a loophole.

## 2. Impact

- **Production:** none. `b1`, like every deployment upgraded past `32df60cf`, still has the old
  index next to the new one, so a rollback there is a no-op. Only a fresh install that is then
  rolled back gains the old index, which is the state upgraded deployments are already in.
- **Signal:** the only suites that exercise real rollback builds were red for a day and a half.
  Had a genuine rollback break landed in that window, it would have been indistinguishable from
  this one.

## 3. How to reproduce

Against live PostgreSQL 17, with the pinned builds available:

    COMPACT_UUID_TEST_POSTGRES_DSN=... ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1 \
      go test ./model -run 'TestCompactUUIDOldBinary$|TestCompactUUIDOldBinaryDrift|TestCompactUUIDMixedVersionDrift' -v

On the pre-fix tree all three fail. With the contract assertion in place and an empty allowlist,
every one of the eight failing checks — six mixed-version stages and two rollback builds —
reports the same single line and nothing else:

    added IDX|idx_async_task_bindings_last_accessed_at|da51571d360ead609e777dc3d608a148

That hash is the md5 of exactly the definition both pinned builds declare:

    CREATE INDEX idx_async_task_bindings_last_accessed_at ON public.async_task_bindings USING btree (last_accessed_at)

## 4. Root cause

### 4.1 AutoMigrate only ever adds

Both pinned builds declare `LastAccessedAt int64 gorm:"index"`. HEAD declares
`gorm:"index:idx_async_task_retention,priority:1"`. GORM creates missing declared indexes and never
drops undeclared ones. So "stop declaring an index" is invisible to deployments that have it, and
reversible by any older build on deployments that do not. `32df60cf` documented the first half in
its own comment; the second half is what the rollback suites exist to catch.

### 4.2 The byte-identity assertion was an over-approximation

`TestCompactUUIDOldBinary` and `TestCompactUUIDMixedVersionDrift` compared a catalog fingerprint for
byte-identity. The fingerprint deliberately includes every index in the schema, not only compact and
legacy UUID objects, because that was the simplest way to be sure nothing changed. It held only while
the current build declared a superset of every rollback build's indexes. The first time an index was
replaced rather than added, byte-identity became unachievable without keeping the redundant index
forever.

The preceding-build suite had already met this once. Build `ed15a144` declares ordinary owned-UUID
indexes that v3 replaced, and the project decided then that "an old binary re-adding an index it owns
is additive… and takes nothing away". That test allowed those indexes by name. The same reasoning had
simply never been written down as a general rule.

### 4.3 Why nobody saw it

The pinned-build suites need real database engines, so they run only in the pull-request workflow.
`32df60cf` was pushed directly to `main`, so no pull-request run ever saw it. The failures surfaced
during an unrelated verification run on 2026-09-10.

## 5. Fix

### 5.1 Decision

Two fixes were possible:

- **Keep the old index declared.** Rollbacks become no-ops again, and every fresh deployment
  maintains a redundant index on `async_task_bindings` forever. `32df60cf` rejected exactly that
  write cost on purpose.
- **State the contract exactly.** Allow a rollback build to re-add an index the current build
  superseded, and nothing else.

The second was chosen. It matches the precedent in section 4.2, leaves fresh installs lean, and ends
every rollback in a state the project already accepts on every upgraded deployment.

### 5.2 One contract, one assertion

`model/compact_uuid_rollback_contract_test.go` defines the contract once:

- every catalog line captured before a rollback build runs must be present, unchanged, afterwards —
  nothing dropped, renamed, retyped, or rewritten;
- an addition is allowed only if it is an index the rollback build itself is known to re-add (the
  preceding build's owned-UUID indexes, as before) or an entry of `compactRollbackSupersededIndexes`,
  matched by name **and** exact definition hash.

`requireRollbackCatalogContract` enforces it in all three suites and reports every violation by line,
where the old byte-identity assertion printed two fingerprints of hundreds of lines each.

Compact and legacy UUID objects remain byte-identical: none of them can appear in the list.

### 5.3 A list that cannot go stale

An allowlist is only safe while every entry stays true. `TestCompactRollbackSupersededIndexesAreReallySuperseded`
runs on every ordinary `go test`, with no database server, and proves for each entry that:

- the current build does not declare the index — otherwise the entry is unnecessary and must go;
- its replacement exists — otherwise the old index is not superseded at all; and
- the replacement leads with the same column, so every lookup the old index served is still served.

It fails the moment someone re-declares the old index or drops the replacement.

## 6. Verification

| Check | Result |
| --- | --- |
| Reproduction, empty allowlist, live PostgreSQL 17 | 8 of 8 failing checks report exactly the one index, with the pinned builds' exact definition hash; nothing dropped or rewritten |
| Fix, live PostgreSQL 17 | all three suites pass; mixed-version cycling completes all six stages through both rounds and reconverges |
| Preceding build's additions after the fix | its 12 own owned-UUID indexes and the one superseded index, nothing else |
| Contract unit tests | pass: allows only listed and own indexes, rejects unlisted additions, changed definitions, rewrites, and drops |
| Staleness check | passes on the current schema |

Mutation checks, each reverted afterwards:

| Mutation | Test that turned red |
| --- | --- |
| Additions ignored | `TestCompactRollbackCatalogContract`: the unlisted, own-index, definition, and rewrite cases |
| Removals ignored | `TestCompactRollbackCatalogContract`: the drop and rewrite cases |
| Superseded index matched by name instead of exact definition | `TestCompactRollbackCatalogContract`: the definition case |
| Current build re-declares the standalone index | `TestCompactRollbackSupersededIndexesAreReallySuperseded` |
| Current build drops the composite | `TestCompactRollbackSupersededIndexesAreReallySuperseded` |

## 7. Lessons

### 7.1 Replacing an index is a schema change with a rollback half

Adding an index is forward-compatible. Replacing one is not, because every older build still
declares what was removed. Any change that stops declaring an index, column, or constraint deserves
the question "what does a rollback put back?" in its commit message.

### 7.2 Write the rule, not the exception

The preceding-build suite already tolerated superseded indexes, by name, for one family. The
general rule stayed implicit, so the next instance looked like a new failure. A named contract with
one assertion makes the next replacement a one-line, reviewed list entry.

### 7.3 An allowlist needs a test that it is still true

Every entry in an allowlist is a place the real assertion is switched off. The staleness test is what
keeps that honest: the entry exists only while the facts that justify it hold.

### 7.4 Tests that only run in one place are only as good as that place

The rollback suites need live engines, so they run only on pull requests, and direct pushes to
`main` skip them. That is a repository setting, not something a test can fix: branch protection
that requires the pull-request workflow, or a scheduled run of the live suites on `main`, would have
caught this the same day.
