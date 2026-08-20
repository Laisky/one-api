package model

// Fault-injection, barrier-hold, and mixed-version qualification for compact UUID storage
// (AUTO-T09/T10/T11/T12; proposal sections 8.4, 8.6, and 10.2). Fixtures live in
// compact_uuid_fault_harness_test.go.
//
// Everything here drives the REAL coordinator against a REAL PostgreSQL 17 server. Nothing
// emulates a trigger, a catalog read, or a marker write in Go: the interesting failures of a
// fault suite live in the engine's half of the contract — a DDL that half-committed, a pool
// that kept a dead socket, a lock wait that outlived its bound — and an emulation would fake
// exactly those.
//
// How a "kill" is simulated, and why it is honest: killing `go test` itself would take the
// assertions with it, so a kill is modelled as the two things a real SIGKILL costs the
// coordinator, applied together — the cycle's context is cancelled, so in-flight statements die
// exactly as they would when the process vanishes, AND the *compactCoordinator is discarded, so
// the restart begins with the empty cursors, zero clean-pass streak, and fresh worker token a
// new process gets. Section 8.4's "one attempt per side effect per cycle" makes this precise:
// one cycle is one side effect, so cancelling cycle N kills side effect N, and discarding the
// coordinator after cycle N is a restart immediately after side effect N committed.
//
// Honest scope, stated up front rather than buried:
//   - The workload is smaller than Section 12's: 4 paced clients (~50 iterations/s each, four
//     operations per iteration) rather than 8 clients at 10 rps, and each barrier is held for 3
//     seconds rather than 60. Every barrier still clears more than 1,000 operations and many
//     audit intervals.
//   - The assigned database is a single PostgreSQL database, so the topology is unified and
//     carries exactly ONE marker key. Partial-marker qualification needs split mode (AUTO-T17).
//   - Only PostgreSQL is covered here; the MySQL half of the matrix is another file.
//
// COMPACT_UUID_TEST_POSTGRES_DSN and COMPACT_UUID_TEST_OLD_BINARY gate these tests. A missing
// variable skips locally so an ordinary `go test ./...` still passes on a laptop; CI's no-skip
// guard fails the run instead of letting it go green.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// compactFaultRunCycle runs exactly one real coordinator cycle under a real ownership claim.
//
// A contended acquisition is retried within the resumption budget the proposal itself grants a
// restarted owner — one lock timeout plus one active interval (AUTO-T32; section 8.6 likewise
// allows "within one active interval"). The budget is not leniency, it is the realistic model:
// when a kill lands inside a previous acquisition, that owner's pinned connection is discarded
// and PostgreSQL releases its advisory lock when the backend exits, which is asynchronous by a
// few milliseconds. A restarted process cannot possibly reacquire before the server reaps its
// predecessor, and demanding that it do so fails the harness on physics rather than on product
// behavior. The bound still catches the real defect this sweep once found: a lock stranded on a
// connection returned ALIVE to the pool never clears, so the retry loop still times out and
// fails loudly.
// Parameters:
//   - ctx: context bounding the cycle; cancelling it is how this suite kills the owner.
//   - coordinator: coordinator under test.
//
// Return values:
//   - compactCycleResult: the cycle's result.
//   - error: wrapped error from ownership or from the cycle.
func compactFaultRunCycle(ctx context.Context, coordinator *compactCoordinator) (compactCycleResult, error) {
	deadline := time.Now().Add(compactLockTimeout() + compactActiveInterval())
	for {
		ownership, acquired, err := acquireCompactOwnership(ctx, coordinator.topology)
		if err != nil {
			return compactCycleResult{}, errors.Wrap(err, "acquire compact ownership for fault cycle")
		}
		if acquired {
			defer ownership.release()
			return runCompactCycle(ctx, coordinator, ownership)
		}
		if time.Now().After(deadline) {
			return compactCycleResult{}, errors.New(
				"compact ownership was not acquired within one lock timeout plus one active interval; " +
					"a lock stranded on a pooled connection never clears, so this is the stranded-lock " +
					"signature rather than ordinary teardown latency")
		}
		select {
		case <-ctx.Done():
			return compactCycleResult{}, errors.Wrap(ctx.Err(), "cancelled while reacquiring compact ownership")
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// compactFaultKill kills one cycle by cancelling it after a fixed delay.
//
// A zero delay kills before the first statement; a non-zero delay lands inside the cycle and,
// across the swept values, reaches into DDL, batch, validation, and marker work. Either outcome
// is legal, so nothing is asserted about the error the cycle returns: the contract is asserted
// afterwards, against the database, which is the only place a real crash leaves evidence.
// Parameters:
//   - coordinator: coordinator to kill; the caller discards it afterwards.
//   - delay: how long the cycle runs before the kill lands.
//
// Return values: none.
func compactFaultKill(coordinator *compactCoordinator, delay time.Duration) {
	ctx, cancel := context.WithCancel(withCompactLogger(context.Background()))
	defer cancel()
	if delay <= 0 {
		cancel()
	} else {
		defer time.AfterFunc(delay, cancel).Stop()
	}
	if _, err := compactFaultRunCycle(ctx, coordinator); err != nil {
		// This harness drives runCompactCycle directly, bypassing the production worker
		// loop — so it must reproduce what that loop does on a failed cycle: reset the
		// clean-pass epoch (runCompactWorkerCycle's error branch, per section 8.5's "the
		// clean-pass epoch resets on ... retry"). Without this, a coordinator that survives
		// its killed cycle keeps a recorded clean pass the production worker would have
		// discarded, and its next cycle completes without ever re-reporting validation.
		coordinator.resetEpoch()
	}
}

// compactFaultAdvanceTo drives real cycles until the coordinator reports the wanted state.
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the cycles.
//   - coordinator: coordinator under test.
//   - wanted: the barrier state to stop at.
//
// Return values:
//   - compactCycleResult: the cycle that reported the wanted state.
func compactFaultAdvanceTo(t *testing.T, ctx context.Context, coordinator *compactCoordinator,
	wanted compactState) compactCycleResult {
	t.Helper()
	for cycle := 0; cycle < compactFaultMaxCycles; cycle++ {
		result, err := compactFaultRunCycle(ctx, coordinator)
		require.NoError(t, err, "cycle %d on the way to %q", cycle, wanted)
		require.NotEqual(t, compactStateBlockedValidation, result.state,
			"compact migration blocked on the way to %q: %s", wanted, result.reason)
		if result.state == wanted {
			return result
		}
		if wanted != compactStateReady && result.state == compactStateReady {
			t.Fatalf("the coordinator reached ready before ever reporting %q", wanted)
		}
	}
	t.Fatalf("the coordinator never reported %q within %d cycles", wanted, compactFaultMaxCycles)
	return compactCycleResult{}
}

func TestCompactUUIDFaultInjection(t *testing.T) {
	// AUTO-T09, AUTO-T11, and AUTO-T12 against a real PostgreSQL 17 server.
	if hooks := strings.TrimSpace(os.Getenv(compactFaultHooksEnv)); hooks != "" {
		t.Logf("%s=%s is set; this suite injects faults from outside the coordinator and needs "+
			"no in-process hooks", compactFaultHooksEnv, hooks)
	}
	t.Run("barrier hold under traffic", compactFaultBarrierHold)
	t.Run("partial backfill barrier", compactFaultPartialBackfillBarrier)
	t.Run("kill around every side effect", compactFaultKillAroundSideEffects)
	t.Run("outage lock and cancellation", compactFaultInjectedFaults)
}

// compactFaultBarrierHold holds each migration state under concurrent traffic (AUTO-T09).
//
// The barriers are visited in the coordinator's real dependency order, which Section 8.4 fixes
// as expansion, then indexing, then fill: an index is created BEFORE historical fill so the
// fill lands into an existing index. So "indexing" legitimately precedes fill here rather than
// following it as the test-matrix row's prose ordering suggests.
//
// The partial-backfill barrier is NOT here: it needs a fill that cannot finish in one cycle,
// which this fixture deliberately does not have. It has its own test, and that test currently
// fails against a real coordinator defect — see compactFaultPartialBackfillBarrier.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func compactFaultBarrierHold(t *testing.T) {
	db, topology, ctx := compactFaultFixture(t, compactFaultSeedRows, 0)
	digest := compactFaultDigest(db, compactFaultSeedRows)
	coordinator := newCompactCoordinator(topology)
	traffic := compactFaultStartTraffic(t, db, 4)

	// Every process audits read-only on its own cadence, so the audit runs against the traffic
	// rather than only in the gaps between barriers.
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

	barriers := []struct {
		name  string
		reach func(*testing.T)
		prove func(*testing.T)
	}{
		{"pre_expansion", func(t *testing.T) {}, func(t *testing.T) {
			require.Zero(t, compactFaultCount(t, db, compactFaultShadowCountSQL),
				"the pre-expansion barrier must really have no shadow at all")
		}},
		{"expansion", func(t *testing.T) {
			compactFaultAdvanceTo(t, ctx, coordinator, compactStateExpanding)
		}, func(t *testing.T) {
			expanded := compactFaultCount(t, db, compactFaultShadowCountSQL)
			require.Positive(t, expanded, "the expansion barrier must have expanded something")
			require.Less(t, expanded, len(compactRegistry()), "the expansion barrier must be partial")
		}},
		{"indexing", func(t *testing.T) {
			compactFaultAdvanceTo(t, ctx, coordinator, compactStateIndexing)
		}, func(t *testing.T) {
			require.Positive(t, compactFaultCount(t, db,
				"SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname LIKE '%compact%'"),
				"the indexing barrier must have created an index")
		}},
		{"validation", func(t *testing.T) {
			compactFaultAdvanceTo(t, ctx, coordinator, compactStateValidating)
		}, func(t *testing.T) {
			require.False(t, compactFaultMarkerIntegrity(t, ctx, topology),
				"validation in progress must never carry a completion marker")
		}},
		{"marked", func(t *testing.T) {
			require.Equal(t, compactStateReady, driveCompactToReady(t, coordinator).state)
		}, func(t *testing.T) {
			require.True(t, compactFaultMarkerIntegrity(t, ctx, topology),
				"the marked barrier must carry a genuine marker")
		}},
	}

	for _, barrier := range barriers {
		t.Run(barrier.name, func(t *testing.T) {
			barrier.reach(t)
			before := traffic.ops.Load()
			time.Sleep(compactFaultHoldFor) // The hold: no cycle runs while the workload keeps going.
			require.NoError(t, traffic.firstFailure(), "the workload must survive the %s barrier", barrier.name)
			barrier.prove(t)
			require.Greater(t, traffic.ops.Load()-before, int64(1000),
				"the %s barrier must be held under at least 1,000 operations", barrier.name)
			require.Equal(t, digest, compactFaultDigest(db, compactFaultSeedRows),
				"the fixture's authoritative text must never move")
		})
	}
	traffic.stopAndReconcile(t, db)
	require.Equal(t, digest, compactFaultDigest(db, compactFaultSeedRows))
}

// compactFaultPartialBackfillBarrier holds an unfinished historical fill under traffic (AUTO-T09).
//
// It is separate from the other barriers because it is the one that needs a fill larger than a
// single cycle's row budget, and that is exactly the shape the coordinator currently cannot
// complete. This test FAILS, and the failure is a real defect rather than a fixture problem:
//
// compactRegistry sorts targets by (role, table, legacyColumn), so users.inviter_uuid is
// reconciled before users.uuid. runCompactReconciliation gives every target a single SHARED
// row budget in that order and breaks when it is spent, while reconcileCompactTarget
// (compact_uuid_backfill.go:122) seeds only progress.cursor from the durable cursor and never
// seeds progress.wrapped from cursor.wrapped. The durable wrapped flag is therefore write-only,
// so a target that has already traversed its table wraps to zero and re-examines it again on
// every subsequent cycle. users.inviter_uuid consequently spends the entire global budget every
// cycle, forever, and users.uuid never receives any: its historical rows are never filled,
// validation always reports actionable rows, and no marker is ever written.
//
// Measured on PostgreSQL 17 with 2,500 seeded users and the minimum row budget of 1,000: 81
// consecutive reconciliation cycles each reported examined=1000 updated=0 while
// users.uuid_compact remained NULL for all 2,500 rows. With the shipped 10,000 default the same
// starvation begins once users exceeds roughly 5,000 rows, which is every real upgrade and both
// of Section 12's own 100k and 1m fixtures.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func compactFaultPartialBackfillBarrier(t *testing.T) {
	db, topology, ctx := compactFaultFixture(t, compactFaultBacklogRows, config.MinCompactUUIDMaxRowsPerCycle)
	digest := compactFaultDigest(db, compactFaultBacklogRows)
	coordinator := newCompactCoordinator(topology)
	traffic := compactFaultStartTraffic(t, db, 4)

	// The guard must ask about users' OWN shadow: a global shadow count is already non-zero once
	// any other table has been expanded, and querying users.uuid_compact before users itself is
	// expanded fails with "column does not exist".
	usersExpanded := "SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public'" +
		" AND table_name = 'users' AND column_name = 'uuid_compact'"
	unfilled := "SELECT count(*) FROM users WHERE uuid_compact IS NULL"
	partial := false
	for cycle := 0; cycle < compactFaultMaxCycles && !partial; cycle++ {
		result, err := compactFaultRunCycle(ctx, coordinator)
		require.NoError(t, err, "cycle %d", cycle)
		require.NotEqual(t, compactStateBlockedValidation, result.state,
			"the fill must never block: %s", result.reason)
		require.NotEqual(t, compactStateReady, result.state,
			"the migration completed before the fill was ever observably partial")
		if compactFaultCount(t, db, usersExpanded) == 0 {
			continue // Still expanding; users has no shadow to count yet.
		}
		// Genuinely partial: the fill has started and provably has not finished.
		remaining := compactFaultCount(t, db, unfilled)
		partial = remaining > 0 && remaining < compactFaultBacklogRows
	}
	require.True(t, partial,
		"the historical fill of users.uuid never started: users.inviter_uuid sorts first and "+
			"spends the whole shared row budget on every cycle, so users.uuid is starved forever")

	before := traffic.ops.Load()
	time.Sleep(compactFaultHoldFor) // The hold.
	require.NoError(t, traffic.firstFailure(), "the workload must survive the partial-backfill barrier")
	require.Positive(t, compactFaultCount(t, db, unfilled),
		"the partial-backfill barrier must still have unfilled rows, or it is not partial")
	require.False(t, compactFaultMarkerIntegrity(t, ctx, topology),
		"an unfinished backfill must never carry a completion marker")
	require.Greater(t, traffic.ops.Load()-before, int64(1000),
		"the partial-backfill barrier must be held under at least 1,000 operations")

	require.Equal(t, compactStateReady, driveCompactToReady(t, coordinator).state)
	traffic.stopAndReconcile(t, db)
	require.Equal(t, digest, compactFaultDigest(db, compactFaultBacklogRows))
}

// compactFaultKillAroundSideEffects kills the owner around every durable side effect (AUTO-T11).
//
// Phase one kills before and after every column add, trigger install, index create, and repair
// batch with a fresh coordinator per cycle — a process that dies and restarts on every single
// side effect. It cannot reach completion by construction, and that is correct rather than a
// limitation: Section 8.5's two clean passes must come from ONE worker's epoch, so a
// coordinator that never survives two cycles must never mark, and asserting that it does not is
// the strongest available "no false marker under a crash loop" evidence.
//
// Phase two therefore keeps one coordinator alive to reach validation and marking, and kills it
// at a swept set of depths inside the completing cycle.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func compactFaultKillAroundSideEffects(t *testing.T) {
	db, topology, ctx := compactFaultFixture(t, compactFaultSeedRows, 0)
	digest := compactFaultDigest(db, compactFaultSeedRows)
	traffic := compactFaultStartTraffic(t, db, 2)

	sideEffects := 0
	for cycle := 0; cycle < compactFaultMaxCycles; cycle++ {
		// Kill immediately BEFORE the side effect, at a swept depth.
		compactFaultKill(newCompactCoordinator(topology),
			compactFaultCancelDelays[cycle%len(compactFaultCancelDelays)])
		require.False(t, compactFaultMarkerIntegrity(t, ctx, topology),
			"a killed cycle must never leave a completion marker")

		// Perform the side effect, then kill immediately AFTER it by discarding the coordinator.
		result, err := compactFaultRunCycle(ctx, newCompactCoordinator(topology))
		require.NoError(t, err, "restart cycle %d must resume automatically after a kill", cycle)
		require.NotEqual(t, compactStateBlockedValidation, result.state,
			"a kill must never block the migration: %s", result.reason)
		require.Equal(t, digest, compactFaultDigest(db, compactFaultSeedRows),
			"committed authoritative bytes must be stable across kill %d", cycle)

		if result.state == compactStateExpanding || result.state == compactStateIndexing ||
			(result.state == compactStateBackfilling && result.updated > 0) {
			sideEffects++
			continue
		}
		require.Equal(t, compactStateValidating, result.state,
			"a crash-looping owner must stall in validation, never mark")
		break
	}
	require.Greater(t, sideEffects, len(compactRegistry()),
		"phase one must have killed around every shadow, trigger, index, and batch side effect")
	require.False(t, compactFaultMarkerIntegrity(t, ctx, topology),
		"a coordinator killed on every cycle must never accumulate the two clean passes a marker needs")
	t.Logf("phase one killed before and after %d durable side effects", sideEffects)

	// Phase two: a surviving owner reaches validation; kill it inside the completing cycle.
	coordinator, swept := newCompactCoordinator(topology), 0
	for _, delay := range compactFaultCancelDelays {
		if compactFaultMarkerIntegrity(t, ctx, topology) {
			break
		}
		// One clean pass recorded: the very next cycle validates, fingerprints, and marks.
		compactFaultAdvanceTo(t, ctx, coordinator, compactStateValidating)
		compactFaultKill(coordinator, delay)
		swept++
		compactFaultMarkerIntegrity(t, ctx, topology)
		require.Equal(t, digest, compactFaultDigest(db, compactFaultSeedRows))
	}
	t.Logf("phase two swept %d kill depths through the validation and marker side effects", swept)

	// The restart after every kill reaches the exact final state, with no command.
	require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)
	marked := readMarkerTimestamp(t, topology.primary, compactPrimaryMigrationKey)
	for round := 0; round < 4; round++ {
		compactFaultKill(newCompactCoordinator(topology),
			compactFaultCancelDelays[round%len(compactFaultCancelDelays)])
		require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)
		require.Equal(t, marked, readMarkerTimestamp(t, topology.primary, compactPrimaryMigrationKey),
			"a marker timestamp must never move once written")
	}
	traffic.stopAndReconcile(t, db)
	require.Equal(t, digest, compactFaultDigest(db, compactFaultSeedRows))
}

