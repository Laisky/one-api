package model

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// This file is the behavioral reproduction and regression suite for the catch-up convergence
// defect diagnosed in production on 2026-09-10; see
// docs/autopsy/20260910_uuid-backfill-never-converges.md for the analysis.
//
// The production shape it reproduces: the logs table holds more rows whose token_uuid can never
// be resolved — their token was hard-deleted — than one catch-up cycle is allowed to examine.
// Every cycle then spent its whole row budget re-reading those rows from id 0, reported that
// work remained, and the worker never observed the quiescence automatic finalization waits for.
//
// Every test here states required behavior. The five that failed on the pre-fix tree are the
// reproduction; the rest were added with the fix. None of them may be weakened to make CI pass.

// convergenceOrphanTokenName is a token name that never exists in the tokens table, so every
// log row carrying it is a permanent orphan for the token_name phase.
const convergenceOrphanTokenName = "deleted-token"

// withCatchUpRowBudget sets the per-cycle examined-row ceiling for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - rows: examined-row ceiling for one catch-up cycle.
//
// Return values: none.
func withCatchUpRowBudget(t *testing.T, rows int) {
	t.Helper()
	original := config.ExternalUUIDBackfillMaxRowsPerCycle
	config.ExternalUUIDBackfillMaxRowsPerCycle = rows
	t.Cleanup(func() { config.ExternalUUIDBackfillMaxRowsPerCycle = original })
}

// seedOrphanTokenLogs inserts one legacy user, one legacy channel, and count legacy log rows
// whose token_name references a token that does not exist. The user and channel references are
// resolvable; the token reference is a permanent orphan.
// Parameters:
//   - t: test handle used for assertions.
//   - db: unified database handle.
//   - count: number of orphan log rows to insert.
//
// Return values: none.
func seedOrphanTokenLogs(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'primary', 'gpt-4o', '{}')").Error)

	const chunk = 200
	for start := 1; start <= count; start += chunk {
		end := start + chunk - 1
		if end > count {
			end = count
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			rows = append(rows, map[string]any{
				"id": id, "user_id": 1, "channel_id": 1, "type": 2,
				"token_name": convergenceOrphanTokenName, "content": "orphan " + strconv.Itoa(id),
			})
		}
		require.NoError(t, db.Table("logs").Create(rows).Error)
	}
}

// requireOrphanTokenUUIDsUntouched asserts that no orphan log row received an invented token
// UUID: convergence must come from scheduling, never from fabricating a reference.
// Parameters:
//   - t: test handle used for assertions.
//   - db: unified database handle.
//   - count: number of orphan rows seeded.
//
// Return values: none.
func requireOrphanTokenUUIDsUntouched(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	var stillMissing int64
	require.NoError(t, db.Table("logs").
		Where("token_name = ? AND token_uuid IS NULL", convergenceOrphanTokenName).
		Count(&stillMissing).Error)
	require.EqualValues(t, count, stillMissing, "orphan rows must keep a missing token_uuid")
}

// uuidRowsCapture records RecordUUIDBackfillRows calls so a test can measure how many rows each
// cycle examined per phase and target.
type uuidRowsCapture struct {
	*metrics.NoOpRecorder
	mu     sync.Mutex
	counts map[string]int
}

// installUUIDRowsCapture routes the process metrics recorder to a capture for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//
// Return values:
//   - *uuidRowsCapture: capture receiving every UUID backfill row metric.
func installUUIDRowsCapture(t *testing.T) *uuidRowsCapture {
	t.Helper()
	capture := &uuidRowsCapture{NoOpRecorder: &metrics.NoOpRecorder{}, counts: map[string]int{}}
	original := metrics.Recorder()
	metrics.SetRecorder(capture)
	t.Cleanup(func() { metrics.SetRecorder(original) })
	return capture
}

