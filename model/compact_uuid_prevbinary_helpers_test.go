package model

// Helpers for the preceding-production-build qualification (AUTO-013).
//
// Split out of compact_uuid_prevbinary_test.go only to keep both files inside the proposal's
// 600-line limit (section 9.3).

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// compactPrevManifestsSatisfiable reports whether every role's manifest still verifies. It asks
// the production function, per role, exactly as runCompactCycle does before any compact DDL, so a
// true result means "compact mutation is unblocked", not "a query this test invented agreed".
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the reads.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - bool: true when every role's manifest matches its baseline.
//   - string: bounded reason for the first role that does not.
func compactPrevManifestsSatisfiable(t *testing.T, ctx context.Context, topology *databaseTopology) (bool, string) {
	t.Helper()
	for _, role := range topology.targetRoles() {
		ok, reason, err := ensureLegacyIndexManifest(ctx, topology.handle(role), role)
		require.NoError(t, err)
		if !ok {
			return false, string(role) + ": " + reason
		}
	}
	return true, ""
}

// compactPrevOwnedTextIndexes lists the ordinary owned-UUID text indexes present on the engine.
// These are the objects the artifact's model tags declare and v3 deliberately drops, so their
// presence decides whether an ordinary rollback leaves the manifest satisfiable.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live PostgreSQL handle.
//
// Return values:
//   - []string: index names present, in deterministic order.
func compactPrevOwnedTextIndexes(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	wanted := []string{}
	for _, owned := range uuidOwnedRegistry() {
		wanted = append(wanted, ordinaryUUIDIndexName(owned.table))
	}
	names := []string{}
	require.NoError(t, db.Raw(
		"SELECT indexname FROM pg_indexes WHERE schemaname = 'public' AND indexname IN ? ORDER BY 1",
		wanted).Scan(&names).Error)
	return names
}

// compactPrevPromoteV3Indexes runs v3's real owned-UUID index promotion over every role.
//
// This makes the fixture a faithful production rollback target rather than a convenient one. A
// database a current binary migrated has been through v3's finalizer, which creates
// idx_<table>_uuid_unique and drops the ordinary idx_<table>_uuid it replaced. The live harness
// satisfies the v3 prerequisite by writing markers, skipping that DDL entirely, and a fixture
// missing a unique index would let the artifact's own v2 migration create it and thereby overstate
// the drift this suite measures.
//
// Both registry roles are promoted, through topology.handle, because both own tables in both
// topologies: driving this from the marker roles would silently skip every logs target in a
// unified deployment, the one table whose baseline this fixture would then get wrong. The
// production function is called directly; reproducing its DDL would be the raw SQL emulation
// section 2 refuses to accept.
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the metadata reads and DDL.
//   - topology: explicitly constructed database topology.
//
// Return values: none.
func compactPrevPromoteV3Indexes(t *testing.T, ctx context.Context, topology *databaseTopology) {
	t.Helper()
	for _, role := range topology.targetRoles() {
		for _, target := range ownedTargetsForRole(role) {
			require.NoError(t, promoteUUIDUniqueIndex(ctx, topology.handle(role), target),
				"v3's real finalizer must be able to promote %s", target.table)
		}
	}
}

// compactPrevAddedLines returns the catalog lines present after a run but not before it, so a
// failure is diagnosable rather than merely loud: the fingerprint is hundreds of lines.
// Parameters:
//   - before: catalog fingerprint captured before the artifact ran.
//   - after: catalog fingerprint captured after it ran.
//
// Return values:
//   - []string: added lines in their observed order.
func compactPrevAddedLines(before string, after string) []string {
	baseline := map[string]bool{}
	for _, line := range strings.Split(before, "\n") {
		baseline[line] = true
	}
	added := []string{}
	for _, line := range strings.Split(after, "\n") {
		if !baseline[line] {
			added = append(added, line)
		}
	}
	return added
}

