package model

// Shared fixtures for the compact UUID storage suite (AUTO-014).
//
// The harness deliberately drives the REAL coordinator against a REAL engine rather than
// emulating it with raw SQL. The proposal is explicit that raw SQL emulation is not an
// acceptable substitute for the compatibility contract, and the same reasoning applies to the
// migration itself: the parts most likely to break — trigger bodies, catalog verification,
// recursion termination, driver scan types — are exactly the parts an emulation would fake.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// compactTestContext returns a context carrying the configured logger and a test deadline.
// Parameters:
//   - t: test handle used for cleanup registration.
//
// Return values:
//   - context.Context: seeded, bounded context.
func compactTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(withCompactLogger(context.Background()), compactTestBudget(t))
	t.Cleanup(cancel)
	return ctx
}

// compactTestBudget sizes the wall-clock budget for one compact-migration test.
//
// A fixed two minutes was too tight under load. TestCompactUUIDConcurrentCyclesConverge
// runs 4 x 60 real migration cycles and then drives the coordinator to ready; on a
// busy machine it exceeded the budget, and because an exhausted context stops the
// migration from progressing without surfacing an error, the test reported
// "table async_task_bindings is not fully expanded with typed compact columns" —
// a convergence failure that had not happened.
//
// Scaling off the binary's own deadline means a longer `go test -timeout` buys the
// work proportionally more room, while still expiring before go test kills the
// binary with an undiagnosable panic.
//
// Parameters:
//   - t: the running test, consulted for the binary's deadline.
//
// Return values:
//   - time.Duration: the budget to give this test's context.
func compactTestBudget(t *testing.T) time.Duration {
	const defaultBudget = 2 * time.Minute
	const maxBudget = 8 * time.Minute
	// Expire before go test's own timeout so the failure is ours and readable.
	const margin = 15 * time.Second

	deadline, ok := t.Deadline()
	if !ok {
		return defaultBudget
	}
	budget := time.Until(deadline) - margin
	switch {
	case budget <= 0:
		return time.Second
	case budget > maxBudget:
		return maxBudget
	default:
		return budget
	}
}

// requireCompactBudgetRemaining fails with an accurate message when the harness
// ran out of wall-clock time.
//
// An expired context makes the migration stop making progress, but the validation
// queries still run and report incomplete work — so without this check a timeout
// masquerades as a correctness failure and sends the reader hunting a bug that is
// not there.
//
// Parameters:
//   - t: the running test.
//   - ctx: the harness context whose budget is being checked.
func requireCompactBudgetRemaining(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, ctx.Err(),
		"compact test budget exhausted before the assertion ran; this is a harness timeout, "+
			"not a migration defect - raise go test -timeout or compactTestBudget")
}

// compactSuiteBudget is the wall-clock the compact-UUID suite needs to finish.
//
// Measured under -race on a developer machine: 51 compact tests take ~449s, i.e.
// 88% of the whole model package's 510s. The suite mutates global
// config.CompactUUID* settings, so its tests cannot use t.Parallel() and the cost
// is serial by construction.
const compactSuiteBudget = 15 * time.Minute

// requireCompactSuiteBudget keeps the compact-UUID suite from blowing the test
// binary's timeout.
//
// go test's default per-package timeout is 10 minutes, which this suite exceeds
// under -race — so a plain `go test -race ./...` died with
// `panic: test timed out after 10m0s` and took the whole model package down with
// it, reporting nothing useful about the 323 other tests in the package.
//
// Rather than fail opaquely, skip with instructions when the binary was not given
// enough time. Both CI workflows (-timeout 20m and 45m) and `make test-race`
// (-timeout 20m) clear the bar and run the suite in full. Set
// ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1 to turn the skip into a failure, so an
// environment that is supposed to run it can never silently stop.
//
// Parameters:
//   - t: the running test.
func requireCompactSuiteBudget(t *testing.T) {
	t.Helper()
	configured := configuredTestTimeout()
	// A zero timeout means `-timeout 0`, i.e. no limit at all.
	if configured <= 0 || configured >= compactSuiteBudget {
		return
	}
	message := fmt.Sprintf(
		"compact-UUID suite needs about %s of wall clock but the test binary was given -timeout %s; "+
			"re-run with `make test-race` or `go test -race -timeout 20m ./model/`",
		compactSuiteBudget, configured)
	if os.Getenv("ONEAPI_REQUIRE_COMPACT_UUID_SUITE") == "1" {
		t.Fatalf("%s (ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1 forbids skipping)", message)
	}
	t.Skip(message)
}

