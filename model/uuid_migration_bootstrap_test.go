package model

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/env"
)

// bootstrapTestCatchUpWorkerRunning reports whether a background catch-up worker is registered.
// It reads the process-local bootstrap state under bootstrapMu so the assertion is race-safe.
// Parameters: none.
//
// Return values:
//   - bool: true when startUUIDCatchUpWorker installed a cancel function that is still live.
func bootstrapTestCatchUpWorkerRunning() bool {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()
	return catchUpWorkerCancel != nil
}

// withBootstrapLogSQLDSN overrides config.LogSQLDSN for one test and restores it afterwards.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - dsn: desired log DSN value; an empty string selects unified deployment semantics.
//
// Return values: none.
func withBootstrapLogSQLDSN(t *testing.T, dsn string) {
	t.Helper()
	original := config.LogSQLDSN
	config.LogSQLDSN = dsn
	t.Cleanup(func() { config.LogSQLDSN = original })
}

// withBootstrapMasterNode overrides config.IsMasterNode for one test and restores it afterwards.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - master: desired master-node state.
//
// Return values: none.
func withBootstrapMasterNode(t *testing.T, master bool) {
	t.Helper()
	original := config.IsMasterNode
	config.IsMasterNode = master
	t.Cleanup(func() { config.IsMasterNode = original })
}

// seedBootstrapLegacyRows inserts owned and referencing rows that carry no UUID values.
// The rows exercise every dependency direction the compatibility catch-up must reconcile on a
// unified database: primary owners, a primary-local self reference, and log references.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//
// Return values: none.
func seedBootstrapLegacyRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password, inviter_id) VALUES (1, 'root', 'password-hash', 0), (2, 'child', 'password-hash', 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'primary', 'gpt-4o', '{}')").Error)
	require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (1, 1, 'legacy-token-key', 'default')").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, channel_id, type, token_name, content) VALUES (1, 1, 1, 1, 'default', 'legacy log')").Error)
}

// TestBootstrapInitDBOnlyRetainsPrimaryCatchUp covers UUID-A09 and UUID-017: the InitDB-only
// compatibility path retains the historical primary-only catch-up side effect, filling owned and
// FK UUIDs while writing no completion marker, because it has no completion authority.
//
// The side effect is preserved but is no longer synchronous: the backlog and its candidate-index
// DDL run in the bounded background worker, so InitDB does not block readiness on them and a
// backlog larger than one cycle is not stranded until the next restart.
func TestBootstrapInitDBOnlyRetainsPrimaryCatchUp(t *testing.T) {
	db, _ := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	// runCompatibilityCatchUp returns early for a split deployment, so unified semantics
	// must be selected explicitly instead of inherited from the developer's environment.
	withBootstrapLogSQLDSN(t, "")
	// The catch-up now runs on a background goroutine. Each new connection to an in-memory
	// SQLite DSN opens a separate empty database, so the handle must be pinned to one
	// connection or the worker would reconcile a different, empty database.
	pinUUIDRaceSQLiteConnection(t, db)
	seedBootstrapLegacyRows(t, db)

	// The compatibility flag is process-global by design: InitDB runs once per process. Other
	// tests in this binary legitimately call InitDB, so model a fresh process explicitly
	// rather than depending on test execution order.
	resetBootstrapStateForTest()
	t.Cleanup(resetBootstrapStateForTest)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, runCompatibilityCatchUp(ctx))

	// The worker drains the backlog asynchronously.
	require.Eventually(t, func() bool {
		var missing int64
		if err := db.Table("logs").Where("token_uuid IS NULL OR token_uuid = ''").Count(&missing).Error; err != nil {
			return false
		}
		return missing == 0
	}, 20*time.Second, 20*time.Millisecond, "the compatibility catch-up worker must drain the backlog")

	var root User
	require.NoError(t, db.First(&root, "id = ?", 1).Error)
	requireHyphenatedUUID(t, root.UUID)

	var child User
	require.NoError(t, db.First(&child, "id = ?", 2).Error)
	requireHyphenatedUUID(t, child.UUID)
	require.NotNil(t, child.InviterUUID, "the primary-local fk uuid must be filled")
	require.Equal(t, root.UUID, *child.InviterUUID)

	var channel Channel
	require.NoError(t, db.First(&channel, "id = ?", 1).Error)
	requireHyphenatedUUID(t, channel.UUID)

	var token Token
	require.NoError(t, db.First(&token, "id = ?", 1).Error)
	requireHyphenatedUUID(t, token.UUID)
	require.NotNil(t, token.UserUUID)
	require.Equal(t, root.UUID, *token.UserUUID)

	var log Log
	require.NoError(t, db.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID)
	require.Equal(t, root.UUID, *log.UserUUID)
	require.NotNil(t, log.ChannelUUID)
	require.Equal(t, channel.UUID, *log.ChannelUUID)
	require.NotNil(t, log.TokenUUID)
	require.Equal(t, token.UUID, *log.TokenUUID)

	// The compatibility path is marker-free: only the global coordinator may complete.
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
	require.False(t, db.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")),
		"the compatibility catch-up must not promote indexes")
}