// RecordUUIDBackfillRows accumulates the count under phase, target, and result.
func (capture *uuidRowsCapture) RecordUUIDBackfillRows(role, phase, target, result string, count int) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.counts[phase+"|"+target+"|"+result] += count
}

// take returns and clears the accumulated count for one phase, target, and result.
// Parameters:
//   - phase: registry phase name.
//   - target: registry table.column identifier.
//   - result: row result label.
//
// Return values:
//   - int: rows accumulated since the last take.
func (capture *uuidRowsCapture) take(phase, target, result string) int {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	key := phase + "|" + target + "|" + result
	value := capture.counts[key]
	capture.counts[key] = 0
	return value
}

// sqlCapture records the text of every statement a handle executes.
type sqlCapture struct {
	mu         sync.Mutex
	statements []string
}

// installSQLCapture attaches callbacks that capture every statement a handle executes.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle to instrument.
//
// Return values:
//   - *sqlCapture: capture observing the handle.
func installSQLCapture(t *testing.T, db *gorm.DB) *sqlCapture {
	t.Helper()
	capture := &sqlCapture{}
	record := func(tx *gorm.DB) {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		capture.statements = append(capture.statements, tx.Statement.SQL.String())
	}
	for name, register := range map[string]func(string, func(*gorm.DB)) error{
		"query":  db.Callback().Query().After("gorm:query").Register,
		"row":    db.Callback().Row().After("gorm:row").Register,
		"raw":    db.Callback().Raw().After("gorm:raw").Register,
		"update": db.Callback().Update().After("gorm:update").Register,
		"create": db.Callback().Create().After("gorm:create").Register,
	} {
		require.NoError(t, register("uuidconvergence:capture_"+name, record))
	}
	return capture
}

// drain returns and clears the captured statements.
// Parameters: none.
//
// Return values:
//   - []string: statements captured since the last drain.
func (capture *sqlCapture) drain() []string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	statements := capture.statements
	capture.statements = nil
	return statements
}

// schemaProbes returns the captured statements that read schema or index metadata.
// Parameters:
//   - statements: statements to classify.
//
// Return values:
//   - []string: statements against sqlite_master, PRAGMA, or the catalog views.
func schemaProbes(statements []string) []string {
	probes := []string{}
	for _, statement := range statements {
		lower := strings.ToLower(statement)
		if strings.Contains(lower, "sqlite_master") || strings.Contains(lower, "pragma") ||
			strings.Contains(lower, "information_schema") || strings.Contains(lower, "pg_indexes") ||
			strings.Contains(lower, "pg_index") {
			probes = append(probes, statement)
		}
	}
	return probes
}

// TestCatchUpSettlesWhenOnlyOrphansRemain reproduces the production hang at the coordinator
// level. When the remaining candidate rows are all permanent orphans and there are more of them
// than one cycle may examine, bounded cycles must still reach a settled no-work pass
// (updated == 0 and no backlog) within a small number of cycles; the finalizer must then
// complete and tolerate the orphans.
//
// Pre-remediation behavior: every cycle restarts its cursor at id 0, examines the first
// budget-worth of orphans, reports budget_exhausted=true, and never settles.
func TestCatchUpSettlesWhenOnlyOrphansRemain(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)

	orphans := config.MinExternalUUIDBackfillMaxRowsPerCycle + config.MinExternalUUIDBackfillMaxRowsPerCycle/2
	seedOrphanTokenLogs(t, db, orphans)

	// Resolvable work: 1 user + 1 channel owned UUIDs, `orphans` owned log UUIDs, `orphans`
	// user_uuid rows, `orphans` channel_uuid rows. Unresolvable work: `orphans` token_uuid rows.
	// Even with one examined row per cycle beyond the budget this settles well inside 40 cycles.
	const maxCycles = 40
	settled := false
	var results []uuidMigrationResult
	for cycle := 1; cycle <= maxCycles; cycle++ {
		result := runCatchUp(t, topology)
		results = append(results, result)
		if result.updated == 0 && !result.budgetExhausted {
			settled = true
			break
		}
	}
	tail := results
	if len(tail) > 5 {
		tail = tail[len(tail)-5:]
	}
	require.True(t, settled,
		"catch-up must settle once only permanent orphans remain; last cycles: %+v", tail)

	requireOrphanTokenUUIDsUntouched(t, db, orphans)

	var missingUserUUID int64
	require.NoError(t, db.Table("logs").Where("user_uuid IS NULL").Count(&missingUserUUID).Error)
	require.Zero(t, missingUserUUID, "every resolvable reference must be filled before settling")

	_, err := runFinalizer(t, topology)
	require.NoError(t, err, "permanent orphans must not block finalization")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)
}

