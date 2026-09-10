package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// resetCompactRepairQueueForTest starts and ends a test with no process-local repair evidence.
// Parameters:
//   - t: test that owns the queue reset.
//
// Return values: none.
func resetCompactRepairQueueForTest(t *testing.T) {
	t.Helper()
	drainCompactRowRepairs()
	t.Cleanup(func() { drainCompactRowRepairs() })
}

// TestGetUserIDByUUIDUsesVerifiedCompactLookup makes the public ID lookup participate in the
// compact read contract after a healthy audit. The compact projection verifies the legacy UUID
// before returning an ID, so this is both the fast path and the safe path; using the historical
// `uuid = ?` model query leaves lookup-driven repair unreachable in production.
func TestGetUserIDByUUIDUsesVerifiedCompactLookup(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	driveCompactToReady(t, newCompactCoordinator(topology))
	enableCompactReadsForTest(t, uuidRolePrimary)

	capture := installCompactStatementCapture(t, db)
	id, err := GetUserIdByUUID(compactUUIDTextFor(1))
	require.NoError(t, err)
	require.Equal(t, 1, id)

	statements := capture.drain()
	for _, statement := range statements {
		if strings.Contains(statement.sql, "uuid_compact") {
			return
		}
	}
	require.Fail(t, "public UUID ID lookup did not use the verified compact path", "%v", statements)
}

// TestPublicUUIDLookupPreservesLegacyFallbackBehavior proves that an unhealthy or incomplete
// compact installation cannot narrow the identifiers served by the authoritative text index.
// Parameters: t is the Go test handle. Return values: none.
func TestPublicUUIDLookupPreservesLegacyFallbackBehavior(t *testing.T) {
	db, _ := newCompactTestTopology(t)
	resetCompactHealthForTest()
	uppercase := upperOf(compactUUIDTextFor(1))
	nonV7 := "018f0000-0000-4000-8000-000000000001"
	seedCompactUser(t, db, 1, uppercase)
	seedCompactUser(t, db, 2, nonV7)

	for _, testCase := range []struct {
		name string
		ref  string
		want int
	}{
		{name: "uppercase stored representation", ref: uppercase, want: 1},
		{name: "pre-v7 blocker row", ref: nonV7, want: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			id, err := GetUserIdByUUID(testCase.ref)
			require.NoError(t, err)
			require.Equal(t, testCase.want, id)
		})
	}
}

// TestPublicUUIDLookupPreservesStoredCaseDuringCompactFallback proves that a missing shadow cannot
// hide an uppercase legacy UUID merely because request canonicalization lowercases it. Parameters:
// t is the Go test handle. Return values: none.
func TestPublicUUIDLookupPreservesStoredCaseDuringCompactFallback(t *testing.T) {
	resetCompactRepairQueueForTest(t)
	db, topology := newCompactTestTopology(t)
	uppercase := upperOf(compactUUIDTextFor(1))
	seedCompactUser(t, db, 1, uppercase)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	dropCompactSyncTriggers(t, db, "users")
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)
	require.NoError(t, installCompactTriggers(compactTestContext(t), db,
		compactTableForTest(t, topology, "users")))
	enableCompactReadsForTest(t, uuidRolePrimary)

	id, err := GetUserIdByUUID(uppercase)
	require.NoError(t, err)
	require.Equal(t, 1, id)
}

// TestCompactSteadyStateDetectsOwnedNullAndEmptyLegacyValues ensures the no-scan owned probe
// preserves the migration invariant that an owned UUID can never be NULL or empty. Both shapes
// leave a NULL derived shadow after a trigger-bypassing write and must take a completed worker
// out of ready before it claims the installation is healthy.
func TestCompactSteadyStateDetectsOwnedNullAndEmptyLegacyValues(t *testing.T) {
	for _, legacy := range []any{nil, ""} {
		legacy := legacy
		t.Run("owned legacy value "+compactLegacyCaseName(legacy), func(t *testing.T) {
			db, topology := newCompactTestTopology(t)
			seedCompactUser(t, db, 1, compactUUIDTextFor(1))
			coordinator := newCompactCoordinator(topology)
			driveCompactToReady(t, coordinator)

			dropCompactSyncTriggers(t, db, "users")
			require.NoError(t, db.Exec("UPDATE users SET uuid = ?, uuid_compact = NULL WHERE id = 1", legacy).Error)
			require.NoError(t, installCompactTriggers(compactTestContext(t), db,
				compactTableForTest(t, topology, "users")))

			result := runCompactCycleForTest(t, coordinator)
			require.NotEqual(t, compactStateReady, result.state,
				"an owned %s UUID must not be hidden from the completed-worker health check", compactLegacyCaseName(legacy))
			require.True(t, coordinator.fullAuditRequired,
				"an owned %s UUID requires full validation before ready can be reported", compactLegacyCaseName(legacy))
		})
	}
}

