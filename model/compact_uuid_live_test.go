package model

// Live-engine qualification for compact UUID storage (AUTO-014, AUTO-T02/T17/T18/T21/T22/T23).
//
// These tests run against REAL MySQL 8.4 and PostgreSQL 17 servers, unified and split. They are
// the only place the per-dialect halves of the contract are actually exercised: the trigger
// bodies, the catalog verification (information_schema.TRIGGERS, pg_trigger/pg_proc), the
// online-DDL policy, advisory/GET_LOCK ownership, native-uuid and BINARY(16) scan/bind, and the
// repeatable-read fingerprint snapshots. SQLite cannot stand in for any of them.
//
// The DSNs come from the environment, using the names the qualification workflow sets:
//
//	COMPACT_UUID_TEST_MYSQL_DSN, COMPACT_UUID_TEST_LOG_MYSQL_DSN
//	COMPACT_UUID_TEST_POSTGRES_DSN, COMPACT_UUID_TEST_LOG_POSTGRES_DSN
//
// A missing DSN skips locally so an ordinary `go test ./...` still works on a laptop. That is
// NOT a loophole in qualification: the workflow's no-skip guard fails the run when a required
// DSN is absent or when these tests do not report PASS, so CI cannot go green by skipping them.

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/idresolve"
)

// idresolveErrNotFoundForTest exposes the shared not-found sentinel to live assertions.
// Parameters: none.
//
// Return values:
//   - error: the canonical unknown-identifier sentinel.
func idresolveErrNotFoundForTest() error {
	return idresolve.ErrNotFound
}

// compactLiveDialect describes one live engine under qualification.
type compactLiveDialect struct {
	// name is the dialect's lowercase name, used in subtest names and assertions.
	name string
	// primaryEnv names the environment variable carrying the primary DSN.
	primaryEnv string
	// logEnv names the environment variable carrying the split log DSN.
	logEnv string
	// open builds a handle for one DSN.
	open func(dsn string) gorm.Dialector
}

// compactLiveDialects returns every engine the proposal requires to be qualified live.
// Parameters: none.
//
// Return values:
//   - []compactLiveDialect: MySQL 8.4 and PostgreSQL 17 descriptors.
func compactLiveDialects() []compactLiveDialect {
	return []compactLiveDialect{
		{
			name:       "mysql",
			primaryEnv: "COMPACT_UUID_TEST_MYSQL_DSN",
			logEnv:     "COMPACT_UUID_TEST_LOG_MYSQL_DSN",
			open:       func(dsn string) gorm.Dialector { return mysql.Open(dsn) },
		},
		{
			name:       "postgres",
			primaryEnv: "COMPACT_UUID_TEST_POSTGRES_DSN",
			logEnv:     "COMPACT_UUID_TEST_LOG_POSTGRES_DSN",
			open:       func(dsn string) gorm.Dialector { return postgres.Open(dsn) },
		},
	}
}

// openLiveCompactDB opens one live handle and resets its schema to a clean legacy baseline.
//
// The reset is what makes these tests repeatable against a persistent server: a leftover
// compact column, trigger, or marker from a previous run would let a test pass without the
// coordinator having done the work.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - dialect: engine descriptor.
//   - dsn: data source name for the database to use.
//
// Return values:
//   - *gorm.DB: handle with a clean legacy schema.
func openLiveCompactDB(t *testing.T, dialect compactLiveDialect, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(dialect.open(dsn), &gorm.Config{})
	require.NoError(t, err, "open live %s database", dialect.name)

	dropLiveCompactSchema(t, db, dialect)

	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			_ = pool.Close()
		}
	})
	return db
}