// TestBootstrapUnifiedLogDBDoesNotDuplicateWork covers UUID-A09 and UUID-018: a unified
// LOG_DB == DB deployment must not run the same catch-up twice. The compatibility flag flips
// exactly once, a repeated compatibility call is a no-op, and the InitDB plus InitLogDB wrapper
// path performs no additional reconciliation once the compatibility catch-up already ran.
func TestBootstrapUnifiedLogDBDoesNotDuplicateWork(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withBootstrapLogSQLDSN(t, "")
	t.Cleanup(resetBootstrapStateForTest)
	seedBootstrapLegacyRows(t, db)

	// The compatibility flag is process-global by design: in production InitDB runs once.
	// Other tests in this binary legitimately call InitDB, so reset the flag here to model a
	// fresh process instead of depending on test execution order.
	resetBootstrapStateForTest()
	setDatabaseTopology(topology)

	require.False(t, compatibilityCatchUpAlreadyRan(),
		"a fresh process has not yet run the primary-only catch-up")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, runCompatibilityCatchUp(ctx))
	require.True(t, compatibilityCatchUpAlreadyRan(),
		"the compatibility catch-up must record that it owns reconciliation")
	require.True(t, bootstrapTestCatchUpWorkerRunning(),
		"the compatibility catch-up owns exactly one background worker")

	// Reconciliation now happens on that worker, so duplication is measured by whether a
	// SECOND worker gets started, not by counting statements: the first worker keeps issuing
	// its own queries in the background. Stopping it makes any new worker unambiguous.
	stopUUIDCatchUpWorker()
	require.False(t, bootstrapTestCatchUpWorkerRunning())

	require.NoError(t, runCompatibilityCatchUp(ctx),
		"a repeated compatibility catch-up must be a no-op")
	require.False(t, bootstrapTestCatchUpWorkerRunning(),
		"a repeated compatibility catch-up must not start a second worker")

	// The wrapper path sees unified topology, a disabled finalizer gate, and a compatibility
	// catch-up that already owns reconciliation, so it must not repeat the identical catch-up.
	require.NoError(t, runWrapperUUIDMigration(ctx, topology))
	require.False(t, bootstrapTestCatchUpWorkerRunning(),
		"suppressed wrapper reconciliation must not start a duplicate background worker")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
}

// TestBootstrapCompatibilityCatchUpNeverFinalizesSplitState covers UUID-A09 and UUID-017: in a
// split deployment the primary logs table is not authoritative, so reconciling it from the
// InitDB-only path could scan or mutate stale primary rows. Only the global coordinator reached
// through InitLogDB or InitDatabases may run, therefore the compatibility path must return
// without touching any database.
func TestBootstrapCompatibilityCatchUpNeverFinalizesSplitState(t *testing.T) {
	db, _ := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	// A non-empty log DSN is exactly how initLogDatabase selects split mode.
	withBootstrapLogSQLDSN(t, "postgres://unused-split-log-dsn/example")
	t.Cleanup(resetBootstrapStateForTest)
	seedBootstrapLegacyRows(t, db)

	// Model a fresh process: the flag is process-global and other tests in this binary
	// legitimately call InitDB, so ownership must not be inherited from test order.
	resetBootstrapStateForTest()

	counter := installQueryCounter(t, db)
	require.NoError(t, runCompatibilityCatchUp(context.Background()),
		"the split-mode compatibility path must defer to the global coordinator, not fail")

	require.Zero(t, counter.total.Load(),
		"the split-mode compatibility path must issue no statements at all")
	requireNoUUIDTableAccess(t, counter, uuidRolePrimary)
	require.False(t, compatibilityCatchUpAlreadyRan(),
		"deferring to the global coordinator must not claim reconciliation ownership")

	var root User
	require.NoError(t, db.First(&root, "id = ?", 1).Error)
	require.Empty(t, root.UUID, "no owned uuid may be written from the split-mode compatibility path")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
}