// configuredTestTimeout reports the -timeout the test binary was started with.
//
// Return values:
//   - time.Duration: the configured timeout, or 0 when unset/unlimited/unreadable.
func configuredTestTimeout() time.Duration {
	flagValue := flag.Lookup("test.timeout")
	if flagValue == nil {
		return 0
	}
	getter, ok := flagValue.Value.(flag.Getter)
	if !ok {
		return 0
	}
	timeout, ok := getter.Get().(time.Duration)
	if !ok {
		return 0
	}
	return timeout
}

// withCompactTestSettings installs fast, bounded settings for one test and restores them after.
//
// The production defaults (five-second active interval, five-minute idle) are correct for a
// deployment and far too slow for a test, so the intervals are compressed. Every other bound —
// batch sizes, bind caps, the health TTL's relationship to the idle interval — is left exactly
// as production computes it, because those are the properties under test.
// Parameters:
//   - t: test handle used for cleanup registration.
//
// Return values: none.
func withCompactTestSettings(t *testing.T) {
	requireCompactSuiteBudget(t)

	t.Helper()
	originalAuto := config.CompactUUIDAutoMigrate
	originalActive := config.CompactUUIDActiveInterval
	originalIdle := config.CompactUUIDIdleInterval
	originalRetry := config.CompactUUIDRetryInterval
	originalLock := config.CompactUUIDLockTimeout

	config.CompactUUIDAutoMigrate = true
	config.CompactUUIDActiveInterval = 10 * time.Millisecond
	config.CompactUUIDIdleInterval = 50 * time.Millisecond
	config.CompactUUIDRetryInterval = 10 * time.Millisecond
	config.CompactUUIDLockTimeout = time.Second

	t.Cleanup(func() {
		config.CompactUUIDAutoMigrate = originalAuto
		config.CompactUUIDActiveInterval = originalActive
		config.CompactUUIDIdleInterval = originalIdle
		config.CompactUUIDRetryInterval = originalRetry
		config.CompactUUIDLockTimeout = originalLock
		resetCompactHealthForTest()
	})
	resetCompactHealthForTest()
}

// newCompactTestTopology builds a unified SQLite topology with the v3 prerequisite satisfied.
//
// The v3 markers are written directly because this suite is about compact storage; the v3
// backfill has its own suite. Tests that specifically exercise the prerequisite gate write
// their own marker state instead of using this helper.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values:
//   - *gorm.DB: primary handle with the legacy schema migrated.
//   - *databaseTopology: unified topology.
func newCompactTestTopology(t *testing.T) (*gorm.DB, *databaseTopology) {
	t.Helper()
	withCompactTestSettings(t)
	db, topology := newUnifiedTestTopology(t)
	requireV3Markers(t, topology)
	return db, topology
}

