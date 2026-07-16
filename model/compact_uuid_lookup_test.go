package model

// Runtime lookup safety tests for compact UUID storage (AUTO-T13, T14, T18).
//
// These are the tests that matter most: the compact index is derived data, so the only property
// standing between it and a wrong answer is that an unverified candidate is never returned.
// Each test here corrupts the shadow in a specific way and asserts the lookup still returns the
// correct row or a correct not-found.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/idresolve"
)

// enableCompactReadsForTest publishes a fresh healthy audit so compact predicates are used.
//
// Tests that want the compact path must state so explicitly, because a process defaults to
// legacy-safe behavior until its own audit passes — which is itself a property under test.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - role: database role to mark healthy.
//
// Return values: none.
func enableCompactReadsForTest(t *testing.T, role uuidDBRole) {
	t.Helper()
	publishCompactHealth(role, compactHealth{
		state:           compactStateReady,
		compactReadable: true,
		observedAt:      timeNowUTCForTest(),
	})
	t.Cleanup(resetCompactHealthForTest)
}

func TestCompactUUIDLookupVerifiesCandidates(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	seedCompactUser(t, db, 2, compactUUIDTextFor(2))
	driveCompactToReady(t, newCompactCoordinator(topology))
	enableCompactReadsForTest(t, uuidRolePrimary)

	t.Run("resolves through the compact index when healthy", func(t *testing.T) {
		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id)
	})

	t.Run("accepts uppercase and padded request input", func(t *testing.T) {
		// Trimming and canonicalization happen at the request boundary, so a pasted value
		// with padding or different case still resolves.
		id, err := resolveIDByUUID(ctx, db, target, "  "+upperOf(compactUUIDTextFor(2))+"  ")
		require.NoError(t, err)
		require.Equal(t, int64(2), id)
	})

	t.Run("invalid input returns the existing ErrInvalidRef", func(t *testing.T) {
		for _, ref := range []string{"", "nope", "018f0000-0000-4000-8000-000000000001", "12345"} {
			_, err := resolveIDByUUID(ctx, db, target, ref)
			require.ErrorIs(t, err, idresolve.ErrInvalidRef, "ref %q", ref)
		}
	})

	t.Run("canonical unknown uuid returns the existing ErrNotFound", func(t *testing.T) {
		_, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(9999))
		require.ErrorIs(t, err, idresolve.ErrNotFound)
	})
}

func TestCompactUUIDLookupNeverReturnsStaleResults(t *testing.T) {
	// AUTO-T14: no stale result on any process, for either failure direction.
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	seedCompactUser(t, db, 2, compactUUIDTextFor(2))
	driveCompactToReady(t, newCompactCoordinator(topology))
	enableCompactReadsForTest(t, uuidRolePrimary)

	// The corruption below is applied with the sync triggers dropped, which is not a
	// convenience: the trigger otherwise re-derives the shadow from the row's own text and
	// undoes the corruption in the same statement. That self-healing is a real property — it
	// is asserted separately below — but it would make every assertion here vacuous, because
	// the lookup would be reading an already-correct shadow. Dropping the trigger reproduces
	// exactly the state section 7 names: "if a trigger is missing or a compact shadow is
	// corrupted".
	dropCompactSyncTriggers(t, db, "users")

	t.Run("a gap does not cause a stale not-found", func(t *testing.T) {
		// A missing trigger or an old-binary write: the text is correct and committed, the
		// shadow is absent. The row must still be found.
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)

		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err, "a committed write must be visible through its text column")
		require.Equal(t, int64(1), id, "authoritative text fallback must find the row")
	})

	t.Run("a wrong shadow does not cause a stale row", func(t *testing.T) {
		// Row 1's shadow is already NULL from the previous case, so pointing row 2's shadow
		// at row 1's identifier does not collide with the unique compact index. The compact
		// index now nominates row 2 for row 1's identifier, and row 2's own authoritative
		// text must veto it.
		wrong, err := parseCompactUUID(compactUUIDTextFor(1))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
			compactBindValue(dialectName(db), wrong)).Error)

		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id, "a candidate whose text disagrees must never be returned")

		// And the identifier that legitimately belongs to row 2 still resolves to row 2,
		// through the authoritative index, even though row 2's shadow now lies.
		id, err = resolveIDByUUID(ctx, db, target, compactUUIDTextFor(2))
		require.NoError(t, err)
		require.Equal(t, int64(2), id)
	})

	t.Run("a corrupted shadow does not resurrect an unknown identifier", func(t *testing.T) {
		unknown, err := parseCompactUUID(compactUUIDTextFor(4242))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
			compactBindValue(dialectName(db), unknown)).Error)

		// The compact index nominates row 2, whose text says otherwise; the authoritative
		// index then finds nothing. A canonical unknown UUID must be not-found, never row 2.
		_, err = resolveIDByUUID(ctx, db, target, compactUUIDTextFor(4242))
		require.ErrorIs(t, err, idresolve.ErrNotFound,
			"a shadow pointing at an identifier no row owns must still be not-found")
	})
}