// compactPrevDriveToTerminal runs real coordinator cycles until the state stops changing. Unlike
// driveCompactToReady it deliberately does NOT assert a state: this suite's job is to find out what
// the artifact actually did to compact health and hold that answer against the proposal, so the
// driver must report a blocked outcome rather than fail on it.
// Parameters:
//   - t: test handle used for assertions.
//   - coordinator: coordinator under test.
//
// Return values:
//   - compactCycleResult: the first terminal result, or the last result observed.
func compactPrevDriveToTerminal(t *testing.T, coordinator *compactCoordinator) compactCycleResult {
	t.Helper()
	const maxCycles = 200
	result := compactCycleResult{}
	for cycle := 0; cycle < maxCycles; cycle++ {
		result = runCompactCycleForTest(t, coordinator)
		switch result.state {
		case compactStateReady, compactStateBlockedValidation, compactStateDegraded:
			return result
		}
	}
	t.Fatalf("compact migration never reached a terminal state; last state %q reason %q",
		result.state, result.reason)
	return result
}

// compactPrevReadyFixture builds a validated-complete compact schema for the artifact to face.
// Reaching completion through the REAL coordinator, rather than by writing markers and columns
// directly, is what makes the later assertions mean anything: the objects the artifact must leave
// untouched are the objects production code actually creates. Ordering matters too — v3's index
// promotion happens BEFORE the first compact cycle, because the manifest is a pre-expansion
// baseline and the coordinator explicitly waits for v3 to finish its legacy index DDL.
// Parameters:
//   - t: test handle used for assertions.
//   - dialect: engine descriptor.
//
// Return values:
//   - *gorm.DB: primary handle.
//   - *databaseTopology: unified topology.
func compactPrevReadyFixture(t *testing.T, dialect compactLiveDialect) (*gorm.DB, *databaseTopology) {
	t.Helper()
	db, topology, ok := newLiveCompactTopology(t, dialect, false)
	require.True(t, ok, "the gate must have proven the DSN is configured")

	for index := 1; index <= 5; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}
	compactPrevPromoteV3Indexes(t, compactTestContext(t), topology)
	require.Empty(t, compactPrevOwnedTextIndexes(t, db),
		"v3's finalizer must have dropped every ordinary owned-uuid text index it replaced")

	require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state,
		"the fixture must reach validated completion before the artifact ever starts")
	return db, topology
}

