package model

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Backend selection, DSN gating, and the live-database fixture shared by the MySQL and
// PostgreSQL acceptance matrix.

// uuidMultiDBBackends lists the live backends every acceptance job must cover. Release
// qualification sets ONEAPI_REQUIRE_DB_BACKENDS=1 so a missing DSN fails instead of skipping.
var uuidMultiDBBackends = []string{"mysql", "postgres"}

const requireDBBackendsEnv = "ONEAPI_REQUIRE_DB_BACKENDS"

const (
	// uuidPlanUserRows is large enough that a sequential scan stops being the planner's
	// cheapest option for a selective candidate predicate.
	uuidPlanUserRows = 20000
	// uuidPlanMissingPerPass is how many users stay missing for the NULL pass and, separately,
	// for the empty-string pass.
	uuidPlanMissingPerPass = 5
	// uuidPlanTokenRows is large enough that the composite token lookup must be indexed.
	uuidPlanTokenRows = 5000
	// uuidPlanTokenUsers is how many users share each token name. Names repeat across users
	// and are unique within a user, so (user_id, name) is the selective key and the
	// single-column name index alone cannot serve the bounded lookup.
	uuidPlanTokenUsers = 100
	// uuidPromotionUserRows keeps the online promotion test honest while staying CI-fast.
	uuidPromotionUserRows = 5000
	// uuidWorkloadFirstID starts the concurrent workload's ids above every seeded id.
	uuidWorkloadFirstID = 1000001
)

// uuidBackendDSNEnv returns the env var carrying a backend's primary DSN.
// Parameters:
//   - backend: "mysql" or "postgres".
//
// Return values:
//   - string: env var name.
func uuidBackendDSNEnv(backend string) string {
	if backend == "mysql" {
		return "MYSQL_DSN"
	}
	return "PG_DSN"
}

// uuidBackendLogDSNEnv returns the env var carrying a backend's optional dedicated log DSN.
// Parameters:
//   - backend: "mysql" or "postgres".
//
// Return values:
//   - string: env var name.
func uuidBackendLogDSNEnv(backend string) string {
	if backend == "mysql" {
		return "MYSQL_LOG_DSN"
	}
	return "PG_LOG_DSN"
}

// uuidBackendsAreMandatory reports whether a missing DSN must fail the job instead of skipping.
// Parameters: none.
//
// Return values:
//   - bool: true when ONEAPI_REQUIRE_DB_BACKENDS=1.
func uuidBackendsAreMandatory() bool {
	return os.Getenv(requireDBBackendsEnv) == "1"
}

// requireBackend opens a live backend, failing rather than skipping in release qualification.
// Ordinary developer runs skip when the DSN is unset; the acceptance job sets
// ONEAPI_REQUIRE_DB_BACKENDS=1 so a MySQL or PostgreSQL subtest cannot silently disappear.
// Parameters:
//   - t: test handle used for assertions.
//   - backend: "mysql" or "postgres".
//
// Return values:
//   - *gorm.DB: live handle; the call never returns when the backend is unavailable.
func requireBackend(t *testing.T, backend string) *gorm.DB {
	t.Helper()
	if db := openBackend(t, backend); db != nil {
		return db
	}
	dsnVar := uuidBackendDSNEnv(backend)
	if uuidBackendsAreMandatory() {
		t.Fatalf("%s=1 makes live backends mandatory for release qualification, but %s is unset: "+
			"the %s acceptance subtest must not silently skip", requireDBBackendsEnv, dsnVar, backend)
	}
	t.Skipf("%s is unset; skipping the live %s subtest (set %s=1 to make it mandatory)",
		dsnVar, backend, requireDBBackendsEnv)
	return nil
}