// TestCatchUpWorkerFinalizesWhenOrphanBacklogExceedsCycleBudget reproduces the production hang
// end to end through the real background worker: with automatic finalization enabled and more
// permanent orphans than one cycle may examine, the worker must still finalize and write the
// completion marker, after which startup is marker-only forever.
//
// Pre-remediation behavior: the worker reschedules at the active interval indefinitely, never
// counts an idle pass, never finalizes, and the same scan repeats every cycle.
func TestCatchUpWorkerFinalizesWhenOrphanBacklogExceedsCycleBudget(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withAutoFinalize(t, true, 2)
	withCatchUpIntervals(t, 5*time.Millisecond, 10*time.Millisecond)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)
	t.Cleanup(resetBootstrapStateForTest)

	// The worker and this goroutine share one in-memory SQLite database; a second connection
	// would open a separate empty database.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	orphans := config.MinExternalUUIDBackfillMaxRowsPerCycle + config.MinExternalUUIDBackfillMaxRowsPerCycle/2
	seedOrphanTokenLogs(t, db, orphans)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		stopUUIDCatchUpWorker()
	})
	require.NoError(t, startExternalUUIDMigration(ctx, topology))
	require.True(t, bootstrapTestCatchUpWorkerRunning())

	require.Eventually(t, func() bool {
		complete, err := isDataMigrationComplete(context.Background(), db, externalUUIDPrimaryMigrationKey)
		return err == nil && complete
	}, 20*time.Second, 20*time.Millisecond,
		"the worker must reach quiescence and finalize even though %d permanent orphans exceed the %d-row cycle budget",
		orphans, config.MinExternalUUIDBackfillMaxRowsPerCycle)

	requireOrphanTokenUUIDsUntouched(t, db, orphans)
}

// TestCatchUpExaminesEachOrphanOncePerPass pins the incremental contract: one traversal of the
// work queue examines each permanently unresolvable row exactly once, however many bounded
// cycles that traversal takes.
//
// A settled cycle (`updated == 0 && !budgetExhausted`) is the end of a traversal. With only
// orphans left, the rows examined between one settled cycle and the next are exactly one
// traversal's worth, so the count must equal the orphan count.
//
// Pre-remediation behavior: no cycle ever settles, because each one restarts at id 0, examines
// the first budget-worth of orphans, and reports backlog — so this test cannot even reach its
// measurement window.
func TestCatchUpExaminesEachOrphanOncePerPass(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)
	capture := installUUIDRowsCapture(t)

	orphans := config.MinExternalUUIDBackfillMaxRowsPerCycle + config.MinExternalUUIDBackfillMaxRowsPerCycle/2
	seedOrphanTokenLogs(t, db, orphans)

	const maxCycles = 40
	require.True(t, driveCatchUpToSettled(t, topology, maxCycles),
		"catch-up must reach a settled cycle within %d cycles", maxCycles)

	// From here the only candidates left are the orphans. Measure one full traversal: cycles
	// until the next settled cycle, inclusive.
	capture.take(uuidPhaseTokenName, "logs.token_uuid", uuidRowResultUnresolved)
	cycles := 0
	for cycle := 1; cycle <= maxCycles; cycle++ {
		result := runCatchUp(t, topology)
		cycles = cycle
		if result.updated == 0 && !result.budgetExhausted {
			break
		}
	}
	examined := capture.take(uuidPhaseTokenName, "logs.token_uuid", uuidRowResultUnresolved)

	require.Equal(t, orphans, examined,
		"one traversal must examine each of the %d orphans exactly once; it examined %d rows over %d cycles",
		orphans, examined, cycles)
	require.Greater(t, cycles, 1,
		"the fixture must need more than one bounded cycle per traversal, otherwise it proves nothing")
	requireOrphanTokenUUIDsUntouched(t, db, orphans)
}

