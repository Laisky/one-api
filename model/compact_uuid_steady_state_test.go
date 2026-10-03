package model

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/idresolve"
)

// This file is the behavioral reproduction and regression suite for the compact UUID worker's
// post-completion cost, diagnosed on b1 on 2026-09-10 once the external UUID backfill fix had
// shipped and the compact migration had completed.
//
// Section 8.6 of the compact proposal specifies the completed worker's steady state: verify
// object metadata, probe the actionable NULL backlog through the compact index, and advance a
// BOUNDED rolling equality scan. What production did instead, every idle interval, forever:
//
//   - a full validation traversal of every row of all 27 targets, the completion gate, rerun
//     although the markers already existed; and
//   - a gap probe that PostgreSQL served by walking the primary key over the whole table,
//     because `ORDER BY id LIMIT n` made the planner prefer id order over the compact index.
//
// Together that was ~1.5 million buffer accesses every five minutes on a 2 GB host, enough to
// sweep the entire shared buffer pool each time.
//
// Every test here states required behavior. Those that failed on the pre-fix tree are the
// reproduction; none of them may be weakened to make CI pass.

// compactStatement is one captured statement with its bound arguments.
type compactStatement struct {
	sql  string
	vars []any
}

// compactStatementCapture records every statement a handle executes, with its arguments.
type compactStatementCapture struct {
	mu         sync.Mutex
	statements []compactStatement
}

// installCompactStatementCapture attaches callbacks that capture statements and arguments.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle to instrument.
//
// Return values:
//   - *compactStatementCapture: capture observing the handle.
func installCompactStatementCapture(t *testing.T, db *gorm.DB) *compactStatementCapture {
	t.Helper()
	capture := &compactStatementCapture{}
	record := func(tx *gorm.DB) {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		vars := append([]any(nil), tx.Statement.Vars...)
		capture.statements = append(capture.statements, compactStatement{sql: tx.Statement.SQL.String(), vars: vars})
	}
	for name, register := range map[string]func(string, func(*gorm.DB)) error{
		"query": db.Callback().Query().After("gorm:query").Register,
		"row":   db.Callback().Row().After("gorm:row").Register,
		"raw":   db.Callback().Raw().After("gorm:raw").Register,
	} {
		require.NoError(t, register("compactsteady:capture_"+name, record))
	}
	return capture
}

// drain returns and clears the captured statements.
// Parameters: none.
//
// Return values:
//   - []compactStatement: statements captured since the last drain.
func (capture *compactStatementCapture) drain() []compactStatement {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	statements := capture.statements
	capture.statements = nil
	return statements
}

// isCompactValidationTraversal reports whether a statement is one page of the full validation
// traversal, which is the only compact read bounded above by a high-water mark.
// Parameters:
//   - sql: captured statement text.
//
// Return values:
//   - bool: true for a validation-traversal page.
func isCompactValidationTraversal(sql string) bool {
	return strings.Contains(sql, `"id" <= ?`) || strings.Contains(sql, "`id` <= ?")
}

// isCompactGapProbe reports whether a statement is the steady-state NULL-backlog existence probe.
// It excludes reconciliation feeds, which select full candidate projections for repair.
// Parameters:
//   - sql: captured statement text.
//
// Return values:
//   - bool: true for a gap probe.
func isCompactGapProbe(sql string) bool {
	hasNullPredicate := strings.Contains(sql, `_compact" IS NULL`) || strings.Contains(sql, "_compact` IS NULL")
	return hasNullPredicate && strings.Contains(sql, "ORDER BY") && !strings.Contains(sql, "AS compact_value")
}

// countCompactTraversals counts validation-traversal pages among captured statements.
// Parameters:
//   - statements: captured statements.
//
// Return values:
//   - int: number of traversal pages.
func countCompactTraversals(statements []compactStatement) int {
	count := 0
	for _, statement := range statements {
		if isCompactValidationTraversal(statement.sql) {
			count++
		}
	}
	return count
}

// seedCompactUsers inserts count users with valid legacy UUIDs through ordinary SQL.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle to insert into.
//   - count: number of users.
//
// Return values: none.
func seedCompactUsers(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	for id := 1; id <= count; id++ {
		seedCompactUser(t, db, id, compactUUIDTextFor(id))
	}
}