// openUUIDSecondHandle opens the independent handle a split topology uses as its log database.
// A dedicated DSN is used when configured. Otherwise the primary DSN is opened a second time,
// which is legitimate for these tests because the proposal selects split mode from
// CONFIGURATION and never from handle identity: a deployment may legally point LOG_DSN and
// SQL_DSN at one physical server, and newSplitTopology still treats it as split. The two
// handles then own independent connection pools (which the outage test depends on) while
// sharing one physical logs and data_migrations table. Assertions in this file stay valid for
// that case because the log marker key differs from the primary marker key, and because
// seeding places the authoritative logs row wherever the log role actually reads it.
// Parameters:
//   - t: test handle used for assertions.
//   - backend: "mysql" or "postgres".
//
// Return values:
//   - *gorm.DB: second live handle.
//   - bool: true when a dedicated log DSN was configured, so the log database is physically
//     separate from the primary.
func openUUIDSecondHandle(t *testing.T, backend string) (*gorm.DB, bool) {
	t.Helper()
	dsn := os.Getenv(uuidBackendLogDSNEnv(backend))
	dedicated := dsn != ""
	if !dedicated {
		dsn = os.Getenv(uuidBackendDSNEnv(backend))
	}
	require.NotEmpty(t, dsn, "a %s DSN must be configured before opening the log handle", backend)

	var dialer gorm.Dialector
	switch backend {
	case "mysql":
		dialer = mysql.Open(dsn)
	case "postgres":
		dialer = postgres.Open(dsn)
	default:
		t.Fatalf("unknown backend %q", backend)
	}
	db, err := gorm.Open(dialer, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err, "open second %s handle", backend)
	return db, dedicated
}

// uuidMultiDBFixture is one prepared live-backend topology with a clean schema.
type uuidMultiDBFixture struct {
	// backend is "mysql" or "postgres".
	backend string
	// primary is the handle owning every non-log resource table.
	primary *gorm.DB
	// logDB is the handle owning the authoritative logs table; it equals primary when unified.
	logDB *gorm.DB
	// topology is the explicitly constructed topology under test.
	topology *databaseTopology
	// dedicatedLog is true when the log handle points at a physically separate database.
	dedicatedLog bool
	// logClosed records that a test deliberately closed the log pool to simulate an outage.
	logClosed bool
}

// newUUIDMultiDBFixture drops every UUID-migrated table, migrates a fresh schema, and builds
// the requested topology on a live backend.
// Parameters:
//   - t: test handle used for assertions and cleanup registration.
//   - backend: "mysql" or "postgres".
//   - mode: unified or split topology.
//
// Return values:
//   - *uuidMultiDBFixture: prepared fixture.
func newUUIDMultiDBFixture(t *testing.T, backend string, mode uuidTopologyMode) *uuidMultiDBFixture {
	t.Helper()
	primary := requireBackend(t, backend)
	fixture := &uuidMultiDBFixture{backend: backend, primary: primary, logDB: primary}
	if mode == uuidTopologySplit {
		fixture.logDB, fixture.dedicatedLog = openUUIDSecondHandle(t, backend)
	}

	// Registered first so it runs last: withTestDBGlobals restores DB and LOG_DB before the
	// tables are dropped, and the drop uses the fixture's own handles either way.
	t.Cleanup(func() {
		fixture.dropAll(t)
		resetBackendFlags()
	})
	fixture.dropAll(t)

	withTestDBGlobals(t, primary, fixture.logDB)
	require.NoError(t, migrateDB(), "migrate the primary schema on %s", backend)
	if mode == uuidTopologySplit {
		require.NoError(t, fixture.logDB.AutoMigrate(&Log{}, &DataMigration{}),
			"migrate the log schema on %s", backend)
	}

	var err error
	if mode == uuidTopologySplit {
		fixture.topology, err = newSplitTopology(primary, fixture.logDB)
	} else {
		fixture.topology, err = newUnifiedTopology(primary)
	}
	require.NoError(t, err, "build the %s topology", mode)
	return fixture
}

// dropAll removes every UUID-migrated table from each physical database in the fixture.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func (fixture *uuidMultiDBFixture) dropAll(t *testing.T) {
	t.Helper()
	dropUUIDMigrationTables(t, fixture.primary)
	if !fixture.dedicatedLog {
		// A shared physical database is already fully cleaned through the primary handle.
		return
	}
	logDB := fixture.logDB
	if fixture.logClosed {
		// The outage test closed this pool, so reopen one purely to clean up.
		logDB, _ = openUUIDSecondHandle(t, fixture.backend)
	}
	dropUUIDMigrationTables(t, logDB)
}

// closeLogHandle closes the log database's connection pool to simulate a reference outage.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func (fixture *uuidMultiDBFixture) closeLogHandle(t *testing.T) {
	t.Helper()
	sqlDB, err := fixture.logDB.DB()
	require.NoError(t, err, "read the log handle's connection pool")
	require.NoError(t, sqlDB.Close(), "close the log handle's connection pool")
	fixture.logClosed = true
}