// driveCatchUpToQuiescentPass runs bounded cycles until one whole traversal writes nothing.
//
// This is the condition automatic finalization waits for, and it is strictly stronger than a
// single settled cycle: a traversal that filled rows in an earlier cycle is not quiescence even
// when its final cycle writes nothing.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology under test.
//   - maxCycles: cycle ceiling before giving up.
//
// Return values:
//   - bool: true when a quiescent pass was observed.
func driveCatchUpToQuiescentPass(t *testing.T, topology *databaseTopology, maxCycles int) bool {
	t.Helper()
	for cycle := 1; cycle <= maxCycles; cycle++ {
		result := runCatchUp(t, topology)
		if result.passComplete && result.passUpdated == 0 {
			return true
		}
	}
	return false
}

// driveCatchUpToSettled runs bounded cycles until one settles.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology under test.
//   - maxCycles: cycle ceiling before giving up.
//
// Return values:
//   - bool: true when a settled cycle was observed.
func driveCatchUpToSettled(t *testing.T, topology *databaseTopology, maxCycles int) bool {
	t.Helper()
	for cycle := 1; cycle <= maxCycles; cycle++ {
		result := runCatchUp(t, topology)
		if result.updated == 0 && !result.budgetExhausted {
			return true
		}
	}
	return false
}

// TestCatchUpSteadyStateCycleIssuesNoSchemaProbes pins the low-cost steady state: after the
// first cycle has validated the schema and ensured the candidate indexes, a later cycle that
// finds no work must not re-read table, column, or index metadata.
//
// Pre-remediation behavior: every cycle re-runs schema validation, candidate-index assurance,
// and per-target HasTable/HasColumn checks, which is roughly 150 catalog statements per cycle on
// PostgreSQL.
func TestCatchUpSteadyStateCycleIssuesNoSchemaProbes(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	capture := installSQLCapture(t, db)

	// Cycle 1 may probe and create whatever it needs.
	runCatchUp(t, topology)
	capture.drain()

	// Cycle 2 is the first steady-state cycle; cycle 3 must be steady too.
	runCatchUp(t, topology)
	second := capture.drain()
	runCatchUp(t, topology)
	third := capture.drain()

	probes := schemaProbes(third)
	t.Logf("steady-state cycle statements: cycle2=%d (schema probes %d) cycle3=%d (schema probes %d)",
		len(second), len(schemaProbes(second)), len(third), len(probes))
	examples := probes
	if len(examples) > 3 {
		examples = examples[:3]
	}
	require.Zero(t, len(probes),
		"a steady-state catch-up cycle must not re-read schema or index metadata; saw %d probes, e.g. %q",
		len(probes), examples)
}