func TestCompactUUIDTriggerSelfHealsDirectShadowWrites(t *testing.T) {
	// Section 6.3: "A type-valid direct compact value is overwritten where the engine
	// permits." SQLite's AFTER trigger re-derives the shadow from the row's own text, so a
	// direct write to a compact column is corrected in the same statement rather than
	// persisting as a lie. Text is never derived from compact, so there is no ambiguity about
	// which side wins.
	db, _ := newCompactTestTopology(t)
	expandAndTriggerAll(t, db, uuidRolePrimary)
	seedCompactUser(t, db, 1, compactUUIDTextFor(1))

	wrong, err := parseCompactUUID(compactUUIDTextFor(999))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 1",
		compactBindValue(dialectName(db), wrong)).Error)

	expected, err := parseCompactUUID(compactUUIDTextFor(1))
	require.NoError(t, err)
	require.Equal(t, upperOf(hexOf(expected)),
		readCompactShadowHex(t, db, "users", "uuid_compact", 1),
		"a direct compact write must be overwritten from authoritative text")

	var stored string
	require.NoError(t, db.Raw("SELECT uuid FROM users WHERE id = 1").Scan(&stored).Error)
	require.Equal(t, compactUUIDTextFor(1), stored, "text is never derived from compact")
}

func TestCompactUUIDLookupFallsBackWithoutFreshHealth(t *testing.T) {
	// A process with no fresh healthy audit must serve legacy predicates and still be correct.
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	driveCompactToReady(t, newCompactCoordinator(topology))

	t.Run("defaults to legacy before the first audit", func(t *testing.T) {
		resetCompactHealthForTest()
		enabled, reason := compactReadsEnabled(uuidRolePrimary)
		require.False(t, enabled, "a process must not use compact predicates before its first audit")
		require.NotEmpty(t, reason)

		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id, "the legacy path must remain fully correct")
	})

	t.Run("expired health forces legacy predicates", func(t *testing.T) {
		// An audit older than twice the idle interval is no evidence at all.
		publishCompactHealth(uuidRolePrimary, compactHealth{
			state:           compactStateReady,
			compactReadable: true,
			observedAt:      timeNowUTCForTest().Add(-10 * compactHealthTTL()),
		})
		t.Cleanup(resetCompactHealthForTest)

		enabled, reason := compactReadsEnabled(uuidRolePrimary)
		require.False(t, enabled, "expired health must force legacy predicates")
		require.Contains(t, reason, "older than")

		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id)
	})

	t.Run("health is per role and does not couple", func(t *testing.T) {
		resetCompactHealthForTest()
		publishCompactHealth(uuidRolePrimary, compactHealth{
			state:           compactStateReady,
			compactReadable: true,
			observedAt:      timeNowUTCForTest(),
		})
		t.Cleanup(resetCompactHealthForTest)

		enabled, _ := compactReadsEnabled(uuidRolePrimary)
		require.True(t, enabled)
		enabled, _ = compactReadsEnabled(uuidRoleLog)
		require.False(t, enabled, "a log-role audit failure must not ride on the primary's health")
	})
}

