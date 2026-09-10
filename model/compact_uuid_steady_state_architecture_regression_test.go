package model

// This file keeps the behavior regressions found while reviewing the bounded compact UUID
// steady state. Each test deliberately corrupts only derived data after completion, leaves the
// trigger catalog healthy, and verifies that the worker cannot claim ready until it has dealt
// with the corruption. These are architecture tests: their purpose is to protect recovery
// guarantees across process boundaries and restarts, not to prescribe one repair algorithm.

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestCompactSteadyStateRepairsMismatchReportedByAnotherProcess proves that a master does not
// rely exclusively on its own in-memory lookup queue. Parameters: t is the Go test handle.
// Return values: none.
func TestCompactSteadyStateRepairsMismatchReportedByAnotherProcess(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 2)
	master := newCompactCoordinator(topology)
	driveCompactToReady(t, master)
	ctx := compactTestContext(t)

	// Create a mismatch which an independently running reader observes. The reader returns the
	// authoritative row correctly, but its process-local request queue disappears when it exits.
	dropCompactSyncTriggers(t, db, "users")
	required := compactUUIDTextFor(1)
	wrong, err := parseCompactUUID(compactUUIDTextFor(999))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 1",
		compactBindValue(dialectName(db), wrong)).Error)
	require.NoError(t, installCompactTriggers(ctx, db, compactTableForTest(t, topology, "users")))

	drainCompactRowRepairs()
	enableCompactReadsForTest(t, uuidRolePrimary)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)
	resolved, err := resolveIDByUUID(ctx, db, target, required)
	require.NoError(t, err)
	require.Equal(t, int64(1), resolved, "the remote reader must still receive the authoritative row")
	require.Len(t, drainCompactRowRepairs(), 1,
		"the fixture must prove that repair evidence existed only in the remote process")

	// The master starts with no local queue entry, exactly as it would after the remote process
	// exits. It must still leave ready and recover the stale shadows through bounded worker work.
	detected := runCompactCycleForTest(t, master)
	require.NotEqual(t, compactStateReady, detected.state,
		"a remote-process mismatch must not be forgotten by the master")
	require.LessOrEqual(t, detected.examined, compactRowBudget(),
		"initial mismatch detection must respect the worker row budget")

	driveCompactToReady(t, master)
	require.Equal(t, compactUUIDHexForTest(t, compactUUIDTextFor(1)),
		readCompactShadowHex(t, db, "users", "uuid_compact", 1),
		"the master must eventually restore the row after remote evidence was lost")
}

// TestCompactRestartAfterDriftRepairRequiresFreshAudit proves that recovery evidence survives a
// process restart. Parameters: t is the Go test handle. Return values: none.
func TestCompactRestartAfterDriftRepairRequiresFreshAudit(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 3)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	ctx := compactTestContext(t)

	// The first worker detects catalog drift. The trigger and row are then repaired, but no full
	// validation pass has run. This models a crash between recovery mutation and its validation.
	dropCompactSyncTriggers(t, db, "users")
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)
	detected := runCompactCycleForTest(t, coordinator)
	require.NotEqual(t, compactStateReady, detected.state)
	require.NoError(t, installCompactTriggers(ctx, db, compactTableForTest(t, topology, "users")))
	require.NoError(t, db.Exec("UPDATE users SET uuid = uuid WHERE id = 1").Error)
	require.Equal(t, compactUUIDHexForTest(t, compactUUIDTextFor(1)),
		readCompactShadowHex(t, db, "users", "uuid_compact", 1), "the fixture must repair the row")

	// A new coordinator represents the restarted master. Markers and object metadata alone are
	// insufficient: it must perform a fresh complete validation before publishing ready.
	restarted := newCompactCoordinator(topology)
	capture := installCompactStatementCapture(t, db)
	result := runCompactCycleForTest(t, restarted)
	require.NotEqual(t, compactStateReady, result.state,
		"a restart after drift recovery must not publish ready before a full audit")
	require.Positive(t, countCompactTraversals(capture.drain()),
		"a restart after drift recovery must begin a fresh full validation traversal")

	driveCompactToReady(t, restarted)
}

// TestCompactDurableAuditStateDisablesReadsUntilTwoCleanPasses proves that every process observes
// crash-safe drift state and that only the full completion gate clears it. Parameters: t is the Go
// test handle. Return values: none.
func TestCompactDurableAuditStateDisablesReadsUntilTwoCleanPasses(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 3)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	ctx := compactTestContext(t)

	require.NoError(t, coordinator.persistFullAuditRequired(ctx))
	runCompactHealthAudit(ctx, topology)
	enabled, reason := compactReadsEnabled(uuidRolePrimary)
	require.False(t, enabled, "durable audit-required state must disable compact reads: %s", reason)

	restarted := newCompactCoordinator(topology)
	first := runCompactCycleForTest(t, restarted)
	require.Equal(t, compactStateValidating, first.state,
		"one clean pass cannot clear durable drift evidence")
	required, err := compactFullAuditRequired(ctx, topology)
	require.NoError(t, err)
	require.True(t, required)

	second := runCompactCycleForTest(t, restarted)
	require.Equal(t, compactStateReady, second.state)
	required, err = compactFullAuditRequired(ctx, topology)
	require.NoError(t, err)
	require.False(t, required, "two clean passes must clear durable drift evidence")
}