// TestCatchUpNewPassPicksUpLateResolvableRows guards the remediation against over-correcting:
// rows that were unresolvable in one pass must be re-examined by a later pass, because an owner
// can appear afterwards (for example a token re-created under the historical name). Remembering
// orphans forever would leave a fillable gap that then blocks finalization.
//
// This test passes on the pre-remediation tree and must keep passing.
func TestCatchUpNewPassPicksUpLateResolvableRows(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)

	// Fewer orphans than the budget so the pre-remediation tree can settle at all.
	orphans := config.MinExternalUUIDBackfillMaxRowsPerCycle / 4
	seedOrphanTokenLogs(t, db, orphans)

	const maxCycles = 40
	settled := false
	for cycle := 1; cycle <= maxCycles; cycle++ {
		result := runCatchUp(t, topology)
		if result.updated == 0 && !result.budgetExhausted {
			settled = true
			break
		}
	}
	require.True(t, settled, "catch-up must settle with %d orphans below the budget", orphans)
	requireOrphanTokenUUIDsUntouched(t, db, orphans)

	// The historical token comes back under the same (user_id, name): every orphan is now fillable.
	require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (1, 1, 'recreated-key', ?)", convergenceOrphanTokenName).Error)

	settled = false
	for cycle := 1; cycle <= maxCycles; cycle++ {
		result := runCatchUp(t, topology)
		if result.updated == 0 && !result.budgetExhausted {
			settled = true
			break
		}
	}
	require.True(t, settled, "catch-up must settle again after filling the late-resolvable rows")

	var stillMissing int64
	require.NoError(t, db.Table("logs").
		Where("token_name = ? AND token_uuid IS NULL", convergenceOrphanTokenName).
		Count(&stillMissing).Error)
	require.Zero(t, stillMissing, "a later pass must fill rows that became resolvable")

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)
}

// TestCompactWorkerWaitsForPrerequisiteAtIdleCadence reproduces the companion polling defect:
// while the external UUID v3 markers are absent the compact worker has nothing it may do, so it
// must wait at the idle cadence (or until signaled), not poll at the active cadence.
//
// Pre-remediation behavior: waiting_prerequisite falls through to the active interval, so the
// compact worker re-acquires ownership and re-reads both marker sets every five seconds for as
// long as the v3 catch-up has not finalized.
func TestCompactWorkerWaitsForPrerequisiteAtIdleCadence(t *testing.T) {
	_, topology := newUnifiedTestTopology(t)
	original := config.CompactUUIDAutoMigrate
	config.CompactUUIDAutoMigrate = true
	t.Cleanup(func() {
		config.CompactUUIDAutoMigrate = original
		resetCompactHealthForTest()
	})

	coordinator := newCompactCoordinator(topology)
	delay := runCompactWorkerCycle(compactTestContext(t), coordinator)

	require.Equal(t, compactIdleInterval(), delay,
		"waiting on the v3 prerequisite has no work to do and must not reschedule at the active interval (%s)",
		compactActiveInterval())
}

// withCatchUpCycleDuration sets the wall-clock ceiling of one catch-up cycle for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - window: cycle duration ceiling.
//
// Return values: none.
func withCatchUpCycleDuration(t *testing.T, window time.Duration) {
	t.Helper()
	original := config.ExternalUUIDBackfillMaxCycleDuration
	config.ExternalUUIDBackfillMaxCycleDuration = window
	t.Cleanup(func() { config.ExternalUUIDBackfillMaxCycleDuration = original })
}

// TestCatchUpSettlesWithOrphansInSplitLogDatabase is the split-topology form of the
// convergence contract. The authoritative log database holds the orphans, so the cursors have
// to be keyed by database role as well as by table, column, and predicate.
func TestCatchUpSettlesWithOrphansInSplitLogDatabase(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)

	require.NoError(t, primary.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, primary.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'primary', 'gpt-4o', '{}')").Error)

	orphans := config.MinExternalUUIDBackfillMaxRowsPerCycle + config.MinExternalUUIDBackfillMaxRowsPerCycle/2
	const chunk = 200
	for start := 1; start <= orphans; start += chunk {
		end := start + chunk - 1
		if end > orphans {
			end = orphans
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			rows = append(rows, map[string]any{
				"id": id, "user_id": 1, "channel_id": 1, "type": 2,
				"token_name": convergenceOrphanTokenName, "content": "split orphan " + strconv.Itoa(id),
			})
		}
		require.NoError(t, logDB.Table("logs").Create(rows).Error)
	}

	require.True(t, driveCatchUpToSettled(t, topology, 40),
		"split-topology catch-up must settle once only permanent orphans remain")

	var missingUserUUID int64
	require.NoError(t, logDB.Table("logs").Where("user_uuid IS NULL").Count(&missingUserUUID).Error)
	require.Zero(t, missingUserUUID, "every resolvable log reference must be filled before settling")

	requireOrphanTokenUUIDsUntouched(t, logDB, orphans)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)
	requireMarker(t, primary, externalUUIDPrimaryMigrationKey, true)
	requireMarker(t, logDB, externalUUIDLogMigrationKey, true)
}