// TestBootstrapRunExternalUUIDMigrationsRejectsInvalidOrder covers UUID-A09 and UUID-019:
// RunExternalUUIDMigrations is the supported completion-capable API and must reject an invalid
// initialization order with a wrapped error instead of rediscovering topology itself.
func TestBootstrapRunExternalUUIDMigrationsRejectsInvalidOrder(t *testing.T) {
	t.Cleanup(resetBootstrapStateForTest)

	setDatabaseTopology(nil)
	err := RunExternalUUIDMigrations(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "topology")
}

// TestBootstrapRunExternalUUIDMigrationsUsesInstalledTopology covers UUID-A09 and UUID-019:
// the supported entry point reconciles the topology installed at initialization, and with the
// finalizer gate disabled it stays in marker-free catch-up mode.
func TestBootstrapRunExternalUUIDMigrationsUsesInstalledTopology(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	t.Cleanup(resetBootstrapStateForTest)
	setDatabaseTopology(topology)
	seedLegacyUsers(t, db, 1)

	require.NoError(t, RunExternalUUIDMigrations(context.Background()))

	var user User
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	requireHyphenatedUUID(t, user.UUID)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
}

// TestBootstrapNonMasterStartupSkipsUUIDMigration covers UUID-A41: a non-master startup performs
// no UUID reconciliation, no index DDL, and no marker write.
//
// Scope note: InitDatabases and InitDB cannot be exercised here because initPrimaryDatabase opens
// a real handle from config.SQLDSN, which a unit test must not do. InitLogDB carries the identical
// non-master guard (topology is installed, then reconciliation is skipped unless
// config.IsMasterNode), and with an empty config.LogSQLDSN it opens nothing: it reuses the already
// installed primary handle. This test therefore drives the real wrapper over test handles and
// asserts the guard's observable contract with a query counter.
func TestBootstrapNonMasterStartupSkipsUUIDMigration(t *testing.T) {
	db, _ := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	withBootstrapLogSQLDSN(t, "")
	withBootstrapMasterNode(t, false)
	t.Cleanup(resetBootstrapStateForTest)
	seedBootstrapLegacyRows(t, db)

	counter := installQueryCounter(t, db)
	InitLogDB()

	// Topology initialization still happens: only reconciliation is guarded.
	topology := databaseTopologySnapshot()
	require.NotNil(t, topology, "a non-master node still installs its topology")
	require.Equal(t, uuidTopologyUnified, topology.mode)

	require.Zero(t, counter.total.Load(),
		"a non-master startup must issue no catch-up, DDL, marker, or probe statements")
	requireNoUUIDTableAccess(t, counter, uuidRolePrimary)
	requireNoUUIDTableAccess(t, counter, uuidRoleLog)
	require.False(t, bootstrapTestCatchUpWorkerRunning(),
		"a non-master node must not start a background catch-up worker")

	var root User
	require.NoError(t, db.First(&root, "id = ?", 1).Error)
	require.Empty(t, root.UUID, "a non-master node must not backfill uuids")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
	require.False(t, db.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")),
		"a non-master node must not promote indexes")
}

// TestBootstrapFinalizerGateDefaultsToDisabled covers UUID-A41: the finalizer configuration
// defaults to disabled, so a process only finalizes after the UUID-aware-writer barrier is
// explicitly declared through EXTERNAL_UUID_BACKFILL_FINALIZER.
func TestBootstrapFinalizerGateDefaultsToDisabled(t *testing.T) {
	const gateEnvVar = "EXTERNAL_UUID_BACKFILL_FINALIZER"
	if os.Getenv(gateEnvVar) != "" {
		t.Skipf("%s is set in this environment, so the package default cannot be observed", gateEnvVar)
	}

	require.False(t, env.Bool(gateEnvVar, false),
		"an unset finalizer gate must resolve to disabled")
	require.False(t, externalUUIDBackfillFinalizerEnabled,
		"the package finalizer gate must default to disabled")
}

