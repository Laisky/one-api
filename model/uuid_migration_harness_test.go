package model

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// uuidQueryCounter counts SQL statements issued through an instrumented handle so tests can
// assert that a completed startup performs marker lookups only.
type uuidQueryCounter struct {
	total   atomic.Int64
	writes  atomic.Int64
	byTable sync.Map
}

// installQueryCounter attaches callbacks that count every statement a handle executes.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle to instrument.
//
// Return values:
//   - *uuidQueryCounter: counter observing the handle.
func installQueryCounter(t *testing.T, db *gorm.DB) *uuidQueryCounter {
	t.Helper()
	counter := &uuidQueryCounter{}
	record := func(tx *gorm.DB) {
		counter.total.Add(1)
		table := tx.Statement.Table
		if table == "" && tx.Statement.SQL.Len() > 0 {
			table = uuidTableFromSQL(tx.Statement.SQL.String())
		}
		if table == "" {
			table = "unknown"
		}
		value, _ := counter.byTable.LoadOrStore(table, new(atomic.Int64))
		value.(*atomic.Int64).Add(1)
	}
	recordWrite := func(tx *gorm.DB) {
		counter.writes.Add(1)
		record(tx)
	}
	for name, register := range map[string]func(string, func(*gorm.DB)) error{
		"query": db.Callback().Query().After("gorm:query").Register,
		"row":   db.Callback().Row().After("gorm:row").Register,
		"raw":   db.Callback().Raw().After("gorm:raw").Register,
	} {
		require.NoError(t, register("uuidtest:count_"+name, record))
	}
	for name, register := range map[string]func(string, func(*gorm.DB)) error{
		"update": db.Callback().Update().After("gorm:update").Register,
		"create": db.Callback().Create().After("gorm:create").Register,
		"delete": db.Callback().Delete().After("gorm:delete").Register,
	} {
		require.NoError(t, register("uuidtest:count_"+name, recordWrite))
	}
	return counter
}

// requireNoUUIDTableAccess asserts that no UUID target or reference table was queried.
// Parameters:
//   - t: test handle used for assertions.
//   - counter: counter observing the handle.
//   - role: database role whose registry tables must be untouched.
//
// Return values: none.
func requireNoUUIDTableAccess(t *testing.T, counter *uuidQueryCounter, role uuidDBRole) {
	t.Helper()
	for _, target := range ownedTargetsForRole(role) {
		require.Zero(t, counter.count(target.table),
			"completed startup must not query UUID target table %s", target.table)
	}
	for _, refRole := range []uuidDBRole{uuidRolePrimary, uuidRoleLog} {
		for _, target := range fkTargetsForRoles(role, refRole) {
			require.Zero(t, counter.count(target.table),
				"completed startup must not query UUID target table %s", target.table)
			require.Zero(t, counter.count(target.refTable),
				"completed startup must not query UUID reference table %s", target.refTable)
		}
	}
	require.Zero(t, counter.writes.Load(), "completed startup must perform no migration writes")
}

// count returns the number of statements observed against one table.
// Parameters:
//   - table: table name to report.
//
// Return values:
//   - int: statements observed for the table.
func (counter *uuidQueryCounter) count(table string) int {
	value, ok := counter.byTable.Load(table)
	if !ok {
		return 0
	}
	return int(value.(*atomic.Int64).Load())
}

// uuidTableFromSQL extracts a table name from a raw statement for counting purposes.
// Parameters:
//   - sql: raw SQL text.
//
// Return values:
//   - string: best-effort table name, or an empty string.
func uuidTableFromSQL(sql string) string {
	fields := strings.Fields(strings.ToLower(strings.ReplaceAll(sql, `"`, " ")))
	for i, field := range fields {
		if (field == "from" || field == "into" || field == "update") && i+1 < len(fields) {
			return strings.Trim(fields[i+1], "`(),;")
		}
	}
	return ""
}