// newFileBackedUnifiedTestTopology builds a unified topology over a temp-file SQLite database.
//
// Tests that deliberately let a context deadline interrupt a statement need this: a cancelled
// statement poisons its connection, database/sql drops it, and for ":memory:" the replacement
// connection is a brand new empty database.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//
// Return values:
//   - *gorm.DB: primary handle with the full schema migrated.
//   - *databaseTopology: unified topology.
func newFileBackedUnifiedTestTopology(t *testing.T) (*gorm.DB, *databaseTopology) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "convergence.db")), &gorm.Config{})
	require.NoError(t, err)
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection keeps SQLite's single-writer model out of the way of the property here.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	withTestDBGlobals(t, db, db)
	require.NoError(t, migrateDB())
	topology, err := newUnifiedTopology(db)
	require.NoError(t, err)
	return db, topology
}

// TestCatchUpTimeBudgetNeverSkipsResolvableRows guards the safety property of a cursor that
// survives across cycles: interrupting a cycle must never let the next one step over rows the
// interrupted cycle read but did not write.
//
// It is checked twice, because a cycle can be cut short two ways. The row budget path is
// deterministic. The wall-clock path is not — how much a cycle finishes in a few milliseconds
// depends on the machine, and under the race detector it may be nothing at all — so that half
// asserts only what is machine-independent: an interrupted cycle is not an error, and it leaves
// no row behind for a later cycle to fill.
func TestCatchUpTimeBudgetNeverSkipsResolvableRows(t *testing.T) {
	// A file-backed database, not the suite's usual ":memory:" one. Interrupting a statement
	// on a cancelled context makes database/sql discard that connection, and a replacement
	// connection to ":memory:" opens a different, empty database — the fixture would vanish
	// mid-test for a reason that has nothing to do with the behavior under test.
	db, topology := newFileBackedUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)

	// More rows than one cycle's budget, so a traversal needs several cycles and the cursor is
	// what carries it between them.
	const legacyUsers = 2500
	seedLegacyUsers(t, db, legacyUsers)

	// A wall-clock ceiling short enough to interrupt cycles, but which is allowed to make no
	// progress at all on a slow machine.
	withCatchUpCycleDuration(t, 15*time.Millisecond)
	for cycle := 1; cycle <= 20; cycle++ {
		// runCatchUp fails the test on any error, which is the assertion that matters here:
		// a cycle ended by its own deadline is a normal cycle boundary, including when the
		// deadline lands inside a batch's transaction and the driver reports the rollback
		// instead of the deadline.
		runCatchUp(t, topology)
	}

	// Now let cycles run to completion. Whatever the interrupted cycles did or did not do, the
	// traversal must still reach every row.
	withCatchUpCycleDuration(t, 30*time.Second)
	require.True(t, driveCatchUpToQuiescentPass(t, topology, 100),
		"catch-up must reach a quiescent pass after being interrupted repeatedly")

	var filled int64
	require.NoError(t, db.Table("users").Where("uuid IS NOT NULL AND uuid != ''").Count(&filled).Error)
	require.EqualValues(t, legacyUsers, filled,
		"an interrupted cycle must not skip rows it read but never wrote")

	var distinct int64
	require.NoError(t, db.Raw("SELECT COUNT(DISTINCT uuid) FROM users WHERE uuid IS NOT NULL AND uuid != ''").Scan(&distinct).Error)
	require.EqualValues(t, legacyUsers, distinct, "no duplicate uuid assignment across resumed cycles")

	_, err := runFinalizer(t, topology)
	require.NoError(t, err, "a repeatedly interrupted catch-up must still finalize cleanly")
}