// compactLegacyCaseName returns the bounded display name for one deliberately invalid owned
// UUID fixture.
// Parameters:
//   - legacy: NULL or empty legacy UUID fixture.
//
// Return values:
//   - string: bounded fixture name suitable for a subtest label.
func compactLegacyCaseName(legacy any) string {
	if legacy == nil {
		return "NULL"
	}
	return "empty"
}

// TestCompactSteadyStateLookupRepairDoesNotReportReadyForBlockersOrCollisions ensures targeted
// repair has the same state semantics as the full reconciliation path. A known bad row cannot
// be discarded merely because it was reached through lookup evidence rather than a scan.
func TestCompactSteadyStateLookupRepairDoesNotReportReadyForBlockersOrCollisions(t *testing.T) {
	t.Run("invalid authoritative value requires full validation", func(t *testing.T) {
		resetCompactRepairQueueForTest(t)
		db, topology := newCompactTestTopology(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		coordinator := newCompactCoordinator(topology)
		driveCompactToReady(t, coordinator)

		dropCompactSyncTriggers(t, db, "users")
		derived, err := parseCompactUUID(compactUUIDTextFor(9999))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid = ?, uuid_compact = ? WHERE id = 1",
			"not-an-owned-uuid", compactBindValue(dialectName(db), derived)).Error)
		require.NoError(t, installCompactTriggers(compactTestContext(t), db,
			compactTableForTest(t, topology, "users")))

		target, err := compactLookupTarget("users")
		require.NoError(t, err)
		enqueueCompactRowRepair(target, 1)
		result := runCompactCycleForTest(t, coordinator)
		require.NotEqual(t, compactStateReady, result.state,
			"clearing a stale shadow over invalid authoritative data must require a full audit")
		require.True(t, coordinator.fullAuditRequired)
	})

	t.Run("unresolvable compact uniqueness permutation blocks validation", func(t *testing.T) {
		resetCompactRepairQueueForTest(t)
		db, topology := newCompactTestTopology(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		seedCompactUser(t, db, 2, compactUUIDTextFor(2))
		coordinator := newCompactCoordinator(topology)
		driveCompactToReady(t, coordinator)

		dropCompactSyncTriggers(t, db, "users")
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id IN (1, 2)").Error)
		one, err := parseCompactUUID(compactUUIDTextFor(1))
		require.NoError(t, err)
		two, err := parseCompactUUID(compactUUIDTextFor(2))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 1",
			compactBindValue(dialectName(db), two)).Error)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
			compactBindValue(dialectName(db), one)).Error)
		require.NoError(t, installCompactTriggers(compactTestContext(t), db,
			compactTableForTest(t, topology, "users")))

		target, err := compactLookupTarget("users")
		require.NoError(t, err)
		enqueueCompactRowRepair(target, 1)
		enqueueCompactRowRepair(target, 2)
		result := runCompactCycleForTest(t, coordinator)
		require.Equal(t, compactStateBlockedValidation, result.state)
		require.Positive(t, result.blockers)
	})
}

// TestCompactRowRepairQueueDeduplicatesAndRetainsFailedWork ensures the bounded process-local
// queue is evidence, not a lossy notification channel. Repeated evidence for one row must not
// crowd out another row, and a failed repair attempt must leave the evidence available for the
// next worker cycle.
func TestCompactRowRepairQueueDeduplicatesAndRetainsFailedWork(t *testing.T) {
	resetCompactRepairQueueForTest(t)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)

	for attempt := 0; attempt < compactRowRepairQueueSize*2; attempt++ {
		enqueueCompactRowRepair(target, 1)
	}
	enqueueCompactRowRepair(target, 2)
	requests := drainCompactRowRepairs()
	require.ElementsMatch(t, []compactRowRepairRequest{{target: target, id: 1}, {target: target, id: 2}}, requests,
		"duplicate evidence must not consume the bounded queue capacity")

	enqueueCompactRowRepair(target, 1)
	coordinator := newCompactCoordinator(&databaseTopology{})
	result := compactCycleResult{}
	ownership := &compactOwnership{
		verify: func(context.Context) (bool, error) { return false, nil },
	}
	err = repairCompactRowsFromLookups(context.Background(), coordinator, ownership, &result)
	require.Error(t, err)
	require.ElementsMatch(t, []compactRowRepairRequest{{target: target, id: 1}}, drainCompactRowRepairs(),
		"failed repair attempts must retain their evidence for a later cycle")
}