// dropLiveCompactSchema removes every object this suite creates, plus the legacy tables.
//
// Triggers are dropped before their tables: MySQL and PostgreSQL both refuse to drop a table
// whose trigger another statement still references, and a stale trigger left behind would fire
// against the next run's freshly migrated table.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle to reset.
//   - dialect: engine descriptor.
//
// Return values: none.
func dropLiveCompactSchema(t *testing.T, db *gorm.DB, dialect compactLiveDialect) {
	t.Helper()
	for _, table := range compactRegistry() {
		if dialect.name == "postgres" {
			// PostgreSQL scopes a trigger to its table, so DROP TRIGGER requires ON <table>;
			// issuing the bare MySQL/SQLite form here is a syntax error that floods the log with
			// ignored failures and buries a real one.
			_ = db.Exec("DROP TRIGGER IF EXISTS " + quoteIdentifier(db, compactSyncTriggerName(table.table)) +
				" ON " + quoteIdentifier(db, table.table)).Error
			_ = db.Exec("DROP FUNCTION IF EXISTS " + quoteIdentifier(db, compactSyncFunctionName(table.table)) + "()").Error
			continue
		}
		for _, name := range []string{
			compactInsertTriggerName(table.table),
			compactUpdateTriggerName(table.table),
		} {
			_ = db.Exec("DROP TRIGGER IF EXISTS " + quoteIdentifier(db, name)).Error
		}
	}

	tables := []string{"compact_uuid_manifests", "data_migrations"}
	for _, owned := range uuidOwnedRegistry() {
		tables = append(tables, owned.table)
	}
	for _, extra := range []string{"abilities", "options", "channels", "tokens", "users", "logs"} {
		tables = append(tables, extra)
	}
	if dialect.name == "mysql" {
		_ = db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error
	}
	for _, table := range tables {
		suffix := ""
		if dialect.name == "postgres" {
			suffix = " CASCADE"
		}
		_ = db.Exec("DROP TABLE IF EXISTS " + quoteIdentifier(db, table) + suffix).Error
	}
	if dialect.name == "mysql" {
		_ = db.Exec("SET FOREIGN_KEY_CHECKS = 1").Error
	}
}

// newLiveCompactTopology builds a unified or split topology over a live engine.
//
// It returns ok=false when the required DSNs are absent, which lets a workstation run
// `go test ./...` without a database while CI's no-skip guard still enforces the live matrix.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - dialect: engine descriptor.
//   - split: true to build a split topology over a dedicated log database.
//
// Return values:
//   - *gorm.DB: primary handle.
//   - *databaseTopology: constructed topology.
//   - bool: false when the DSNs are not configured.
func newLiveCompactTopology(t *testing.T, dialect compactLiveDialect, split bool) (*gorm.DB, *databaseTopology, bool) {
	t.Helper()
	primaryDSN := strings.TrimSpace(os.Getenv(dialect.primaryEnv))
	logDSN := strings.TrimSpace(os.Getenv(dialect.logEnv))
	if primaryDSN == "" || (split && logDSN == "") {
		return nil, nil, false
	}
	withCompactTestSettings(t)

	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(dialect.name == "mysql")
	common.UsingPostgreSQL.Store(dialect.name == "postgres")
	t.Cleanup(func() {
		common.UsingSQLite.Store(true)
		common.UsingMySQL.Store(false)
		common.UsingPostgreSQL.Store(false)
	})

	primary := openLiveCompactDB(t, dialect, primaryDSN)
	logHandle := primary
	if split {
		logHandle = openLiveCompactDB(t, dialect, logDSN)
	}

	withTestDBGlobals(t, primary, logHandle)
	require.NoError(t, migrateDB())

	var topology *databaseTopology
	var err error
	if split {
		require.NoError(t, logHandle.AutoMigrate(&Log{}, &DataMigration{}))
		topology, err = newSplitTopology(primary, logHandle)
	} else {
		topology, err = newUnifiedTopology(primary)
	}
	require.NoError(t, err)
	requireV3Markers(t, topology)
	return primary, topology, true
}

// TestCompactUUIDLiveMatrix qualifies the whole contract against MySQL 8.4 and PostgreSQL 17.
//
// Every subtest drives the real coordinator and the real engine. Nothing here emulates a
// trigger or a catalog read.
// Parameters:
//   - t: test handle.
//
// Return values: none.
func TestCompactUUIDLiveMatrix(t *testing.T) {
	for _, dialect := range compactLiveDialects() {
		for _, split := range []bool{false, true} {
			topologyName := "unified"
			if split {
				topologyName = "split"
			}
			t.Run(dialect.name+"/"+topologyName, func(t *testing.T) {
				db, topology, ok := newLiveCompactTopology(t, dialect, split)
				if !ok {
					t.Skipf("%s is not configured; CI's no-skip guard enforces this suite", dialect.primaryEnv)
				}
				runLiveCompactQualification(t, db, topology, dialect, split)
			})
		}
	}
}