// compactFaultInjectedFaults injects an outage, a lock wait, and cancellation (AUTO-T12).
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func compactFaultInjectedFaults(t *testing.T) {
	db, topology, ctx := compactFaultFixture(t, compactFaultSeedRows, 0)
	digest := compactFaultDigest(db, compactFaultSeedRows)

	t.Run("bounded retry", func(t *testing.T) {
		// The backoff is the retry bound the proposal fixes, so it is asserted directly rather
		// than inferred from how long some loop happened to take.
		for failures := 0; failures <= 8; failures++ {
			exponent := failures
			if exponent > 5 {
				exponent = 5
			}
			ceiling := compactRetryInterval() * time.Duration(1<<uint(exponent))
			for sample := 0; sample < 50; sample++ {
				delay := compactBackoffDelay(failures)
				require.GreaterOrEqual(t, delay, time.Duration(0))
				require.LessOrEqual(t, delay, ceiling, "backoff must stay inside its full-jitter ceiling")
			}
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		compactFaultKill(newCompactCoordinator(topology), 0)
		require.False(t, compactFaultMarkerIntegrity(t, ctx, topology),
			"a cancelled cycle must never write a marker")
		require.Equal(t, digest, compactFaultDigest(db, compactFaultSeedRows))
	})

	t.Run("lock wait", func(t *testing.T) {
		// A conflicting ACCESS EXCLUSIVE lock on the first table the coordinator would expand.
		//
		// Section 8.4 titles itself "One bounded cycle" and caps lock acquisition at five
		// seconds, and the DDL layer does install a session lock_timeout around every compact
		// DDL statement. This asserts the cycle as a whole honours that bound.
		//
		// The statement that blocks is not the DDL but the metadata read that precedes it:
		// compactTableExpanded issues an information_schema column query through the gorm
		// Migrator, which needs ACCESS SHARE on the relation. withCompactMetadataDeadline
		// (compact_uuid_schema.go) now bounds those reads with compactMetadataTimeout, so the
		// cycle returns an error within the cap instead of blocking for as long as the caller's
		// context allows. This assertion locks in that bound; the 40s context below is only a
		// safety net and must never be what stops the cycle.
		table := compactTablesForTopology(topology)[0].table

		// The fault is injected and removed inside this closure, so a failed assertion below
		// can never leave an ACCESS EXCLUSIVE lock held: a require that fires between the LOCK
		// and its ROLLBACK would take the whole database down for every later test.
		elapsed, cycleErr := func() (time.Duration, error) {
			pool, err := db.DB()
			require.NoError(t, err)
			blocker, err := pool.Conn(ctx)
			require.NoError(t, err)
			defer func() {
				_, _ = blocker.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
				_ = blocker.Close()
			}()
			for _, statement := range []string{"BEGIN", "LOCK TABLE " + table + " IN ACCESS EXCLUSIVE MODE"} {
				_, err = blocker.ExecContext(ctx, statement)
				require.NoError(t, err)
			}

			cycleCtx, cancelCycle := context.WithTimeout(ctx, 40*time.Second)
			defer cancelCycle()
			started := time.Now()
			_, cycleErr := compactFaultRunCycle(cycleCtx, newCompactCoordinator(topology))
			return time.Since(started), cycleErr
		}()

		require.Error(t, cycleErr, "a cycle blocked behind a conflicting lock must fail, not hang or skip")
		require.False(t, compactFaultMarkerIntegrity(t, ctx, topology))
		t.Logf("cycle blocked behind ACCESS EXCLUSIVE on %s returned in %s: %v", table, elapsed, cycleErr)

		// Fault removed: work resumes automatically, with no command. This asserts AUTO-T12's
		// recovery half after the bound below proves the cycle refuses to hang.
		require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)
		require.Equal(t, digest, compactFaultDigest(db, compactFaultSeedRows))

		require.Less(t, elapsed, 30*time.Second,
			"the cycle must bound its own wait on a conflicting relation lock; it instead blocked "+
				"until this test's own context expired, because the pre-DDL metadata read carries "+
				"no lock_timeout")
	})

	t.Run("database outage", func(t *testing.T) {
		dialect := compactFaultDialect()
		direct := strings.TrimSpace(os.Getenv(dialect.primaryEnv))

		// A clean fixture behind the relay, so the outage hits a migration in progress.
		dropLiveCompactSchema(t, db, dialect)
		require.NoError(t, migrateDB())
		compactFaultSeed(t, db, compactFaultSeedRows)
		requireV3Markers(t, topology)

		_, server, err := compactFaultRedirectDSN(direct, "")
		require.NoError(t, err)
		proxy := compactFaultStartProxy(t, server)
		relayDSN, _, err := compactFaultRedirectDSN(direct, proxy.listener.Addr().String())
		require.NoError(t, err)
		relayed, err := gorm.Open(postgres.Open(relayDSN), &gorm.Config{})
		require.NoError(t, err)
		t.Cleanup(func() {
			if relayPool, poolErr := relayed.DB(); poolErr == nil {
				_ = relayPool.Close()
			}
		})
		relayTopology, err := newUnifiedTopology(relayed)
		require.NoError(t, err)

		// A healthy bootstrap first, as AUTO-T12 requires.
		compactFaultAdvanceTo(t, ctx, newCompactCoordinator(relayTopology), compactStateExpanding)

		proxy.setOpen(false)
		outage := newCompactCoordinator(relayTopology)
		for attempt := 0; attempt < 3; attempt++ {
			_, cycleErr := compactFaultRunCycle(ctx, outage)
			require.Error(t, cycleErr, "a cycle against a severed database must fail, not claim progress")
		}
		require.False(t, compactFaultMarkerIntegrity(t, ctx, topology),
			"an outage must never produce a false marker")

		proxy.setOpen(true)
		// Automatic recovery on the SAME handle and pool, with no command.
		require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(relayTopology)).state)
		require.Equal(t, compactFaultDigest(db, compactFaultSeedRows), compactFaultDigest(relayed, compactFaultSeedRows),
			"the recovered handle must see identical authoritative text")
	})
}