// seedLegacyRows inserts pre-migration rows into the database that authoritatively owns each.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func (fixture *uuidMultiDBFixture) seedLegacyRows(t *testing.T) {
	t.Helper()
	seedLegacyUUIDRows(t, fixture.primary)
	if !fixture.dedicatedLog {
		return
	}
	// seedLegacyUUIDRows wrote its logs row to the primary database, where split mode treats it
	// as a non-authoritative leftover that is never scanned. The authoritative row must exist in
	// the dedicated log database instead.
	require.NoError(t, fixture.logDB.Table("logs").Create(map[string]any{
		"id": 1, "user_id": 1, "channel_id": 1, "type": 1, "token_name": "default", "content": "legacy log",
	}).Error, "seed the authoritative log row")
}

// requireLegacyRowsReconciled asserts every owned and denormalized FK UUID from the legacy
// fixture is filled and agrees with its live owner.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func (fixture *uuidMultiDBFixture) requireLegacyRowsReconciled(t *testing.T) {
	t.Helper()

	var user User
	require.NoError(t, fixture.primary.First(&user, "id = ?", 1).Error)
	requireHyphenatedUUID(t, user.UUID)

	var child User
	require.NoError(t, fixture.primary.First(&child, "id = ?", 2).Error)
	requireHyphenatedUUID(t, child.UUID)
	require.NotNil(t, child.InviterUUID, "users.inviter_uuid must be resolved from the live owner")
	require.Equal(t, user.UUID, *child.InviterUUID)

	var channel Channel
	require.NoError(t, fixture.primary.First(&channel, "id = ?", 1).Error)
	requireHyphenatedUUID(t, channel.UUID)

	var token Token
	require.NoError(t, fixture.primary.First(&token, "id = ?", 1).Error)
	requireHyphenatedUUID(t, token.UUID)
	require.NotNil(t, token.UserUUID, "tokens.user_uuid must be resolved from the live owner")
	require.Equal(t, user.UUID, *token.UserUUID)

	var redemption Redemption
	require.NoError(t, fixture.primary.First(&redemption, "id = ?", 1).Error)
	requireHyphenatedUUID(t, redemption.UUID)
	require.NotNil(t, redemption.UserUUID, "redemptions.user_uuid must be resolved from the live owner")
	require.Equal(t, user.UUID, *redemption.UserUUID)

	// The log row is read through the authoritative log handle, which is the whole point of the
	// split matrix: in split mode this is the dedicated database, not the primary.
	var logRow Log
	require.NoError(t, fixture.logDB.First(&logRow, "id = ?", 1).Error)
	requireHyphenatedUUID(t, logRow.UUID)
	require.NotNil(t, logRow.UserUUID, "logs.user_uuid must be resolved from the primary owner")
	require.Equal(t, user.UUID, *logRow.UserUUID)
	require.NotNil(t, logRow.ChannelUUID, "logs.channel_uuid must be resolved from the primary owner")
	require.Equal(t, channel.UUID, *logRow.ChannelUUID)
	require.NotNil(t, logRow.TokenUUID, "logs.token_uuid must be resolved by composite token lookup")
	require.Equal(t, token.UUID, *logRow.TokenUUID)
}

// requireMarkers asserts markers exist only for a finalizer run, and only where they belong.
// Parameters:
//   - t: test handle used for assertions.
//   - finalized: whether the finalizer was enabled for the run under test.
//
// Return values: none.
func (fixture *uuidMultiDBFixture) requireMarkers(t *testing.T, finalized bool) {
	t.Helper()
	requireMarker(t, fixture.primary, externalUUIDPrimaryMigrationKey, finalized)
	if fixture.topology.mode == uuidTopologySplit {
		requireMarker(t, fixture.logDB, externalUUIDLogMigrationKey, finalized)
		return
	}
	requireMarker(t, fixture.primary, externalUUIDLogMigrationKey, false)
}

// uuidMultiDBFixtureUUID builds a deterministic canonical UUID value for a row id.
// Parameters:
//   - id: row id to derive from.
//
// Return values:
//   - string: canonical hyphenated UUID unique to the id.
func uuidMultiDBFixtureUUID(id int) string {
	return fmt.Sprintf("018f0000-0000-7000-8000-%012d", id)
}

// seedUUIDLiveUsers inserts users on a live backend with a controlled UUID distribution.
// Every map carries the same key set because GORM derives batch INSERT columns from the first
// map only; a nil uuid therefore becomes a real NULL rather than a dropped column.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - prefix: username prefix keeping the unique username index satisfied.
//   - total: number of users to insert.
//   - missingPerPass: users left NULL for the NULL candidate pass, and the same number left
//     empty for the empty-string candidate pass.
//
// Return values: none.