// TestCompactReadySteadyStateCycleIsIndependentOfTableSize reproduces the dominant production
// cost. Once the compact migration has completed, a routine worker cycle must do bounded work:
// its statement count may not grow with the number of rows, and it must never page through a
// target with the full validation traversal.
//
// Pre-fix behavior: every ready cycle reran the completion-gate traversal over every row of
// every target, so its cost grew linearly with table size.
func TestCompactReadySteadyStateCycleIsIndependentOfTableSize(t *testing.T) {
	statementsBySize := map[int]int{}
	for _, users := range []int{300, 1500} {
		users := users
		t.Run("", func(t *testing.T) {
			db, topology := newCompactTestTopology(t)
			seedCompactUsers(t, db, users)

			coordinator := newCompactCoordinator(topology)
			driveCompactToReady(t, coordinator)
			requireCompactMarkersPresent(t, topology)

			capture := installCompactStatementCapture(t, db)
			result := runCompactCycleForTest(t, coordinator)
			statements := capture.drain()

			require.Equal(t, compactStateReady, result.state, "a completed, clean installation stays ready")
			require.Zero(t, countCompactTraversals(statements),
				"a ready steady-state cycle must not rerun the full validation traversal (%d users)", users)
			statementsBySize[users] = len(statements)
		})
	}
	require.Equal(t, statementsBySize[300], statementsBySize[1500],
		"a ready steady-state cycle must cost the same however many rows exist: %v", statementsBySize)
}

// TestCompactRestartWithMarkersReachesReadyWithoutFullTraversal pins the restart case. A
// process that starts against an installation whose markers exist, whose objects verify, and
// whose data has no gaps is ready at once — exactly as every non-master process already is,
// on the strength of its object audit alone. It must not pay two full traversals first.
//
// Pre-fix behavior: a fresh coordinator reported `validating` and reran the traversal twice,
// on every deploy.
func TestCompactRestartWithMarkersReachesReadyWithoutFullTraversal(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 600)
	driveCompactToReady(t, newCompactCoordinator(topology))
	requireCompactMarkersPresent(t, topology)

	capture := installCompactStatementCapture(t, db)
	restarted := newCompactCoordinator(topology)
	result := runCompactCycleForTest(t, restarted)
	statements := capture.drain()

	require.Equal(t, compactStateReady, result.state,
		"a restarted worker over a completed, verified installation is ready in its first cycle")
	require.True(t, result.completed)
	require.Zero(t, countCompactTraversals(statements),
		"a restart over a completed installation must not rerun the full validation traversal")
}