// newCompactFileTestTopology builds a unified topology over a real SQLite file.
//
// A file-backed database is required, not merely nicer, for any test with concurrent
// connections: every connection to ":memory:" opens its OWN empty database, so a pool that
// grows past one connection would see a schema-less database and fail. It also exercises the
// production SQLite ownership path — the sidecar OS advisory lock — rather than the in-memory
// process mutex.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//
// Return values:
//   - *gorm.DB: primary handle with the legacy schema migrated.
//   - *databaseTopology: unified topology.
func newCompactFileTestTopology(t *testing.T) (*gorm.DB, *databaseTopology) {
	t.Helper()
	withCompactTestSettings(t)

	path := filepath.Join(t.TempDir(), "compact.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)

	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)

	withTestDBGlobals(t, db, db)
	require.NoError(t, migrateDB())

	topology, err := newUnifiedTopology(db)
	require.NoError(t, err)
	requireV3Markers(t, topology)

	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			_ = pool.Close()
		}
	})
	return db, topology
}

// requireV3Markers satisfies the external UUID v3 source prerequisite for a topology.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology whose applicable v3 markers are written.
//
// Return values: none.
func requireV3Markers(t *testing.T, topology *databaseTopology) {
	t.Helper()
	ctx := compactTestContext(t)
	for _, role := range topology.markerRoles() {
		require.NoError(t, markDataMigrationComplete(ctx, topology.handle(role), uuidCompletionKeyForRole(role)))
	}
}

// runCompactCycleForTest runs one real coordinator cycle under a real ownership claim.
// Parameters:
//   - t: test handle used for assertions.
//   - coordinator: coordinator under test.
//
// Return values:
//   - compactCycleResult: the cycle's result.
func runCompactCycleForTest(t *testing.T, coordinator *compactCoordinator) compactCycleResult {
	t.Helper()
	ctx := compactTestContext(t)
	ownership, acquired, err := acquireCompactOwnership(ctx, coordinator.topology)
	require.NoError(t, err)
	require.True(t, acquired, "test cycle must obtain ownership")
	defer ownership.release()

	result, err := runCompactCycle(ctx, coordinator, ownership)
	require.NoError(t, err)
	return result
}

// driveCompactToReady runs cycles until the coordinator reports ready or the bound is reached.
//
// The bound is a test failure rather than a silent stop: "reaches ready automatically, with no
// command" is the headline acceptance criterion (AUTO-A01), so a coordinator that stalls must
// fail loudly and report the state it stalled in.
// Parameters:
//   - t: test handle used for assertions.
//   - coordinator: coordinator under test.
//
// Return values:
//   - compactCycleResult: the terminal ready result.
func driveCompactToReady(t *testing.T, coordinator *compactCoordinator) compactCycleResult {
	t.Helper()
	// 27 targets need at most one expansion and one index cycle each, plus fill and two clean
	// validation passes; 200 is generous headroom without being unbounded.
	const maxCycles = 200
	result := compactCycleResult{}
	for cycle := 0; cycle < maxCycles; cycle++ {
		result = runCompactCycleForTest(t, coordinator)
		if result.state == compactStateReady {
			return result
		}
		require.NotEqual(t, compactStateBlockedValidation, result.state,
			"compact migration blocked unexpectedly: %s", result.reason)
	}
	t.Fatalf("compact migration did not reach ready; last state %q reason %q", result.state, result.reason)
	return result
}

// seedCompactUser inserts one users row through ordinary SQL, as any writer would.
//
// Writes go through the database rather than through a helper that sets the shadow directly:
// the derivation under test is the trigger's, and a helper that wrote the shadow itself would
// test nothing.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle to insert into.
//   - id: primary key.
//   - uuid: legacy UUID text, which may deliberately be invalid.
//
// Return values: none.
func seedCompactUser(t *testing.T, db *gorm.DB, id int, uuid string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, password, uuid) VALUES (?, ?, 'x', ?)",
		id, fmt.Sprintf("user-%d", id), uuid).Error)
}

// seedCompactUserNullUUID inserts one users row whose legacy UUID is SQL NULL.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle to insert into.
//   - id: primary key.
//
// Return values: none.
func seedCompactUserNullUUID(t *testing.T, db *gorm.DB, id int) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, password, uuid) VALUES (?, ?, 'x', NULL)",
		id, fmt.Sprintf("user-%d", id)).Error)
}