// TestCompactSteadyStateDetectsForeignKeyOnlyDrift proves that a populated foreign-key shadow
// cannot remain stale merely because nullable foreign keys make a NULL-only probe impractical.
// Parameters: t is the Go test handle. Return values: none.
func TestCompactSteadyStateDetectsForeignKeyOnlyDrift(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 2)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	ctx := compactTestContext(t)

	// Only the derived foreign-key column is damaged. Every owned shadow remains valid and the
	// trigger catalog is restored, so object checks and owned NULL probes alone cannot see this.
	dropCompactSyncTriggers(t, db, "users")
	require.NoError(t, db.Exec("UPDATE users SET inviter_uuid = ?, inviter_uuid_compact = NULL WHERE id = 2",
		compactUUIDTextFor(1)).Error)
	require.NoError(t, installCompactTriggers(ctx, db, compactTableForTest(t, topology, "users")))

	detected := runCompactCycleForTest(t, coordinator)
	require.NotEqual(t, compactStateReady, detected.state,
		"foreign-key-only drift must take a completed worker out of ready")
	require.LessOrEqual(t, detected.examined, compactRowBudget(),
		"foreign-key drift detection must be bounded by the worker row budget")

	driveCompactToReady(t, coordinator)
	require.Equal(t, compactUUIDHexForTest(t, compactUUIDTextFor(1)),
		readCompactShadowHex(t, db, "users", "inviter_uuid_compact", 2),
		"the foreign-key compact shadow must eventually be restored")
}

// TestCompactSteadyStateDetectsUntouchedWrongNonNullShadow proves that the worker eventually
// audits an owned shadow even when no lookup happens to touch the corrupted identifier.
// Parameters: t is the Go test handle. Return values: none.
func TestCompactSteadyStateDetectsUntouchedWrongNonNullShadow(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 2)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	ctx := compactTestContext(t)

	// Use a distinct, otherwise-unused compact UUID so uniqueness remains valid and no request
	// lookup has evidence to enqueue. The restored catalog therefore appears healthy.
	dropCompactSyncTriggers(t, db, "users")
	wrong, err := parseCompactUUID(compactUUIDTextFor(999))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 1",
		compactBindValue(dialectName(db), wrong)).Error)
	require.NoError(t, installCompactTriggers(ctx, db, compactTableForTest(t, topology, "users")))

	detected := runCompactCycleForTest(t, coordinator)
	require.NotEqual(t, compactStateReady, detected.state,
		"untouched wrong non-NULL derived data must not remain hidden forever")
	require.LessOrEqual(t, detected.examined, compactRowBudget(),
		"wrong-shadow detection must be bounded by the worker row budget")

	driveCompactToReady(t, coordinator)
	require.Equal(t, compactUUIDHexForTest(t, compactUUIDTextFor(1)),
		readCompactShadowHex(t, db, "users", "uuid_compact", 1),
		"the wrong non-NULL compact shadow must eventually be restored")
}

// TestCompactSteadySweepRotatesUnderConstrainedBudget proves that a target sorting behind another
// populated target cannot be starved by the shared row budget. Parameters: t is the Go test
// handle. Return values: none.
func TestCompactSteadySweepRotatesUnderConstrainedBudget(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 3)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	ctx := compactTestContext(t)
	dropCompactSyncTriggers(t, db, "users")
	wrong, err := parseCompactUUID(compactUUIDTextFor(999))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 1",
		compactBindValue(dialectName(db), wrong)).Error)
	require.NoError(t, installCompactTriggers(ctx, db, compactTableForTest(t, topology, "users")))

	originalBudget := config.CompactUUIDMaxRowsPerCycle
	config.CompactUUIDMaxRowsPerCycle = 1
	t.Cleanup(func() { config.CompactUUIDMaxRowsPerCycle = originalBudget })
	detected := false
	for cycle := 0; cycle <= len(compactTargetsForTopology(topology)); cycle++ {
		result := runCompactCycleForTest(t, coordinator)
		require.LessOrEqual(t, result.examined, 1, "steady sweep exceeded the constrained row budget")
		if result.state != compactStateReady {
			detected = true
			break
		}
	}
	require.True(t, detected, "rotating target priority must eventually reach the late owned target")
}

// compactUUIDHexForTest returns the uppercase RFC-order hexadecimal shadow for valid UUID text.
// Parameters: t is the Go test handle and text is canonical UUIDv7 text. Return values: hex text.
func compactUUIDHexForTest(t *testing.T, text string) string {
	t.Helper()
	value, err := parseCompactUUID(text)
	require.NoError(t, err)
	return upperOf(hex.EncodeToString(value.bytes()))
}
