package model

// Section 12 compatibility workload for compact UUID storage
// (docs/proposals/20260715_compact-uuid-storage.md):
//
//	"The compatibility workload uses the recorded fixture hash, eight concurrent clients, at
//	 least 10 requests/second, and at least 1,000 successful operations per held migration
//	 state: 30% creates/updates covering every writer target, 40% exact reads covering every
//	 owned type, and 10% each search/report/cache paths. Status, payload, row count, ordering,
//	 and acknowledged-write reconciliation are compared with a migration-disabled legacy
//	 baseline."
//
// The workload runs against a real PostgreSQL 17 server while the migration is HELD at each of
// pre-expansion, expansion, indexing, partial backfill, validation, and marked/ready — no
// coordinator cycle runs during a hold, so the state genuinely stands still under traffic. The
// identical deterministic operation stream then replays against a second database that never
// migrates at all (config.CompactUUIDAutoMigrate=false, zero compact objects), and every
// category's outcome must match byte for byte: a held state may change timing, never semantics.
//
// The per-state hold defaults to a few seconds and asserts the operation count rather than the
// wall clock (the 60-second hold is AUTO-T09's, which has its own suite); WORKLOAD_HOLD_SECONDS
// lengthens the holds in CI without changing the schedule's semantics.
//
// COMPACT_UUID_TEST_POSTGRES_DSN and COMPACT_UUID_TEST_POSTGRES_BASE_DSN gate this test. A
// missing variable skips locally so an ordinary `go test ./...` still passes on a laptop; CI's
// no-skip guard fails the run instead of letting it go green.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

func TestCompactUUIDCompatibilityWorkload(t *testing.T) {
	dialect := compactFaultDialect()
	if strings.TrimSpace(os.Getenv(dialect.primaryEnv)) == "" {
		compactLiveSkipf(t, "%s is not configured", dialect.primaryEnv)
	}
	if strings.TrimSpace(os.Getenv(compactWorkloadBaseDSNEnv)) == "" {
		compactLiveSkipf(t, "%s is not configured", compactWorkloadBaseDSNEnv)
	}
	// The cache category must exercise the real SQL fallback, so Redis is disabled for the
	// test's duration. The flag cannot be asserted instead: it initializes to true and only
	// InitRedisClient — which test processes never call — turns it off, so a bare test process
	// reports "enabled" with no Redis anywhere near it.
	originalRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(originalRedis) })

	schedule, err := compactWorkloadScheduleFromEnv()
	require.NoError(t, err)
	t.Logf("schedule: %d workers, %d template operations per worker per state, %s hold, %s pace",
		compactWorkloadWorkers, schedule.opsPerWorker, schedule.holdFor, schedule.pace)

	// The baseline replays first so no residue of the migrated run — package-global health
	// gates, marker rows, metrics — can possibly precede it. Both runs then reduce to
	// deterministic outcomes that are compared category by category.
	baseline := compactWorkloadBaselineOutcome(t, schedule)
	migrated := compactWorkloadMigratedOutcome(t, schedule)
	compareCompactWorkloadOutcomes(t, migrated, baseline)
}

// compactWorkloadBaselineOutcome replays the deterministic stream against the second database
// with the migration disabled entirely: no coordinator, no shadow column, no trigger, no marker.
// Its outcome defines what "legacy" means for the comparison.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - schedule: deterministic shape shared with the migrated run.
//
// Return values:
//   - compactWorkloadOutcome: the baseline's reconciled outcome.
func compactWorkloadBaselineOutcome(t *testing.T, schedule compactWorkloadSchedule) compactWorkloadOutcome {
	t.Helper()
	dialect := compactFaultDialect()
	withCompactTestSettings(t)
	originalAuto := config.CompactUUIDAutoMigrate
	config.CompactUUIDAutoMigrate = false
	defer func() { config.CompactUUIDAutoMigrate = originalAuto }()

	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(true)
	t.Cleanup(func() {
		common.UsingSQLite.Store(true)
		common.UsingMySQL.Store(false)
		common.UsingPostgreSQL.Store(false)
	})

	db := openLiveCompactDB(t, dialect, strings.TrimSpace(os.Getenv(compactWorkloadBaseDSNEnv)))
	withTestDBGlobals(t, db, db)
	require.NoError(t, migrateDB())
	require.Zero(t, compactFaultCount(t, db, compactFaultShadowCountSQL),
		"the baseline must start with no compact object at all")
	compactFaultSeed(t, db, compactWorkloadFixtureRows)

	ctx, cancel := context.WithTimeout(withCompactLogger(context.Background()), 15*time.Minute)
	defer cancel()
	run := newCompactWorkloadRun(t, db, schedule, false)
	for segment := 0; segment < compactWorkloadSegments; segment++ {
		ops, elapsed := run.runSegment(t, ctx, segment)
		t.Logf("baseline segment %d: %d operations in %s", segment, ops, elapsed)
	}
	require.Zero(t, compactFaultCount(t, db, compactFaultShadowCountSQL),
		"the baseline must never gain a compact object")
	return run.finish(t)
}