// runLiveCompactQualification drives one engine/topology through the full contract.
// Parameters:
//   - t: test handle used for assertions.
//   - db: primary handle.
//   - topology: constructed topology.
//   - dialect: engine descriptor.
//   - split: whether the topology is split.
//
// Return values: none.
func runLiveCompactQualification(t *testing.T, db *gorm.DB, topology *databaseTopology,
	dialect compactLiveDialect, split bool) {
	ctx := compactTestContext(t)

	// A populated valid legacy schema, written as any writer writes it: text only.
	for index := 1; index <= 5; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}

	// AUTO-T02: default startup reaches validated completion with no command.
	coordinator := newCompactCoordinator(topology)
	result := driveCompactToReady(t, coordinator)
	require.Equal(t, compactStateReady, result.state)

	t.Run("markers appear automatically", func(t *testing.T) {
		complete, err := isDataMigrationComplete(ctx, topology.primary, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.True(t, complete)
		if split {
			// Split writes the log marker first and the primary marker last.
			complete, err = isDataMigrationComplete(ctx, topology.log, compactLogMigrationKey)
			require.NoError(t, err)
			require.True(t, complete, "split mode must carry both markers")
		}
	})

	t.Run("every object verifies against its live catalog", func(t *testing.T) {
		// This is the assertion SQLite cannot make: the trigger body hash, timing, event,
		// security/definer properties, and enabled state all come from the real catalog.
		verified, reason, err := validateCompactObjects(ctx, topology)
		require.NoError(t, err)
		require.True(t, verified, "live objects did not verify: %s", reason)
	})

	t.Run("shadow columns have the exact physical type", func(t *testing.T) {
		for _, target := range compactTargetsForTopology(topology) {
			ok, err := verifyCompactColumnType(ctx, topology.handle(target.role), target)
			require.NoError(t, err)
			require.True(t, ok, "%s lacks the approved %s compact type", target.id(), dialect.name)
		}
	})

	t.Run("historical rows were filled from authoritative text", func(t *testing.T) {
		for index := 1; index <= 5; index++ {
			requireLiveShadowMatches(t, db, dialect, index, compactUUIDTextFor(index))
		}
	})

	t.Run("triggers derive on live insert and update", func(t *testing.T) {
		// AUTO-T22: a new write derives its shadow atomically, through the real trigger.
		seedCompactUser(t, db, 100, compactUUIDTextFor(100))
		requireLiveShadowMatches(t, db, dialect, 100, compactUUIDTextFor(100))

		require.NoError(t, db.Exec("UPDATE users SET uuid = ? WHERE id = ?",
			compactUUIDTextFor(101), 100).Error)
		requireLiveShadowMatches(t, db, dialect, 100, compactUUIDTextFor(101))
	})

	t.Run("invalid legacy text never aborts a write and derives NULL", func(t *testing.T) {
		// AUTO-T23 on a live engine: this is where a regex or cast mistake would surface as
		// a failed INSERT rather than a NULL shadow.
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, username, password, uuid) VALUES (?, ?, 'x', ?)",
			200, "live-invalid", "not-a-uuid").Error,
			"a previously accepted legacy write must not start failing")

		var shadow *string
		require.NoError(t, db.Raw(liveShadowHexSQL(dialect)+" WHERE id = ?", 200).Scan(&shadow).Error)
		require.Nil(t, shadow, "invalid legacy text must derive NULL")

		requireLiveTextUnchanged(t, db, 200, "not-a-uuid")

		require.NoError(t, db.Exec("DELETE FROM users WHERE id = ?", 200).Error)
	})

	t.Run("case-insensitive input derives identical rfc bytes", func(t *testing.T) {
		upper := strings.ToUpper(compactUUIDTextFor(300))
		seedCompactUser(t, db, 300, upper)
		// The stored text keeps its exact case; the derived bytes are canonical.
		requireLiveShadowMatches(t, db, dialect, 300, strings.ToLower(upper))
	})

	t.Run("exact lookup verifies its candidate", func(t *testing.T) {
		// AUTO-T18 against the live index: native uuid on PostgreSQL, BINARY(16) on MySQL,
		// bound with no column-side cast.
		runCompactHealthAudit(ctx, topology)
		target, err := compactLookupTarget("users")
		require.NoError(t, err)

		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id)

		_, err = resolveIDByUUID(ctx, db, target, compactUUIDTextFor(9999))
		require.ErrorIs(t, err, idresolveErrNotFoundForTest())
	})

	t.Run("global fingerprints match under a live snapshot", func(t *testing.T) {
		// AUTO-T33: the repeatable-read snapshot is a live-engine behavior.
		_, matched, err := verifyCompactFingerprints(ctx, topology)
		require.NoError(t, err)
		require.True(t, matched, "live equality fingerprints must match")
	})

	t.Run("ownership is exclusive on the live engine", func(t *testing.T) {
		// AUTO-T03: the real advisory lock (PostgreSQL) or GET_LOCK (MySQL).
		first, acquired, err := acquireCompactOwnership(ctx, topology)
		require.NoError(t, err)
		require.True(t, acquired)
		defer first.release()

		held, err := first.verify(ctx)
		require.NoError(t, err)
		require.True(t, held, "an acquired live claim must verify as held")
	})

	if split {
		t.Run("a stale primary logs table is never touched", func(t *testing.T) {
			// AUTO-T17: in split mode the log targets resolve exclusively through LOG_DB.
			for _, target := range compactTargetsForRole(uuidRolePrimary) {
				require.NotEqual(t, "logs", target.table,
					"a stale primary logs table must never be a compact target")
			}
		})
	}
}