// TestCatchUpFailedCycleRestartsThePass pins the failure contract: a cycle that errors may have
// left a cursor mid-scan, so the pass it belonged to is abandoned and the next cycle traverses
// from the beginning. Without that, an error could hide rows for the rest of the pass and let
// the pass report quiescence it never verified.
func TestCatchUpFailedCycleRestartsThePass(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)
	seedOrphanTokenLogs(t, db, 40)

	require.True(t, driveCatchUpToSettled(t, topology, 40), "catch-up must settle first")

	progress := topology.catchUpProgress()
	require.True(t, progress.schemaIsValidated())
	require.True(t, progress.indexesAreEnsured())

	// A cycle failed: the pass and every shape memo are discarded.
	progress.failPass()
	require.False(t, progress.schemaIsValidated(),
		"a failure must let the next cycle re-check the schema it may have been about")
	require.False(t, progress.indexesAreEnsured())

	// The next cycle starts a brand new pass rather than resuming finished cursors.
	before := progress.pass
	pass := progress.beginCycle()
	require.Equal(t, before+1, pass, "a failed pass must not be resumed")
	key := uuidCursorKey{
		role: uuidRolePrimary, phase: uuidPhaseTokenName, table: "logs",
		column: "token_uuid", predicate: `"token_uuid" IS NULL`,
	}
	cursor, done := progress.scanStart(key)
	require.Zero(t, cursor, "a new pass starts every scan at the beginning")
	require.False(t, done, "a new pass has no exhausted scans")
}

// TestCatchUpWorkerRestartStartsAFreshPass pins that pass state is per database installation:
// a restart pays for one full traversal rather than inheriting cursors it cannot vouch for.
func TestCatchUpWorkerRestartStartsAFreshPass(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)
	seedOrphanTokenLogs(t, db, 40)

	require.True(t, driveCatchUpToSettled(t, topology, 40), "catch-up must settle first")

	restarted, err := newUnifiedTopology(db)
	require.NoError(t, err)
	progress := restarted.catchUpProgress()
	require.Zero(t, progress.pass, "a new topology carries no pass state")
	require.False(t, progress.schemaIsValidated(), "a new topology re-validates the schema once")

	capture := installUUIDRowsCapture(t)
	runCatchUp(t, restarted)
	examined := capture.take(uuidPhaseTokenName, "logs.token_uuid", uuidRowResultUnresolved)
	require.Equal(t, 40, examined,
		"a restarted worker must traverse the orphans again rather than trust a lost cursor")
}

// TestCatchUpBacklogGaugeClearsOnAQuiescentPass pins the operator-visible signal. The gauge is
// the documented way to decide that catch-up has drained, so it must report backlog while a
// traversal is unfinished and clear once a whole traversal writes nothing.
func TestCatchUpBacklogGaugeClearsOnAQuiescentPass(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)
	capture := installBacklogCapture(t)

	orphans := config.MinExternalUUIDBackfillMaxRowsPerCycle + config.MinExternalUUIDBackfillMaxRowsPerCycle/2
	seedOrphanTokenLogs(t, db, orphans)

	runCatchUp(t, topology)
	require.EqualValues(t, 1, capture.last(),
		"an unfinished traversal must report backlog")

	require.True(t, driveCatchUpToQuiescentPass(t, topology, 40), "catch-up must reach a quiescent pass")
	require.EqualValues(t, 0, capture.last(),
		"a traversal that examined only permanently unresolvable rows is quiescence, not backlog")
}

