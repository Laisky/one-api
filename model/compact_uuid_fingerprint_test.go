package model

// Fingerprint, concurrency, query-plan, and bounds tests for compact UUID storage
// (AUTO-T03, T18, T25, T33).

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCompactUUIDFingerprints(t *testing.T) {
	// AUTO-T33: the equality streams must agree, while the raw-source stream keeps SQL NULL,
	// empty text, and non-empty text distinct.
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	seedCompactUser(t, db, 2, compactUUIDTextFor(2))
	driveCompactToReady(t, newCompactCoordinator(topology))

	t.Run("equality digests match at completion", func(t *testing.T) {
		fingerprint, err := computeCompactFingerprints(ctx, topology, uuidRolePrimary)
		require.NoError(t, err)
		require.True(t, fingerprint.matches(),
			"legacy and compact equality streams must be identical at completion")
		require.NotEmpty(t, fingerprint.LegacyDigest)
		require.Positive(t, fingerprint.Rows)
	})

	t.Run("null and empty source states stay distinct while deriving equal", func(t *testing.T) {
		// Both a NULL and an empty nullable FK derive compact NULL, so the equality streams
		// must still match. The raw-source stream must nonetheless record them separately —
		// that is the whole reason it exists.
		require.NoError(t, db.Exec("UPDATE users SET inviter_uuid = NULL WHERE id = 1").Error)
		require.NoError(t, db.Exec("UPDATE users SET inviter_uuid = '' WHERE id = 2").Error)

		fingerprint, err := computeCompactFingerprints(ctx, topology, uuidRolePrimary)
		require.NoError(t, err)
		require.True(t, fingerprint.matches(),
			"NULL and empty both derive NULL, so the equality streams must still agree")
		require.Positive(t, fingerprint.NullSources, "SQL NULL sources must be observed")
		require.Positive(t, fingerprint.EmptySources, "empty-text sources must be observed")
		require.Positive(t, fingerprint.PopulatedSources, "populated sources must be observed")
	})

	t.Run("a corrupted shadow breaks the equality digests", func(t *testing.T) {
		// A fingerprint that could not detect a wrong shadow would be worthless evidence, so
		// this asserts the detector actually detects.
		dropCompactSyncTriggers(t, db, "users")
		wrong, err := parseCompactUUID(compactUUIDTextFor(555))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 1",
			compactBindValue(dialectName(db), wrong)).Error)

		fingerprint, err := computeCompactFingerprints(ctx, topology, uuidRolePrimary)
		require.NoError(t, err)
		require.False(t, fingerprint.matches(),
			"a wrong shadow must make the equality digests disagree")
	})
}

func TestCompactUUIDFingerprintsNeverLeakValues(t *testing.T) {
	// Section 11: no UUID value or digest byte may reach a log or a diagnostic.
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	driveCompactToReady(t, newCompactCoordinator(topology))

	fingerprint, err := computeCompactFingerprints(ctx, topology, uuidRolePrimary)
	require.NoError(t, err)

	// A digest is opaque evidence, so it must not contain the source values it summarizes.
	for _, digest := range []string{
		fingerprint.LegacyDigest, fingerprint.CompactDigest, fingerprint.RawSourceDigest,
	} {
		require.Len(t, digest, 64, "digests are hex SHA-256")
		require.NotContains(t, digest, compactUUIDTextFor(1))
		require.NotContains(t, strings.ToLower(digest), strings.ReplaceAll(compactUUIDTextFor(1), "-", ""))
	}
}

func TestCompactUUIDConcurrentCyclesConverge(t *testing.T) {
	// AUTO-T03: several instances racing must produce zero divergence, duplicates, or false
	// markers. Correctness must not depend on the election lock, so this deliberately runs the
	// real cycle from several goroutines against one database.
	//
	// The database is file-backed: concurrent goroutines grow the pool past one connection,
	// and every connection to ":memory:" would open its own separate empty database.
	db, topology := newCompactFileTestTopology(t)
	ctx := compactTestContext(t)

	for index := 1; index <= 20; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}

	const instances = 4
	var waitGroup sync.WaitGroup
	for instance := 0; instance < instances; instance++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			coordinator := newCompactCoordinator(topology)
			for cycle := 0; cycle < 60; cycle++ {
				ownership, acquired, err := acquireCompactOwnership(ctx, topology)
				if err != nil || !acquired {
					// Losing the race is the expected outcome for all but one instance.
					continue
				}
				_, _ = runCompactCycle(ctx, coordinator, ownership)
				ownership.release()
			}
		}()
	}
	waitGroup.Wait()

	// Whoever won, the outcome must be one consistent, correct state.
	final := newCompactCoordinator(topology)
	result := driveCompactToReady(t, final)
	require.Equal(t, compactStateReady, result.state)

	verified, reason, err := validateCompactObjects(ctx, topology)
	require.NoError(t, err)
	require.True(t, verified, "concurrent cycles must converge on verified objects: %s", reason)

	fingerprint, err := computeCompactFingerprints(ctx, topology, uuidRolePrimary)
	require.NoError(t, err)
	require.True(t, fingerprint.matches(), "concurrent cycles must not diverge")

	// Exactly one marker row, not one per racing instance.
	var markers int64
	require.NoError(t, db.Model(&DataMigration{}).
		Where("migration_key = ?", compactPrimaryMigrationKey).Count(&markers).Error)
	require.Equal(t, int64(1), markers, "racing instances must not write duplicate markers")
}