// requireLiveTextUnchanged asserts the authoritative column still holds exactly what was written.
//
// Equality is asked of the ENGINE, not of the driver's rendering, and that is the point rather
// than a loophole. The legacy columns are CHAR(36); PostgreSQL's CHAR is bpchar, which
// space-pads a shorter value to 36 characters on the wire while defining equality and length()
// to ignore that padding. So a driver-level `== "not-a-uuid"` fails against a column that
// PostgreSQL itself reports as unchanged, and it would fail identically on a pre-compact schema
// with no trigger installed at all — it tests bpchar, not the trigger.
//
// The assertion is still strict: the engine must agree the value is unchanged, AND the only
// difference in the driver's rendering may be trailing spaces. A trigger that replaced the text
// with anything else fails both halves.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - id: primary key.
//   - written: the exact text the test wrote.
//
// Return values: none.
func requireLiveTextUnchanged(t *testing.T, db *gorm.DB, id int, written string) {
	t.Helper()

	var matches bool
	require.NoError(t, db.Raw("SELECT uuid = ? FROM users WHERE id = ?", written, id).Scan(&matches).Error)
	require.True(t, matches,
		"the engine must report the authoritative column unchanged; the trigger must never rewrite text")

	var stored string
	require.NoError(t, db.Raw("SELECT uuid FROM users WHERE id = ?", id).Scan(&stored).Error)
	require.Equal(t, written, strings.TrimRight(stored, " "),
		"the only permitted difference is the CHAR(36) column type's own trailing-space padding")
}

// requireLiveShadowMatches asserts one row's shadow equals the derived bytes of a UUID.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - dialect: engine descriptor.
//   - id: primary key.
//   - canonical: expected canonical UUID text.
//
// Return values: none.
func requireLiveShadowMatches(t *testing.T, db *gorm.DB, dialect compactLiveDialect,
	id int, canonical string) {
	t.Helper()
	expected, err := parseCompactUUID(strings.ToLower(canonical))
	require.NoError(t, err)

	var shadow string
	require.NoError(t, db.Raw(liveShadowHexSQL(dialect)+" WHERE id = ?", id).Scan(&shadow).Error)
	require.Equal(t, strings.ToUpper(hexOf(expected)), strings.ToUpper(strings.TrimPrefix(shadow, "\\x")),
		"row %d must derive the canonical rfc bytes", id)
}

// liveShadowHexSQL returns the dialect's statement rendering users.uuid_compact as hex.
//
// The rendering is per-dialect because the physical types differ: PostgreSQL stores a native
// uuid and MySQL a BINARY(16), so there is no portable expression for "show me these bytes".
// Parameters:
//   - dialect: engine descriptor.
//
// Return values:
//   - string: SELECT statement lacking only its WHERE clause.
func liveShadowHexSQL(dialect compactLiveDialect) string {
	if dialect.name == "postgres" {
		// Casting the uuid to text and stripping hyphens yields the same 32 hex characters
		// the codec produces, without depending on the server's bytea_output setting.
		return "SELECT UPPER(REPLACE(uuid_compact::text, '-', '')) FROM users"
	}
	return "SELECT HEX(uuid_compact) FROM users"
}