func TestCompactUUIDDriftRepair(t *testing.T) {
	// AUTO-T13/T14: repairable drift is fixed automatically, with no restart or command, and
	// the completion marker's timestamp never moves.
	db, topology := newCompactTestTopology(t)

	for index := 1; index <= 3; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)

	before := readMarkerTimestamp(t, db, compactPrimaryMigrationKey)

	t.Run("a gap is repaired automatically", func(t *testing.T) {
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)

		driveCompactToReady(t, coordinator)
		expected, err := parseCompactUUID(compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, upperOf(hexOf(expected)),
			readCompactShadowHex(t, db, "users", "uuid_compact", 1),
			"the worker must repair a gap from authoritative text")
	})

	t.Run("a mismatch is repaired automatically", func(t *testing.T) {
		wrong, err := parseCompactUUID(compactUUIDTextFor(777))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
			compactBindValue(dialectName(db), wrong)).Error)

		driveCompactToReady(t, coordinator)
		expected, err := parseCompactUUID(compactUUIDTextFor(2))
		require.NoError(t, err)
		require.Equal(t, upperOf(hexOf(expected)),
			readCompactShadowHex(t, db, "users", "uuid_compact", 2),
			"a verified mismatch is repaired from authoritative text")
	})

	t.Run("authoritative text is never mutated by repair", func(t *testing.T) {
		for index := 1; index <= 3; index++ {
			var stored string
			require.NoError(t, db.Raw("SELECT uuid FROM users WHERE id = ?", index).Scan(&stored).Error)
			require.Equal(t, compactUUIDTextFor(index), stored,
				"repair must never touch the authoritative column")
		}
	})

	t.Run("marker timestamps are stable across drift and repair", func(t *testing.T) {
		after := readMarkerTimestamp(t, db, compactPrimaryMigrationKey)
		require.Equal(t, before, after,
			"drift and repair must never rewrite a completion marker's timestamp")
	})
}

func TestCompactUUIDBlockedValidation(t *testing.T) {
	// AUTO-T15/T16: invalid authoritative data is fail-safe and observable, and correcting it
	// recovers automatically with no restart, flag, or CLI.
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	// A malformed owned UUID cannot be repaired without changing user data.
	seedCompactUser(t, db, 2, "not-a-valid-uuid")

	coordinator := newCompactCoordinator(topology)
	var result compactCycleResult
	for cycle := 0; cycle < 200; cycle++ {
		result = runCompactCycleForTest(t, coordinator)
		if result.state == compactStateBlockedValidation {
			break
		}
		require.NotEqual(t, compactStateReady, result.state,
			"compact must not complete over invalid authoritative data")
	}
	require.Equal(t, compactStateBlockedValidation, result.state)

	t.Run("the blocker is reported without exposing the value", func(t *testing.T) {
		require.NotEmpty(t, result.reason)
		require.NotContains(t, result.reason, "not-a-valid-uuid",
			"diagnostics must be aggregate and must not contain uuid values")
		require.Contains(t, result.reason, "users.uuid")
	})

	t.Run("no marker is written while blocked", func(t *testing.T) {
		complete, err := isDataMigrationComplete(ctx, db, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.False(t, complete, "a blocked migration must never write a completion marker")
	})

	t.Run("invalid text is never mutated", func(t *testing.T) {
		var stored string
		require.NoError(t, db.Raw("SELECT uuid FROM users WHERE id = 2").Scan(&stored).Error)
		require.Equal(t, "not-a-valid-uuid", stored,
			"blocked data must be reported, never silently corrected")
	})

	t.Run("legacy readiness and traffic continue", func(t *testing.T) {
		seedCompactUser(t, db, 3, compactUUIDTextFor(3))
		var count int64
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM users").Scan(&count).Error)
		require.Equal(t, int64(3), count, "blocked compact state must not affect legacy service")
	})

	t.Run("operator correction recovers automatically without a restart", func(t *testing.T) {
		// AUTO-T16: the same running coordinator must leave the blocked state on its next
		// audit and reach ready, with no restart, flag, or command.
		require.NoError(t, db.Exec("UPDATE users SET uuid = ? WHERE id = 2",
			compactUUIDTextFor(2)).Error)

		result := driveCompactToReady(t, coordinator)
		require.Equal(t, compactStateReady, result.state)

		complete, err := isDataMigrationComplete(ctx, db, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.True(t, complete, "the marker must appear once the source data is corrected")
	})
}

func TestCompactUUIDBlockedValidationNullAndEmpty(t *testing.T) {
	// A NULL owned uuid and an empty owned uuid are distinct legacy states, and both block.
	db, topology := newCompactTestTopology(t)
	coordinator := newCompactCoordinator(topology)

	seedCompactUserNullUUID(t, db, 1)
	seedCompactUser(t, db, 2, "")

	var result compactCycleResult
	for cycle := 0; cycle < 200; cycle++ {
		result = runCompactCycleForTest(t, coordinator)
		if result.state == compactStateBlockedValidation {
			break
		}
		require.NotEqual(t, compactStateReady, result.state)
	}
	require.Equal(t, compactStateBlockedValidation, result.state)
	require.Positive(t, result.blockers)

	// The two states stay distinct in the authoritative column, exactly as before compact.
	var nullCount, emptyCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM users WHERE uuid IS NULL").Scan(&nullCount).Error)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM users WHERE uuid = ''").Scan(&emptyCount).Error)
	require.Equal(t, int64(1), nullCount, "SQL NULL must remain distinct from empty text")
	require.Equal(t, int64(1), emptyCount)
}