// TestCompactSteadyStateStillDetectsAndRepairsDrift guards the fast path against hiding the
// drift the post-completion audit exists to catch. A dropped trigger is object drift; a shadow
// written wrong while it was gone is data drift. Both must still be detected, repaired, and
// followed by a fresh full audit before the worker reports ready again (section 8.6).
func TestCompactSteadyStateStillDetectsAndRepairsDrift(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 50)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	require.Equal(t, compactStateReady, runCompactCycleForTest(t, coordinator).state)

	// Drift: the trigger goes away and a row's shadow is blanked behind its back. The trigger has
	// to go first, because it re-derives the shadow from the row's text on every update.
	dropCompactSyncTriggers(t, db, "users")
	require.NoError(t, db.Exec(`UPDATE users SET uuid_compact = NULL WHERE id = 7`).Error)

	detected := runCompactCycleForTest(t, coordinator)
	require.NotEqual(t, compactStateReady, detected.state,
		"a dropped trigger must take the worker out of ready on the very next cycle")

	capture := installCompactStatementCapture(t, db)
	result := driveCompactToReady(t, coordinator)
	require.Equal(t, compactStateReady, result.state, "drift must be repaired automatically")
	require.Positive(t, countCompactTraversals(capture.drain()),
		"returning to ready after drift requires a fresh full audit")

	var mismatched int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM users WHERE uuid_compact IS NULL AND uuid IS NOT NULL AND uuid <> ''`).
		Scan(&mismatched).Error)
	require.Zero(t, mismatched, "the blanked shadow must be repaired")

	// And once repaired and re-audited, the worker is back in the bounded steady state.
	capture.drain()
	require.Equal(t, compactStateReady, runCompactCycleForTest(t, coordinator).state)
	require.Zero(t, countCompactTraversals(capture.drain()),
		"after a completed re-audit the worker returns to the bounded steady state")
}

// requireCompactMarkersPresent asserts that every applicable compact completion marker exists.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology under test.
//
// Return values: none.
func requireCompactMarkersPresent(t *testing.T, topology *databaseTopology) {
	t.Helper()
	markers, err := readCompactMarkerState(compactTestContext(t), topology)
	require.NoError(t, err)
	require.True(t, markers.allPresent(), "the compact migration must have completed")
}

// TestCompactSteadyStateHonorsExternalUUIDRollback pins the one reason a completed compact
// installation leaves ready without any drift of its own. Rolling back the external UUID
// migration deletes its markers; the compact worker has always answered that by reporting
// waiting_prerequisite, which also disables compact reads, and the bounded steady state must not
// paper over it.
func TestCompactSteadyStateHonorsExternalUUIDRollback(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 20)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	require.Equal(t, compactStateReady, runCompactCycleForTest(t, coordinator).state)

	require.NoError(t, db.Exec("DELETE FROM data_migrations WHERE migration_key = ?",
		externalUUIDPrimaryMigrationKey).Error)

	result := runCompactCycleForTest(t, coordinator)
	require.Equal(t, compactStateWaitingPrerequisite, result.state,
		"deleting the external UUID markers must take the compact worker out of ready")
}

// isCompactRowScan reports whether a statement reads target rows the way a scan does: the
// validation traversal, the rolling sweep, and the id-ordered repair feed all select the legacy
// value beside its shadow. The steady state's owned probe selects only an id, and its by-id repair
// read is recognized separately.
// Parameters:
//   - sql: captured statement text.
//
// Return values:
//   - bool: true for a row-scanning read.
func isCompactRowScan(sql string) bool {
	if !strings.Contains(sql, "AS compact_value") {
		return false
	}
	return !strings.HasSuffix(strings.TrimSpace(sql), `"id" = ?`) && !strings.HasSuffix(strings.TrimSpace(sql), "`id` = ?")
}

// compactProbeTargetsOwnedColumn reports whether a gap probe targets an owned shadow column.
// Parameters:
//   - sql: captured gap-probe text.
//
// Return values:
//   - bool: true when the probe's shadow column is an owned `uuid_compact`.
func compactProbeTargetsOwnedColumn(sql string) bool {
	return strings.Contains(sql, `"uuid_compact" IS NULL`) || strings.Contains(sql, "`uuid_compact` IS NULL")
}

// TestCompactSteadyStateUsesOnlyBoundedRollingScans pins the completed worker's cost contract.
// It may advance one bounded equality page per target, but it must not rerun full validation or
// use the expensive NULL-backlog probe for nullable foreign-key targets.
func TestCompactSteadyStateUsesOnlyBoundedRollingScans(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 400)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)

	capture := installCompactStatementCapture(t, db)
	for cycle := 0; cycle < 3; cycle++ {
		require.Equal(t, compactStateReady, runCompactCycleForTest(t, coordinator).state)
	}
	rowScans := 0
	for _, statement := range capture.drain() {
		require.False(t, isCompactValidationTraversal(statement.sql),
			"a steady-state cycle ran the validation traversal: %s", statement.sql)
		if isCompactRowScan(statement.sql) {
			rowScans++
			require.NotEmpty(t, statement.vars, "a rolling scan must carry bounded query parameters")
			limit, ok := statement.vars[len(statement.vars)-1].(int)
			require.True(t, ok, "a rolling scan limit must be an integer: %v", statement.vars)
			require.LessOrEqual(t, limit, compactMaxMaterializedRows,
				"a rolling scan exceeded the materialization cap: %s", statement.sql)
		}
		if isCompactGapProbe(statement.sql) {
			require.True(t, compactProbeTargetsOwnedColumn(statement.sql),
				"a steady-state cycle probed a foreign-key shadow: %s", statement.sql)
		}
	}
	require.Positive(t, rowScans, "steady state must retain bounded eventual drift detection")
}

// TestCompactSteadyStateDetectsTriggerBypassedRows covers the one drift the catalog cannot see.
// `pg_dump --data-only --disable-triggers` is a supported restore: it disables the triggers,
// loads rows, and enables them again, so afterwards every object verifies while the loaded rows
// have NULL shadows. Their owned shadow is NULL too, so the owned probe finds them, and the
// worker recovers through a full audit.
func TestCompactSteadyStateDetectsTriggerBypassedRows(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 30)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	ctx := compactTestContext(t)

	// The restore shape: triggers off, rows in, triggers back exactly as they were.
	dropCompactSyncTriggers(t, db, "users")
	for id := 31; id <= 40; id++ {
		seedCompactUser(t, db, id, compactUUIDTextFor(id))
	}
	users := compactTableForTest(t, topology, "users")
	require.NoError(t, installCompactTriggers(ctx, db, users))
	verified, reason, err := validateCompactObjects(ctx, topology)
	require.NoError(t, err)
	require.True(t, verified, "the fixture must leave a catalog that verifies: %s", reason)

	detected := runCompactCycleForTest(t, coordinator)
	require.NotEqual(t, compactStateReady, detected.state,
		"rows that bypassed the trigger must take the worker out of ready")

	require.Equal(t, compactStateReady, driveCompactToReady(t, coordinator).state)
	var gaps int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM users WHERE uuid_compact IS NULL AND uuid IS NOT NULL AND uuid <> ''`).
		Scan(&gaps).Error)
	require.Zero(t, gaps, "every row that bypassed the trigger must be repaired")
}

