package model

// AUTO-T10 mixed-version cycling: old -> new -> old -> new across held migration states, using
// the real pinned pre-migration binary. Split from compact_uuid_fault_test.go only for the
// 600-line ceiling (proposal section 9.3); the fault helpers it drives live there.

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactUUIDMixedVersionDrift(t *testing.T) {
	// AUTO-T10: old -> new -> old -> new at every migration percentage, where "old" is the real
	// pinned pre-migration artifact executed against the same database, not an emulation of it.
	binary := strings.TrimSpace(os.Getenv(compactOldBinaryEnv))
	if binary == "" {
		t.Skipf("%s is not configured; CI's no-skip guard enforces this suite", compactOldBinaryEnv)
	}
	dsn := strings.TrimSpace(os.Getenv(compactFaultDialect().primaryEnv))
	if dsn == "" {
		t.Skipf("%s is not configured; CI's no-skip guard enforces this suite",
			compactFaultDialect().primaryEnv)
	}

	for _, stage := range []struct {
		name  string
		state compactState
	}{
		{"zero", ""},
		{"expansion_partial", compactStateExpanding},
		{"indexed_partial", compactStateIndexing},
		{"backfill_partial", compactStateBackfilling},
		{"validated", compactStateValidating},
		{"marked", compactStateReady},
	} {
		t.Run(stage.name, func(t *testing.T) { compactFaultCycleVersions(t, binary, dsn, stage.state) })
	}
}

// compactFaultCycleVersions runs old -> new -> old -> new at one migration percentage.
// Parameters:
//   - t: test handle used for assertions.
//   - binary: absolute path to the pinned pre-migration artifact.
//   - dsn: key/value DSN of the database under test.
//   - stage: the state to bring the database to first; empty means no compact work at all.
//
// Return values: none.
func compactFaultCycleVersions(t *testing.T, binary string, dsn string, stage compactState) {
	db, topology, ctx := compactFaultFixture(t, compactFaultBacklogRows, 0)
	// A private port keeps concurrent agents' pinned artifacts from colliding.
	t.Setenv(compactOldBinaryPortEnv, "14093")
	digest := compactFaultDigest(db, compactFaultBacklogRows)
	if stage != "" {
		compactFaultAdvanceTo(t, ctx, newCompactCoordinator(topology), stage)
	}

	for round := 1; round <= 2; round++ {
		before := compactCatalogFingerprint(t, db)
		output := runPinnedOldBinary(t, binary, oldBinaryDSN(dsn), compactFaultOldSettle)
		require.Contains(t, output, "database schema migrated",
			"the pinned artifact's own AutoMigrate must have run in round %d; output:\n%s", round, output)
		require.Equal(t, before, compactCatalogFingerprint(t, db),
			"round %d: the pinned artifact must leave every compact and legacy object byte-identical", round)
		require.Equal(t, digest, compactFaultDigest(db, compactFaultBacklogRows),
			"round %d: the pinned artifact must not move the fixture's authoritative text", round)

		// The new binary is redeployed: automatic work resumes from database state, no command.
		coordinator := newCompactCoordinator(topology)
		if round == 2 {
			require.Equal(t, compactStateReady, driveCompactToReady(t, coordinator).state,
				"the migration must reconverge to ready after the full old/new cycle")
			continue
		}
		resumed := false
		for cycle := 0; cycle < 3; cycle++ {
			result, err := compactFaultRunCycle(ctx, coordinator)
			require.NoError(t, err, "the redeployed binary must resume automatically")
			require.NotEqual(t, compactStateBlockedValidation, result.state,
				"a rollback cycle must never block the migration: %s", result.reason)
			resumed = resumed || result.progressed || result.state == compactStateReady
		}
		require.True(t, resumed, "the redeployed binary must make progress or already be complete")
	}

	require.True(t, compactFaultMarkerIntegrity(t, ctx, topology),
		"the completed migration must carry a genuine marker")
	require.Equal(t, digest, compactFaultDigest(db, compactFaultBacklogRows))
	for _, index := range []int{1, compactFaultBacklogRows / 2, compactFaultBacklogRows} {
		requireLiveShadowMatches(t, db, compactFaultDialect(), index, compactUUIDTextFor(index))
	}
}