// TestCompactRowRepairQueueRetainsBatchAfterReadFailure proves that draining is not an
// acknowledgement. A database failure after the drain must leave every unprocessed request for a
// later cycle. Parameters: t is the Go test handle. Return values: none.
func TestCompactRowRepairQueueRetainsBatchAfterReadFailure(t *testing.T) {
	resetCompactRepairQueueForTest(t)
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 2)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)
	enqueueCompactRowRepair(target, 1)
	enqueueCompactRowRepair(target, 2)
	require.NoError(t, db.Exec("DROP TABLE users").Error)

	result := compactCycleResult{state: compactStateReady}
	ownership := &compactOwnership{verify: func(context.Context) (bool, error) { return true, nil }}
	err = repairCompactRowsFromLookups(compactTestContext(t), coordinator, ownership, &result)
	require.Error(t, err)
	require.ElementsMatch(t,
		[]compactRowRepairRequest{{target: target, id: 1}, {target: target, id: 2}},
		drainCompactRowRepairs(), "a failed post-drain read must retain the complete pending batch")
}

// TestCompactStaleRepairHintsDoNotForceFullAudit proves that queue entries are hints rather than
// drift state. Deleted or already-correct rows must be acknowledged without degrading health.
// Parameters: t is the Go test handle. Return values: none.
func TestCompactStaleRepairHintsDoNotForceFullAudit(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		delete bool
	}{
		{name: "already correct"},
		{name: "already deleted", delete: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resetCompactRepairQueueForTest(t)
			db, topology := newCompactTestTopology(t)
			seedCompactUser(t, db, 1, compactUUIDTextFor(1))
			coordinator := newCompactCoordinator(topology)
			driveCompactToReady(t, coordinator)
			target, err := compactLookupTarget("users")
			require.NoError(t, err)
			enqueueCompactRowRepair(target, 1)
			if testCase.delete {
				require.NoError(t, db.Exec("DELETE FROM users WHERE id = 1").Error)
			}

			result := runCompactCycleForTest(t, coordinator)
			require.Equal(t, compactStateReady, result.state)
			required, err := compactFullAuditRequired(compactTestContext(t), topology)
			require.NoError(t, err)
			require.False(t, required, "stale queue hints must not force a database-wide audit")
		})
	}
}

// TestCompactTargetedRepairRechecksOwnershipAfterPersist proves that durable drift recording does
// not authorize a later mutation after ownership is lost. Parameters: t is the Go test handle.
// Return values: none.
func TestCompactTargetedRepairRechecksOwnershipAfterPersist(t *testing.T) {
	resetCompactRepairQueueForTest(t)
	db, topology := newCompactTestTopology(t)
	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	dropCompactSyncTriggers(t, db, "users")
	wrong, err := parseCompactUUID(compactUUIDTextFor(999))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 1",
		compactBindValue(dialectName(db), wrong)).Error)
	require.NoError(t, installCompactTriggers(compactTestContext(t), db,
		compactTableForTest(t, topology, "users")))
	target, err := compactLookupTarget("users")
	require.NoError(t, err)
	enqueueCompactRowRepair(target, 1)
	before := readCompactShadowHex(t, db, "users", "uuid_compact", 1)

	checks := 0
	ownership := &compactOwnership{verify: func(context.Context) (bool, error) {
		checks++
		return checks < 4, nil
	}}
	result := compactCycleResult{state: compactStateReady}
	err = repairCompactRowsFromLookups(compactTestContext(t), coordinator, ownership, &result)
	require.Error(t, err)
	require.Equal(t, before, readCompactShadowHex(t, db, "users", "uuid_compact", 1),
		"ownership loss after persistence must prevent row mutation")
}