func TestCompactUUIDOldBinaryDrift(t *testing.T) {
	binary, dialect, dsn, ok := compactPrevArtifact(t)
	if !ok {
		return
	}
	// The oldest-artifact suite defaults to 13999; a distinct port lets both run in one package.
	t.Setenv(compactOldBinaryPortEnv, compactPrevBinaryPort)

	db, topology := compactPrevReadyFixture(t, dialect)
	ctx := compactTestContext(t)

	before := compactCatalogFingerprint(t, db)
	require.Contains(t, before, "COL|users|uuid_compact", "the fixture must really be expanded")
	require.Contains(t, before, "TRG|cuuid_v1_users_sync", "the fixture must really be triggered")
	require.Contains(t, before, "MARK|"+compactPrimaryMigrationKey, "the fixture must really be complete")
	markerBefore := readMarkerTimestamp(t, db, compactPrimaryMigrationKey)
	manifestBefore := compactPrevManifestChecksums(t, ctx, topology)

	// Emptying users makes the artifact's startup create the root account: a real write through its
	// own writer contract rather than a statement this test composed.
	require.NoError(t, db.Exec("DELETE FROM users").Error)

	load := compactPrevStartLegacyLoad(t, compactPrevOpenLoadHandle(t, dialect, dsn))
	output := runPinnedOldBinary(t, binary, oldBinaryDSN(dsn), compactPrevSettleFor)
	load.halt()

	after := compactCatalogFingerprint(t, db)
	users := compactPrevReadUsers(t, db)

	t.Run("the preceding build starts and runs its own AutoMigrate", func(t *testing.T) {
		// AUTO-T05/T06: the artifact's own AutoMigrate must run to completion over a schema full of
		// compact objects it has never heard of. Its own log line is what proves the migration path
		// ran, rather than the process merely having been alive.
		require.Contains(t, output, "database schema migrated",
			"the preceding build's own AutoMigrate must have run; output:\n%s", output)
		require.Contains(t, output, "database migration completed",
			"the preceding build's whole startup migration must succeed; output:\n%s", output)
	})

	t.Run("legacy service is unaffected while the preceding build is live", func(t *testing.T) {
		// AUTO-T04: ordinary legacy CRUD on its own connection, through the permanently
		// authoritative text contract, for the whole time the artifact is up and migrating.
		t.Logf("acknowledged legacy operations while the preceding build was live: %d", load.operations)
		require.NoError(t, load.failure, "legacy CRUD must not fail while the preceding build is live")
		require.GreaterOrEqual(t, load.operations, compactPrevRequiredOperations,
			"AUTO-T04 requires at least %d acknowledged operations", compactPrevRequiredOperations)
	})

	t.Run("the preceding build's write is derived or blocks, never silently wrong", func(t *testing.T) {
		// The empirical question section 2.2 fixes the answer to. The artifact predates v3, so
		// whether its root-account insert leaves an owned UUID behind is a property of the real
		// artifact, not something this test may assume either way.
		require.NotEmpty(t, users, "the preceding build must have written its root account")
		for _, row := range users {
			t.Logf("row %d written by the preceding build: owned uuid blank=%t, shadow derived=%t",
				row.ID, row.blank(), row.Shadow != nil)
			if row.blank() {
				// Section 2.2: a missing owned UUID cannot be repaired without changing user
				// data, so it derives NULL and blocks health. It must never be fabricated.
				require.Nil(t, row.Shadow,
					"a missing owned uuid must derive NULL, never a fabricated shadow (row %d)", row.ID)
				continue
			}
			require.NotNil(t, row.Shadow,
				"a valid owned uuid written by the preceding build must derive its shadow atomically (row %d)", row.ID)
			require.NotNil(t, row.Agrees)
			require.True(t, *row.Agrees,
				"the derived shadow must equal the authoritative text the preceding build wrote (row %d)", row.ID)
		}
	})

	t.Run("compact health tells the truth about that write", func(t *testing.T) {
		// Section 2.2 again: a UUID-less owned row must place health in blocked_validation. The
		// failure this asserts against is compact reporting ready over a row it cannot resolve.
		blanks := 0
		for _, row := range users {
			if row.blank() {
				blanks++
			}
		}
		result := compactPrevDriveToTerminal(t, newCompactCoordinator(topology))
		t.Logf("compact state after the preceding build ran: %q (reason %q, blank owned uuids %d)",
			result.state, result.reason, blanks)
		if blanks > 0 {
			require.Equal(t, compactStateBlockedValidation, result.state,
				"a missing owned uuid must block compact health, never complete silently; reason %q", result.reason)
			require.Positive(t, result.blockers, "the blocked state must be backed by counted blockers")
		} else {
			require.Equal(t, compactStateReady, result.state,
				"the preceding build wrote only valid owned uuids, so compact must stay ready; reason %q", result.reason)
		}

		// Either way the authoritative text and the marker history are untouchable.
		require.Equal(t, markerBefore, readMarkerTimestamp(t, db, compactPrimaryMigrationKey),
			"no compact side effect may rewrite a completion marker's timestamp")
		require.Equal(t, users, compactPrevReadUsers(t, db),
			"no compact side effect may mutate the authoritative legacy text the artifact wrote")
	})

	t.Run("the preceding build's AutoMigrate loses nothing and only adds its own indexes", func(t *testing.T) {
		// AUTO-T21 forbids "drop, rename, retype, nullability/collation change, or rewrite".
		// Containment is therefore the assertion that matters, and it is asserted in full: every
		// single catalog line captured before the artifact ran must still be present after.
		// Nothing may vanish or change shape.
		for _, line := range strings.Split(before, "\n") {
			require.Contains(t, after, line,
				"the preceding build's AutoMigrate dropped, renamed, retyped, or rewrote %q", line)
		}

		// Byte-identity is deliberately NOT asserted for THIS artifact, and the reason is a fact
		// about it rather than a concession. ed15a144 declares `gorm:"...;index;column:uuid"` on
		// every owned UUID column, so its AutoMigrate is *defined* to create idx_<table>_uuid.
		// Section 2 requires this build to stay supported for rollback, so demanding that its
		// AutoMigrate add nothing would demand it not be itself. The oldest supported rollback
		// build (4dfec29a) dropped that tag, and TestCompactUUIDOldBinary does assert byte-identity
		// for it — so the strict form is still enforced where it is achievable.
		//
		// What is asserted instead is that every addition is one of ITS OWN declared owned-uuid
		// text indexes. Anything else appearing would be unexplained and is a failure.
		added := compactPrevAddedLines(before, after)
		t.Logf("catalog lines the preceding build added:\n%s", strings.Join(added, "\n"))
		allowed := map[string]struct{}{}
		for _, owned := range uuidOwnedRegistry() {
			allowed["IDX|"+ordinaryUUIDIndexName(owned.table)] = struct{}{}
		}
		for _, line := range added {
			prefix := line
			if cut := strings.LastIndex(line, "|"); cut > 0 {
				prefix = line[:cut]
			}
			_, ok := allowed[prefix]
			require.True(t, ok,
				"the preceding build added %q, which is not one of its own declared owned-uuid indexes", line)
		}
	})

	t.Run("an ordinary rollback leaves the legacy-index manifest satisfiable", func(t *testing.T) {
		// This is the regression test for a real defect this artifact exposed. Section 2 requires
		// ed15a144 to remain supported; AUTO-T10 requires automatic work to reconverge across
		// old -> new -> old -> new "without a command"; section 10.2 promises redeploying the new
		// binary "resumes from database state and reconverges".
		//
		// The manifest verified by SET-EQUALITY, so the 12 owned-uuid indexes this artifact's own
		// AutoMigrate legitimately re-adds made the checksum diverge and wedged compact in
		// blocked_validation forever — on a database nothing had damaged, with no operator recourse
		// short of dropping indexes the running old binary wants. Verification is now SUBSET: every
		// captured index must survive unchanged, but an addition is not a violation.
		require.NotEmpty(t, compactPrevOwnedTextIndexes(t, db),
			"this artifact is only interesting because it really does re-add owned-uuid text indexes")
		require.Equal(t, manifestBefore, compactPrevManifestChecksums(t, ctx, topology),
			"the durable legacy-index manifest must never be rewritten to launder what it observed")

		satisfiable, reason := compactPrevManifestsSatisfiable(t, ctx, topology)
		require.True(t, satisfiable,
			"an ordinary rollback to a build section 2 requires to be supported must not block compact: %s", reason)
	})

	t.Run("the blocked state is caused by the added indexes and only an operator clears it", func(t *testing.T) {
		// Diagnostic, deliberately last: it isolates the cause rather than asserting the contract.
		// If the suite above is red, this is the proof of why — compact reconverges the instant an
		// operator removes the indexes the artifact added, and not before. Section 10.3 forbids the
		// product itself from dropping legacy UUID indexes, so this DROP is an operator action that
		// zero-touch completion has no place for.
		added := compactPrevOwnedTextIndexes(t, db)
		if len(added) == 0 {
			t.Skip("the preceding build added no owned-uuid text index; there is nothing to isolate")
		}
		for _, name := range added {
			require.NoError(t, db.Exec("DROP INDEX "+quoteIdentifier(db, name)).Error)
		}

		satisfiable, reason := compactPrevManifestsSatisfiable(t, ctx, topology)
		require.True(t, satisfiable,
			"removing exactly the indexes the artifact added must restore every role's manifest: %s", reason)
		require.Equal(t, manifestBefore, compactPrevManifestChecksums(t, ctx, topology),
			"the manifest itself must never have been rewritten to launder the change")
		result := compactPrevDriveToTerminal(t, newCompactCoordinator(topology))
		t.Logf("compact state after removing the added indexes: %q (reason %q)", result.state, result.reason)
		require.Equal(t, compactStateReady, result.state,
			"compact must reconverge once the added indexes are gone, proving they were the sole cause; reason %q",
			result.reason)
		require.Equal(t, markerBefore, readMarkerTimestamp(t, db, compactPrimaryMigrationKey),
			"reconvergence must not rewrite the completion marker's timestamp")
	})
}