// newUnifiedTestTopology builds an initialized unified topology over one SQLite handle.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values:
//   - *gorm.DB: primary handle with the full schema migrated.
//   - *databaseTopology: unified topology.
func newUnifiedTestTopology(t *testing.T) (*gorm.DB, *databaseTopology) {
	t.Helper()
	db := setupMigrationTestDB(t)
	withTestDBGlobals(t, db, db)
	require.NoError(t, migrateDB())
	topology, err := newUnifiedTopology(db)
	require.NoError(t, err)
	return db, topology
}

// newSplitTestTopology builds an initialized split topology over two SQLite handles.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values:
//   - *gorm.DB: primary handle with the full schema migrated.
//   - *gorm.DB: authoritative log handle.
//   - *databaseTopology: split topology.
func newSplitTestTopology(t *testing.T) (*gorm.DB, *gorm.DB, *databaseTopology) {
	t.Helper()
	primary := setupMigrationTestDB(t)
	logDB := setupMigrationTestDB(t)
	withTestDBGlobals(t, primary, logDB)
	require.NoError(t, migrateDB())
	require.NoError(t, logDB.AutoMigrate(&Log{}, &DataMigration{}))
	topology, err := newSplitTopology(primary, logDB)
	require.NoError(t, err)
	return primary, logDB, topology
}

// withTestDBGlobals points the package handles at test databases and restores them after.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - primary: primary handle.
//   - logDB: log handle.
//
// Return values: none.
func withTestDBGlobals(t *testing.T, primary *gorm.DB, logDB *gorm.DB) {
	t.Helper()
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = primary
	LOG_DB = logDB
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
		resetBootstrapStateForTest()
	})
}

// withFinalizerEnabled turns the finalizer gate on for one test.
// SQLite promotion no longer needs the blocking-DDL flag, so nothing else is granted.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - enabled: desired gate state.
//
// Return values: none.
func withFinalizerEnabled(t *testing.T, enabled bool) {
	t.Helper()
	original := externalUUIDBackfillFinalizerEnabled
	externalUUIDBackfillFinalizerEnabled = enabled
	t.Cleanup(func() { externalUUIDBackfillFinalizerEnabled = original })
}

// withBlockingDDLAllowed sets the operator's blocking-DDL approval for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - allowed: desired approval state.
//
// Return values: none.
func withBlockingDDLAllowed(t *testing.T, allowed bool) {
	t.Helper()
	original := config.ExternalUUIDBackfillAllowBlockingDDL
	config.ExternalUUIDBackfillAllowBlockingDDL = allowed
	t.Cleanup(func() { config.ExternalUUIDBackfillAllowBlockingDDL = original })
}

// withAutoFinalize sets the automatic-completion policy and quiescence threshold for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - enabled: desired automatic-completion state.
//   - idlePasses: consecutive no-work passes required before finalizing.
//
// Return values: none.
func withAutoFinalize(t *testing.T, enabled bool, idlePasses int) {
	t.Helper()
	originalEnabled := config.ExternalUUIDBackfillAutoFinalize
	originalPasses := config.ExternalUUIDBackfillAutoFinalizeIdlePasses
	config.ExternalUUIDBackfillAutoFinalize = enabled
	config.ExternalUUIDBackfillAutoFinalizeIdlePasses = idlePasses
	t.Cleanup(func() {
		config.ExternalUUIDBackfillAutoFinalize = originalEnabled
		config.ExternalUUIDBackfillAutoFinalizeIdlePasses = originalPasses
	})
}

// withCatchUpIntervals shrinks the background worker's scheduling delays for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - active: delay after a cycle that observed backlog.
//   - idle: delay after a full no-work pass.
//
// Return values: none.
func withCatchUpIntervals(t *testing.T, active time.Duration, idle time.Duration) {
	t.Helper()
	originalActive := config.ExternalUUIDBackfillActiveInterval
	originalIdle := config.ExternalUUIDBackfillIdleInterval
	config.ExternalUUIDBackfillActiveInterval = active
	config.ExternalUUIDBackfillIdleInterval = idle
	t.Cleanup(func() {
		config.ExternalUUIDBackfillActiveInterval = originalActive
		config.ExternalUUIDBackfillIdleInterval = originalIdle
	})
}