// backlogCapture records the last UUID backfill backlog gauge value.
type backlogCapture struct {
	*metrics.NoOpRecorder
	mu    sync.Mutex
	value float64
}

// installBacklogCapture routes the process metrics recorder to a backlog capture for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//
// Return values:
//   - *backlogCapture: capture receiving every backlog gauge update.
func installBacklogCapture(t *testing.T) *backlogCapture {
	t.Helper()
	capture := &backlogCapture{NoOpRecorder: &metrics.NoOpRecorder{}}
	original := metrics.Recorder()
	metrics.SetRecorder(capture)
	t.Cleanup(func() { metrics.SetRecorder(original) })
	return capture
}

// UpdateUUIDBackfillBacklog records the most recent gauge value.
func (capture *backlogCapture) UpdateUUIDBackfillBacklog(role, target string, backlog float64) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.value = backlog
}

// last returns the most recently recorded gauge value.
// Parameters: none.
//
// Return values:
//   - float64: last observed backlog value.
func (capture *backlogCapture) last() float64 {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.value
}

// TestCompactWorkerWakesWhenPrerequisiteIsSignaled pins the other half of the compact cadence
// change: parking at the idle cadence is only acceptable because completing the external UUID
// migration wakes the worker immediately.
func TestCompactWorkerWakesWhenPrerequisiteIsSignaled(t *testing.T) {
	// Drain any signal a previous test left behind so this test observes only its own.
	select {
	case <-compactPrerequisiteSignal:
	default:
	}

	woken := make(chan struct{})
	go func() {
		select {
		case <-compactPrerequisiteSignal:
			close(woken)
		case <-time.After(5 * time.Second):
		}
	}()

	signalCompactPrerequisite()

	select {
	case <-woken:
	case <-time.After(time.Second):
		t.Fatal("a worker parked on the v3 prerequisite must be woken within one second of completion")
	}
}

// TestCatchUpPassCompletionDistinguishesBudgetFromQuiescence pins the distinction the whole
// scheduler rests on. A cycle stopped by its row budget has an incomplete traversal and real
// backlog; the cycle that then reaches the end of every scan has a complete traversal, and it
// is quiescent even though every row it examined was permanently unresolvable and it therefore
// wrote nothing.
//
// Conflating those two states is the original defect: reporting "budget spent" as "work
// remains" is what stopped the quiescence streak from ever advancing.
func TestCatchUpPassCompletionDistinguishesBudgetFromQuiescence(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withCatchUpRowBudget(t, config.MinExternalUUIDBackfillMaxRowsPerCycle)

	// More orphans than one cycle may examine, so a traversal of them needs two cycles.
	orphans := config.MinExternalUUIDBackfillMaxRowsPerCycle + config.MinExternalUUIDBackfillMaxRowsPerCycle/2
	seedOrphanTokenLogs(t, db, orphans)
	require.True(t, driveCatchUpToQuiescentPass(t, topology, 40), "catch-up must reach a quiescent pass")

	// Only orphans remain. The first cycle of the next traversal runs out of row budget.
	first := runCatchUp(t, topology)
	require.False(t, first.passComplete,
		"a cycle stopped by its row budget has not finished the traversal")
	require.True(t, first.budgetExhausted,
		"an unfinished traversal is backlog and must be rescheduled promptly")

	// The second cycle reaches the end of every scan. It wrote nothing, and nothing is left
	// to examine, so this is quiescence and the streak toward finalization may advance.
	second := runCatchUp(t, topology)
	require.True(t, second.passComplete,
		"reaching the end of every scan completes the traversal")
	require.Zero(t, second.passUpdated,
		"a traversal over permanently unresolvable rows writes nothing")
	require.False(t, second.budgetExhausted,
		"a completed traversal is not backlog, however many rows it had to examine")
}