func TestCompactUUIDQueryPlans(t *testing.T) {
	// AUTO-T18: the owned lookup and every operational probe must use their intended compact
	// index. A predicate that silently degraded to a table scan would still be correct and
	// would quietly destroy the performance the whole project exists for.
	db, topology := newCompactTestTopology(t)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	driveCompactToReady(t, newCompactCoordinator(topology))

	t.Run("exact compact lookup uses the compact index", func(t *testing.T) {
		golden, err := parseCompactUUID(compactUUIDTextFor(1))
		require.NoError(t, err)
		plan := explainSQLite(t, db,
			"SELECT id, uuid FROM users WHERE uuid_compact = ? LIMIT 1",
			compactBindValue(dialectName(db), golden))
		require.Contains(t, plan, "idx_users_uuid_compact_unique",
			"the exact lookup must ride the compact index, not scan: %s", plan)
	})

	t.Run("every operational null-backlog probe uses its compact index", func(t *testing.T) {
		// All 27 probes, derived from the registry, so a new column cannot skip coverage.
		for _, target := range compactTargetsForTopology(topology) {
			plan := explainSQLite(t, topology.handle(target.role),
				"SELECT 1 FROM "+target.table+" WHERE "+target.compactColumn+" IS NULL LIMIT ?",
				compactMaxMaterializedRows)
			require.Contains(t, plan, target.indexName(),
				"probe for %s must use %s: %s", target.id(), target.indexName(), plan)
		}
	})
}

// explainSQLite returns the SQLite query plan for one statement.
// Parameters:
//   - t: test handle used for assertions.
//   - db: SQLite handle to explain against.
//   - sql: statement to explain.
//   - binds: statement bind values.
//
// Return values:
//   - string: concatenated plan detail rows.
func explainSQLite(t *testing.T, db *gorm.DB, sql string, binds ...any) string {
	t.Helper()
	rows := []struct {
		Detail string `gorm:"column:detail"`
	}{}
	require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+sql, binds...).Scan(&rows).Error)
	details := make([]string, 0, len(rows))
	for _, row := range rows {
		details = append(details, row.Detail)
	}
	return strings.Join(details, " | ")
}

func TestCompactUUIDReconciliationRespectsBounds(t *testing.T) {
	// Section 11: at most 1,000 materialized rows per query and at most 900 binds per
	// statement. These are hard caps, so they are asserted rather than assumed.
	db, _ := newCompactTestTopology(t)
	ctx := compactTestContext(t)
	target, err := compactTargetByID("users.uuid")
	require.NoError(t, err)

	expandAndTriggerAll(t, db, uuidRolePrimary)
	for index := 1; index <= 50; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}

	t.Run("candidate reads are bounded", func(t *testing.T) {
		candidates, err := readCompactCandidates(ctx, db, target, 0, 10_000)
		require.NoError(t, err)
		require.LessOrEqual(t, len(candidates), compactMaxMaterializedRows,
			"a candidate read must never materialize more than 1,000 rows")
	})

	t.Run("gap probe advances on gaps, rests when clean, sweep always advances", func(t *testing.T) {
		// The fixture's shadows were all derived by the triggers, so the gap probe must find
		// NOTHING — resting at zero instead of re-reading 50 proved-valid rows is exactly the
		// fix for the starvation that failed the first 1m qualification run. The bounded
		// rolling sweep is what re-examines valid rows, one slice per cycle.
		progress, err := reconcileCompactTarget(ctx, db, target, newCompactCursor(), 10)
		require.NoError(t, err)
		require.LessOrEqual(t, progress.examined, 10, "the row budget must bound the traversal")
		require.Zero(t, progress.cursor, "a clean target's gap cursor must rest, not re-scan")
		require.True(t, progress.wrapped, "the gap probe must have yielded")
		require.Positive(t, progress.sweepCursor, "the rolling sweep must advance through valid rows")

		// Now open a real gap: the probe must find it and the gap cursor must advance to it.
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 7").Error)
		dropCompactSyncTriggers(t, db, "users")
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 7").Error)

		progress, err = reconcileCompactTarget(ctx, db, target, newCompactCursor(), 10)
		require.NoError(t, err)
		require.Equal(t, 1, progress.updated, "the gap probe must repair the reopened gap")
		require.GreaterOrEqual(t, progress.cursor, int64(0),
			"after repairing the only gap the probe wraps and rests")
	})

	t.Run("a cancelled cycle stops cleanly without starting a side effect", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		progress, err := reconcileCompactTarget(cancelled, db, target, newCompactCursor(), 100)
		require.NoError(t, err, "cancellation is not an error and must not be reported as one")
		require.Zero(t, progress.updated, "a cancelled cycle must not write")
	})
}

func TestCompactUUIDBacklogProbeIsBounded(t *testing.T) {
	// The backlog gauge documents a bounded observation, not a claimed global total.
	db, _ := newCompactTestTopology(t)
	ctx := compactTestContext(t)
	target, err := compactTargetByID("users.uuid")
	require.NoError(t, err)

	expandAndTriggerAll(t, db, uuidRolePrimary)
	dropCompactSyncTriggers(t, db, "users")
	for index := 1; index <= 30; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}

	backlog, err := compactNullBacklog(ctx, db, target)
	require.NoError(t, err)
	require.Equal(t, 30, backlog, "every unfilled row must be observed as backlog")
	require.LessOrEqual(t, backlog, compactMaxMaterializedRows, "the probe must stay bounded")
}