// runCatchUp runs one catch-up cycle over the topology.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology under test.
//
// Return values:
//   - uuidMigrationResult: coordinator result.
func runCatchUp(t *testing.T, topology *databaseTopology) uuidMigrationResult {
	t.Helper()
	result, err := runUUIDMigrationCoordinator(context.Background(), topology, uuidMigrationModeCatchUp)
	require.NoError(t, err)
	return result
}

// runFinalizer runs one finalizer cycle over the topology.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology under test.
//
// Return values:
//   - uuidMigrationResult: coordinator result.
//   - error: coordinator error, returned so tests can assert failures.
func runFinalizer(t *testing.T, topology *databaseTopology) (uuidMigrationResult, error) {
	t.Helper()
	return runUUIDMigrationCoordinator(context.Background(), topology, uuidMigrationModeFinalizer)
}

// requireHyphenatedUUID asserts that uuid looks like the canonical external UUID form.
// Parameters:
//   - t: test handle used for assertions.
//   - uuid: value to inspect.
//
// Return values: none.
func requireHyphenatedUUID(t *testing.T, uuid string) {
	t.Helper()
	require.Len(t, uuid, 36)
	require.Equal(t, 4, strings.Count(uuid, "-"))
	require.True(t, isCanonicalHyphenatedUUID(uuid), "uuid must parse as canonical: %s", uuid)
}

// requireUUIDUniqueIndex asserts that the target table has a verified UUID unique index.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning the table.
//   - target: owned UUID registry entry.
//
// Return values: none.
func requireUUIDUniqueIndex(t *testing.T, db *gorm.DB, target uuidOwnedTarget) {
	t.Helper()
	verified, err := verifyUniqueUUIDIndex(context.Background(), db, target, uuidUniqueIndexName(target.table))
	require.NoError(t, err)
	require.True(t, verified, "missing verified UUID unique index for %s", target.table)
}

// dropUUIDMigrationTables clears UUID-migrated tables for live-database matrix tests.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle to clean.
//
// Return values: none.
func dropUUIDMigrationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Migrator().DropTable(
		&PasskeyCredential{},
		&MCPTool{},
		&MCPServer{},
		&AsyncTaskBinding{},
		&Trace{},
		&UserRequestCost{},
		&TokenTransaction{},
		&Log{},
		&Redemption{},
		&Ability{},
		&Option{},
		&Token{},
		&Channel{},
		&User{},
		&DataMigration{},
	))
}

// seedLegacyUUIDRows inserts rows without UUID values to simulate a pre-migration database.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//
// Return values: none.
func seedLegacyUUIDRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Table("users").Create([]map[string]any{
		{"id": 1, "username": "root", "password": "password-hash", "inviter_id": 0},
		{"id": 2, "username": "child", "password": "password-hash", "inviter_id": 1},
	}).Error)
	require.NoError(t, db.Table("channels").Create(map[string]any{
		"id": 1, "type": 1, "name": "primary", "models": "gpt-4o", "config": "{}",
	}).Error)
	require.NoError(t, db.Table("tokens").Create(map[string]any{
		"id": 1, "user_id": 1, "key": "legacy-token-key", "name": "default",
	}).Error)
	require.NoError(t, db.Table("logs").Create(map[string]any{
		"id": 1, "user_id": 1, "channel_id": 1, "type": 1, "token_name": "default", "content": "legacy log",
	}).Error)
	require.NoError(t, db.Table("redemptions").Create(map[string]any{
		"id": 1, "user_id": 1, "key": "legacy-redemption-key", "name": "gift",
	}).Error)
}

// requireMarker asserts the presence or absence of a completion marker.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle carrying data_migrations.
//   - key: completion key to inspect.
//   - want: expected presence.
//
// Return values: none.
func requireMarker(t *testing.T, db *gorm.DB, key string, want bool) {
	t.Helper()
	complete, err := isDataMigrationComplete(context.Background(), db, key)
	require.NoError(t, err)
	require.Equal(t, want, complete, "marker %s presence", key)
}