// TestCompactLookupMismatchRepairsExactlyThoseRows pins read-path-driven repair. A wrong non-NULL
// shadow over a verified catalog is invisible to every probe; only a lookup can find it, and when
// it does it knows exactly which rows disagree. The worker repairs those rows through their
// primary keys, in the next cycle, without scanning anything.
func TestCompactLookupMismatchRepairsExactlyThoseRows(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 5)
	coordinator := newCompactCoordinator(topology)
	driveCompactToReady(t, coordinator)
	ctx := compactTestContext(t)

	// Row 2's shadow claims row 1's identifier and row 1's shadow is gone; then the triggers are
	// restored, so the catalog verifies and no probe can see the damage.
	dropCompactSyncTriggers(t, db, "users")
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)
	wrong, err := parseCompactUUID(compactUUIDTextFor(1))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
		compactBindValue(dialectName(db), wrong)).Error)
	require.NoError(t, installCompactTriggers(ctx, db, compactTableForTest(t, topology, "users")))

	enableCompactReadsForTest(t, uuidRolePrimary)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)
	id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
	require.NoError(t, err)
	require.Equal(t, int64(1), id, "the read path must answer from authoritative text")

	capture := installCompactStatementCapture(t, db)
	result := runCompactCycleForTest(t, coordinator)
	require.Equal(t, compactStateDegraded, result.state)
	require.Equal(t, 2, result.updated, "both rows the lookup named must be repaired")
	for _, statement := range capture.drain() {
		require.False(t, isCompactValidationTraversal(statement.sql) || isCompactRowScan(statement.sql),
			"read-path repair must not scan: %s", statement.sql)
	}
	require.Equal(t, compactStateReady, driveCompactToReady(t, coordinator).state,
		"targeted repair must be followed by the required full audit")

	for _, row := range []int{1, 2} {
		var matches int64
		derived, err := parseCompactUUID(compactUUIDTextFor(row))
		require.NoError(t, err)
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM users WHERE id = ? AND uuid_compact = ?",
			row, compactBindValue(dialectName(db), derived)).Scan(&matches).Error)
		require.EqualValues(t, 1, matches, "row %d's shadow must match its text again", row)
	}
	id, err = resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
	require.NoError(t, err)
	require.Equal(t, int64(1), id)
	require.Empty(t, drainCompactRowRepairs(), "a lookup served by the compact index must queue nothing")
}

// TestCompactUnknownIdentifierLookupsCostNothing guards the request path. Before this change every
// compact miss woke the worker into a full traversal, so a stream of requests for identifiers that
// exist nowhere kept the database scanning. A miss the text index cannot resolve either is not
// evidence of anything, and must queue no repair and wake nobody.
func TestCompactUnknownIdentifierLookupsCostNothing(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	seedCompactUsers(t, db, 3)
	driveCompactToReady(t, newCompactCoordinator(topology))
	ctx := compactTestContext(t)
	enableCompactReadsForTest(t, uuidRolePrimary)

	drainCompactRowRepairs()
	select {
	case <-compactRepairSignal:
	default:
	}

	target, err := compactLookupTarget("users")
	require.NoError(t, err)
	for index := 100000; index < 100050; index++ {
		_, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(index))
		require.ErrorIs(t, err, idresolve.ErrNotFound)
	}
	require.Empty(t, drainCompactRowRepairs(), "unknown identifiers must queue no repair")
	select {
	case <-compactRepairSignal:
		t.Fatal("unknown identifiers must not wake the worker")
	default:
	}
}

// compactTableForTest returns one registry table by name.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology whose tables are searched.
//   - name: table name.
//
// Return values:
//   - compactTable: the registry entry.
func compactTableForTest(t *testing.T, topology *databaseTopology, name string) compactTable {
	t.Helper()
	for _, table := range compactTablesForTopology(topology) {
		if table.table == name {
			return table
		}
	}
	t.Fatalf("compact registry has no table %q", name)
	return compactTable{}
}