// TestBootstrapFinalizerModeFailsStartupSynchronously covers UUID-A41 and UUID-021: finalizer
// reconciliation is synchronous, so a failing phase fails startup through the returned error and
// leaves no completion marker behind.
func TestBootstrapFinalizerModeFailsStartupSynchronously(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	t.Cleanup(resetBootstrapStateForTest)

	// Two users share one owned uuid, which no phase may repair and validation must reject.
	shared := "018f0000-0000-7000-8000-000000000001"
	require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, ?, 'root', 'password-hash'), (2, ?, 'other', 'password-hash')", shared, shared).Error)

	err := startExternalUUIDMigration(context.Background(), topology)
	require.Error(t, err, "finalizer mode must fail startup rather than degrade to background work")
	require.ErrorContains(t, err, "finalize external resource uuids")
	require.ErrorContains(t, err, "duplicate owned uuid")

	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
	require.False(t, bootstrapTestCatchUpWorkerRunning(),
		"finalizer mode is synchronous and must not hand work to a background worker")
	require.False(t, db.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")))
}

// TestBootstrapCatchUpRunsInBoundedBackgroundWorker covers UUID-A41 and UUID-020: non-finalizer
// catch-up leaves the readiness-critical path. Startup returns immediately, the backlog drains
// asynchronously in a context-bound worker, no marker is ever written, and cancelling the context
// stops the worker without a panic or a leaked goroutine.
func TestBootstrapCatchUpRunsInBoundedBackgroundWorker(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	t.Cleanup(resetBootstrapStateForTest)

	// The worker and this goroutine share one in-memory SQLite database. Each extra connection
	// to ":memory:" would open a separate empty database, so pin the pool to a single
	// connection and let the driver serialize the concurrent access instead.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	const legacyUsers = 25
	seedLegacyUsers(t, db, legacyUsers)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	baselineGoroutines := runtime.NumGoroutine()
	started := time.Now()
	require.NoError(t, startExternalUUIDMigration(ctx, topology))
	require.Less(t, time.Since(started), time.Second,
		"readiness must not wait for historical catch-up")
	require.True(t, bootstrapTestCatchUpWorkerRunning(),
		"an incomplete catch-up must be resumed by a background worker")

	require.Eventually(t, func() bool {
		var filled int64
		if err := db.Table("users").Where("uuid IS NOT NULL AND uuid != ''").Count(&filled).Error; err != nil {
			return false
		}
		return filled == legacyUsers
	}, 10*time.Second, 20*time.Millisecond, "the background worker must drain the backlog")

	// A catch-up cycle has no completion authority regardless of how much it reconciled.
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)

	cancel()
	stopUUIDCatchUpWorker()
	// Stopping twice must stay safe: CloseDB may run after a cancelled bootstrap context.
	stopUUIDCatchUpWorker()
	require.False(t, bootstrapTestCatchUpWorkerRunning())

	// Poll from this goroutine rather than through require.Eventually, whose condition runs in
	// an extra goroutine that would itself be counted.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > baselineGoroutines && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	require.LessOrEqual(t, runtime.NumGoroutine(), baselineGoroutines,
		"the cancelled worker must exit and leak no goroutine")
}

// TestBootstrapCompletedMarkersStartNoWorker covers UUID-A41 and UUID-020: once the
// current-generation markers exist, startup is marker-only. It starts no background worker and
// issues no UUID target or reference query.
func TestBootstrapCompletedMarkersStartNoWorker(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)
	t.Cleanup(resetBootstrapStateForTest)
	seedBootstrapLegacyRows(t, db)
	require.NoError(t, markDataMigrationComplete(context.Background(), db, externalUUIDPrimaryMigrationKey))

	counter := installQueryCounter(t, db)
	require.NoError(t, startExternalUUIDMigration(context.Background(), topology))

	require.Equal(t, 1, counter.count("data_migrations"),
		"completed unified startup performs exactly one marker lookup")
	require.EqualValues(t, 1, counter.total.Load(),
		"completed startup performs no statement beyond the marker lookup")
	requireNoUUIDTableAccess(t, counter, uuidRolePrimary)
	requireNoUUIDTableAccess(t, counter, uuidRoleLog)
	require.False(t, bootstrapTestCatchUpWorkerRunning(),
		"completed markers mean there is nothing for a worker to do")
}