func TestCompactUUIDBlockedValidationUniquePermutation(t *testing.T) {
	// AUTO-T14: "an unrepairable unique-value permutation ... enters blocked_validation; text
	// and marker timestamps never change."
	//
	// This is a regression test. The repair used to swallow the duplicate-key error and report
	// zero rows written, which left the row actionable forever: the cycle reported backfilling
	// on every pass and never reached the blocked state the contract requires.
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	seedCompactUser(t, db, 2, compactUUIDTextFor(2))
	driveCompactToReady(t, newCompactCoordinator(topology))

	// Build a permutation the worker cannot resolve row-by-row: row 2's shadow already holds
	// the value row 1's authoritative text derives to. Repairing row 1 would collide, and the
	// only row-level escape would be rewriting somebody's text, which is never allowed.
	dropCompactSyncTriggers(t, db, "users")
	target, err := parseCompactUUID(compactUUIDTextFor(1))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
		compactBindValue(dialectName(db), target)).Error)

	coordinator := newCompactCoordinator(topology)
	var result compactCycleResult
	for cycle := 0; cycle < 60; cycle++ {
		result = runCompactCycleForTest(t, coordinator)
		if result.state == compactStateBlockedValidation {
			break
		}
	}
	require.Equal(t, compactStateBlockedValidation, result.state,
		"an uncorrectable permutation must block, not spin as backfilling forever")
	require.Contains(t, result.reason, "uniqueness permutation")
	require.NotContains(t, result.reason, compactUUIDTextFor(1), "diagnostics must not carry uuid values")

	// Neither authoritative text nor the marker timestamp may move.
	for index := 1; index <= 2; index++ {
		var stored string
		require.NoError(t, db.Raw("SELECT uuid FROM users WHERE id = ?", index).Scan(&stored).Error)
		require.Equal(t, compactUUIDTextFor(index), stored,
			"a permutation must never cause text mutation")
	}
	complete, err := isDataMigrationComplete(ctx, db, compactPrimaryMigrationKey)
	require.NoError(t, err)
	require.True(t, complete, "an existing marker is never deleted by later drift")
}

func TestCompactUUIDOwnership(t *testing.T) {
	// AUTO-T03: at most one owner per topology may start a mutating side effect, and ownership
	// keys must not couple separate databases.
	_, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)

	t.Run("at most one owner at an instant", func(t *testing.T) {
		first, acquired, err := acquireCompactOwnership(ctx, topology)
		require.NoError(t, err)
		require.True(t, acquired)
		defer first.release()

		_, acquired, err = acquireCompactOwnership(ctx, topology)
		require.NoError(t, err, "a contended lock is an ordinary outcome, not an error")
		require.False(t, acquired, "a second owner must not acquire the same topology")

		held, err := first.verify(ctx)
		require.NoError(t, err)
		require.True(t, held)
	})

	t.Run("ownership is reacquirable after release", func(t *testing.T) {
		first, acquired, err := acquireCompactOwnership(ctx, topology)
		require.NoError(t, err)
		require.True(t, acquired)
		first.release()

		second, acquired, err := acquireCompactOwnership(ctx, topology)
		require.NoError(t, err)
		require.True(t, acquired, "a released claim must be reacquirable")
		second.release()
	})

	t.Run("keys do not couple separate databases", func(t *testing.T) {
		// Two unrelated one-api databases on one server must never contend.
		_, other := newCompactTestTopology(t)

		first, acquired, err := acquireCompactOwnership(ctx, topology)
		require.NoError(t, err)
		require.True(t, acquired)
		defer first.release()

		second, acquired, err := acquireCompactOwnership(ctx, other)
		require.NoError(t, err)
		require.True(t, acquired, "an unrelated database must not be blocked by this one's lock")
		defer second.release()
	})

	t.Run("a cycle without ownership is rejected before any side effect", func(t *testing.T) {
		require.Error(t, requireOwnership(ctx, nil))
	})
}