// readCompactShadowHex returns one row's shadow as uppercase hex, or an empty string for NULL.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle to read from.
//   - table: trusted table name.
//   - column: trusted compact column name.
//   - id: primary key.
//
// Return values:
//   - string: uppercase hex of the shadow, or an empty string when it is NULL.
func readCompactShadowHex(t *testing.T, db *gorm.DB, table string, column string, id int) string {
	t.Helper()
	var value *string
	require.NoError(t, db.Raw(
		"SELECT hex("+column+") FROM "+table+" WHERE id = ?", id).Scan(&value).Error)
	if value == nil {
		return ""
	}
	return *value
}

// compactUUIDTextFor returns a deterministic valid UUIDv7 text for a test index.
//
// Deterministic values keep a failure reproducible and let a test assert an exact derived
// vector rather than only "some bytes appeared".
// Parameters:
//   - index: distinguishing index, rendered into the final field.
//
// Return values:
//   - string: canonical UUIDv7 text.
func compactUUIDTextFor(index int) string {
	const digits = "0123456789abcdef"
	suffix := []byte("000000000000")
	position := len(suffix) - 1
	for value := index; value > 0 && position >= 0; value /= 16 {
		suffix[position] = digits[value%16]
		position--
	}
	return "018f0000-0000-7000-8000-" + string(suffix)
}

// dropCompactSyncTriggers removes one table's sync triggers to simulate drift.
//
// Tests that corrupt a shadow must do this first. The trigger re-derives the shadow from the
// row's own text on every update, so a direct corruption is undone in the same statement and
// the test would assert against an already-correct value.
// Parameters:
//   - t: test handle used for assertions.
//   - db: authoritative handle for the table.
//   - table: trusted table name.
//
// Return values: none.
func dropCompactSyncTriggers(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	for _, name := range []string{compactInsertTriggerName(table), compactUpdateTriggerName(table)} {
		require.NoError(t, db.Exec("DROP TRIGGER IF EXISTS "+name).Error)
	}
}

// timeNowUTCForTest returns the current UTC time for health-gate fixtures.
// Parameters: none.
//
// Return values:
//   - time.Time: current UTC time.
func timeNowUTCForTest() time.Time {
	return time.Now().UTC()
}

// upperOf uppercases ASCII text for hex and UUID assertions.
// Parameters:
//   - text: text to uppercase.
//
// Return values:
//   - string: uppercased text.
func upperOf(text string) string {
	return strings.ToUpper(text)
}

// readMarkerTimestamp returns one completion marker's recorded UTC timestamp.
//
// Marker stability across drift and repair is a contract, so tests compare this before and
// after inducing drift.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle carrying data_migrations.
//   - key: versioned migration key.
//
// Return values:
//   - time.Time: the marker's completion timestamp.
func readMarkerTimestamp(t *testing.T, db *gorm.DB, key string) time.Time {
	t.Helper()
	marker := DataMigration{}
	require.NoError(t, db.Where("migration_key = ?", key).First(&marker).Error)
	return marker.CompletedAt
}

// expandAndTriggerAll expands every table of a role and installs its trigger set.
//
// It is the shared setup for tests that care about post-expansion behavior rather than about
// the expansion sequence itself.
// Parameters:
//   - t: test handle used for assertions.
//   - db: authoritative handle.
//   - role: database role to expand.
//
// Return values: none.
func expandAndTriggerAll(t *testing.T, db *gorm.DB, role uuidDBRole) {
	t.Helper()
	ctx := compactTestContext(t)
	for _, table := range compactTablesForRole(role) {
		_, err := expandCompactTable(ctx, db, table)
		require.NoError(t, err, "expand %s", table.table)
		require.NoError(t, installCompactTriggers(ctx, db, table), "install triggers for %s", table.table)
	}
}