// compactWorkloadMigratedOutcome runs the workload while the real coordinator's migration is
// held at each required state, asserting the per-state operation floor, the aggregate rate, and
// the fixture's authoritative-text stability, then reconciles the run's outcome.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - schedule: deterministic shape shared with the baseline run.
//
// Return values:
//   - compactWorkloadOutcome: the migrated run's reconciled outcome.
func compactWorkloadMigratedOutcome(t *testing.T, schedule compactWorkloadSchedule) compactWorkloadOutcome {
	t.Helper()
	dialect := compactFaultDialect()

	// The bounded row budget is what makes the backfilling hold honest: the users fixture is
	// larger than one cycle's budget, so an unfinished fill provably exists to be held.
	originalBudget := config.CompactUUIDMaxRowsPerCycle
	config.CompactUUIDMaxRowsPerCycle = config.MinCompactUUIDMaxRowsPerCycle
	t.Cleanup(func() { config.CompactUUIDMaxRowsPerCycle = originalBudget })

	db, topology, ok := newLiveCompactTopology(t, dialect, false)
	if !ok {
		compactLiveSkipf(t, "%s is not configured", dialect.primaryEnv)
	}
	ctx, cancel := context.WithTimeout(withCompactLogger(context.Background()), 15*time.Minute)
	t.Cleanup(cancel)
	compactFaultSeed(t, db, compactWorkloadFixtureRows)
	fixtureDigest := compactFaultDigest(db, compactWorkloadFixtureRows)
	coordinator := newCompactCoordinator(topology)

	// Every process audits read-only on its own cadence, so lookup health evolves under the
	// workload exactly as it would in production — including enabling verified compact reads
	// once the migration is marked.
	auditStop, auditDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(auditDone)
		for {
			runCompactHealthAudit(ctx, topology)
			select {
			case <-auditStop:
				return
			case <-time.After(compactIdleInterval()):
			}
		}
	}()
	t.Cleanup(func() { close(auditStop); <-auditDone })

	run := newCompactWorkloadRun(t, db, schedule, true)
	states := []struct {
		name string
		// reach drives real coordinator cycles until the database reports the state.
		reach func()
		// prove asserts the database really is held at the state, after the workload ran.
		prove func()
	}{
		{"pre_expansion", func() {}, func() {
			require.Zero(t, compactFaultCount(t, db, compactFaultShadowCountSQL),
				"the pre-expansion hold must really have no shadow at all")
		}},
		{"expansion", func() {
			compactFaultAdvanceTo(t, ctx, coordinator, compactStateExpanding)
		}, func() {
			expanded := compactFaultCount(t, db, compactFaultShadowCountSQL)
			require.Positive(t, expanded, "the expansion hold must have expanded something")
			require.Less(t, expanded, len(compactRegistry()), "the expansion hold must be partial")
		}},
		{"indexing", func() {
			compactFaultAdvanceTo(t, ctx, coordinator, compactStateIndexing)
		}, func() {
			require.Positive(t, compactFaultCount(t, db,
				"SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname LIKE '%compact%'"),
				"the indexing hold must have created a compact index")
		}},
		{"backfilling", func() {
			compactFaultAdvanceTo(t, ctx, coordinator, compactStateBackfilling)
		}, func() {
			require.Positive(t, compactFaultCount(t, db,
				"SELECT count(*) FROM users WHERE uuid_compact IS NULL"),
				"the backfilling hold must still be provably partial: the fixture exceeds one cycle's budget")
		}},
		{"validating", func() {
			compactFaultAdvanceTo(t, ctx, coordinator, compactStateValidating)
		}, func() {
			require.False(t, compactFaultMarkerIntegrity(t, ctx, topology),
				"validation in progress must never carry a completion marker")
		}},
		{"marked", func() {
			require.Equal(t, compactStateReady, driveCompactToReady(t, coordinator).state)
		}, func() {
			require.True(t, compactFaultMarkerIntegrity(t, ctx, topology),
				"the marked hold must carry a genuine completion marker")
		}},
	}

	var totalOps int64
	var totalElapsed time.Duration
	for segment, state := range states {
		state.reach()
		ops, elapsed := run.runSegment(t, ctx, segment)
		require.GreaterOrEqual(t, ops, int64(1000),
			"the %s state must be held under at least 1,000 successful operations", state.name)
		require.GreaterOrEqual(t, elapsed, schedule.holdFor,
			"the %s state must be held for at least the scheduled duration", state.name)
		state.prove()
		require.Equal(t, fixtureDigest, compactFaultDigest(db, compactWorkloadFixtureRows),
			"the fixture's authoritative text must never move under the %s hold", state.name)
		totalOps += ops
		totalElapsed += elapsed
		t.Logf("migrated state %s: %d operations in %s (%.1f ops/s)",
			state.name, ops, elapsed, float64(ops)/elapsed.Seconds())
	}

	rate := float64(totalOps) / totalElapsed.Seconds()
	require.GreaterOrEqual(t, rate, compactWorkloadMinRate,
		"the eight clients must sustain the aggregate request-rate floor")
	t.Logf("workload total: %d operations across %d held states in %s (%.1f ops/s aggregate)",
		totalOps, len(states), totalElapsed, rate)
	return run.finish(t)
}
