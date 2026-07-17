package model

// Docker/dump infrastructure and the three supported restore modes for AUTO-T28.
//
// This is the second half of the AUTO-T28 suite; compact_uuid_replication_test.go owns the
// replication qualification and the TestCompactUUIDReplicationAndRestore entry point. The two
// files split because one file carrying both exceeds the project's Go file-length limit.
//
// Proposal section 6.3: "Supported restore cases are pre-migration full dump to a fresh legacy
// schema, completed full dump to a provisioned fresh server, and approved data-only/
// trigger-disabled restore followed by automatic repair. Positional data-only restore into an
// expanded schema is unsupported." Only the three approved shapes are exercised.
//
// Shelling out to docker, pg_dump/psql, and mysqldump/mysql builds the real artifacts under
// test. It is never used to compute an expected value, which would be the emulation the
// proposal forbids.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// =============================================================================
// INFRASTRUCTURE PRIMITIVES
// =============================================================================

// compactReplShell runs one infrastructure command through a shell, failing the test on error.
// Parameters:
//   - t: test handle used for assertions.
//   - script: shell script to execute.
//
// Return values:
//   - string: combined output.
func compactReplShell(t *testing.T, script string) string {
	t.Helper()
	output, err := compactReplTryShell(script)
	require.NoError(t, err, "infrastructure command failed:\n%s\n%s", script, output)
	return output
}

// compactReplTryShell runs one shell command and reports its output and error.
// Parameters:
//   - script: shell script to execute.
//
// Return values:
//   - string: combined output.
//   - error: wrapped error when the command exits non-zero.
func compactReplTryShell(script string) (string, error) {
	raw, err := exec.Command("bash", "-c", script).CombinedOutput()
	if err != nil {
		return string(raw), errors.Wrapf(err, "run shell script")
	}
	return string(raw), nil
}

// compactReplContainer resolves a primary's container name from the environment.
// Parameters:
//   - key: environment variable naming the container.
//   - fallback: default container name.
//
// Return values:
//   - string: container name to address.
func compactReplContainer(key string, fallback string) string {
	if name := strings.TrimSpace(os.Getenv(key)); name != "" {
		return name
	}
	return fallback
}

// compactReplDockerIP returns a container's bridge address.
//
// Replicas reach their primary over the docker bridge because a published host port is not
// routable from inside another container.
// Parameters:
//   - t: test handle used for assertions.
//   - container: container name.
//
// Return values:
//   - string: bridge IP address.
func compactReplDockerIP(t *testing.T, container string) string {
	t.Helper()
	address := strings.TrimSpace(compactReplShell(t,
		"docker inspect "+container+" --format '{{.NetworkSettings.Networks.bridge.IPAddress}}'"))
	require.NotEmpty(t, address, "container %s has no bridge address", container)
	return address
}

// compactReplStartContainer replaces any stale container of the same name and starts a new one.
//
// Cleanup removes only containers this suite created; the shared primaries are never touched.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - name: container name to create.
//   - runArgs: `docker run` arguments after the detach flag.
//
// Return values: none.
func compactReplStartContainer(t *testing.T, name string, runArgs string) {
	t.Helper()
	_, _ = compactReplTryShell("docker rm -f " + name)
	t.Cleanup(func() { _, _ = compactReplTryShell("docker rm -f " + name) })
	compactReplShell(t, "docker run -d --name "+name+" "+runArgs)
}

// compactReplWaitUntil polls a condition until it holds or the bound elapses.
// Parameters:
//   - t: test handle used for assertions.
//   - what: description used in the timeout message.
//   - condition: predicate to poll.
//
// Return values: none.
func compactReplWaitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(compactReplWait)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", compactReplWait, what)
}

// compactReplReadFile returns a dump file's contents for a fixture sanity assertion.
// Parameters:
//   - t: test handle used for assertions.
//   - path: file to read.
//
// Return values:
//   - string: file contents.
func compactReplReadFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "read dump %s", path)
	return string(raw)
}

// =============================================================================
// DSN REWRITING
// =============================================================================

// compactReplPGParts is one PostgreSQL URL DSN decomposed so it can be re-aimed.
type compactReplPGParts struct {
	// user is the login role.
	user string
	// password is the login secret.
	password string
	// host is the TCP host.
	host string
	// port is the TCP port.
	port string
	// database is the database name.
	database string
}

// compactReplParsePG decomposes the URL-form PostgreSQL DSN the workflow configures.
// Parameters:
//   - t: test handle used for assertions.
//   - dsn: postgres:// URL DSN.
//
// Return values:
//   - compactReplPGParts: decomposed connection settings.
func compactReplParsePG(t *testing.T, dsn string) compactReplPGParts {
	t.Helper()

	// Both DSN forms are accepted because both are legitimately in play, and rejecting one made
	// this suite fail on a correct configuration. COMPACT_UUID_TEST_POSTGRES_DSN is consumed by
	// gorm.io/driver/postgres, which takes the key/value form ("host=... port=... dbname=..."),
	// and that is what the rest of the live matrix uses. The URL form is what psql, pg_basebackup,
	// and one-api's own SQL_DSN take. Parsing whichever arrives keeps the workflow to a single
	// variable per engine rather than two that could silently drift apart.
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		require.Contains(t, dsn, "host=",
			"a PostgreSQL DSN must be either the postgres:// URL form or the key/value form, got %q", dsn)
		dsn = oldBinaryDSN(dsn)
	}

	credentials, remainder, found := strings.Cut(dsn[strings.Index(dsn, "//")+2:], "@")
	require.True(t, found, "PostgreSQL DSN must carry credentials")
	user, password, _ := strings.Cut(credentials, ":")
	authority, path, _ := strings.Cut(remainder, "/")
	host, port, _ := strings.Cut(authority, ":")
	database, _, _ := strings.Cut(path, "?")
	return compactReplPGParts{user: user, password: password, host: host, port: port, database: database}
}

// at returns a copy of the parts aimed at another port and database.
// Parameters:
//   - port: host port to address.
//   - database: database name to address.
//
// Return values:
//   - compactReplPGParts: rewritten settings.
func (parts compactReplPGParts) at(port string, database string) compactReplPGParts {
	parts.port = port
	parts.database = database
	return parts
}

// dsn renders the parts back into the URL DSN form gorm and psql both accept.
// Parameters: none.
//
// Return values:
//   - string: postgres:// URL DSN matching the workflow's form.
func (parts compactReplPGParts) dsn() string {
	return "postgres://" + parts.user + ":" + parts.password + "@" + parts.host + ":" + parts.port +
		"/" + parts.database + "?sslmode=disable"
}

// psql renders the environment prefix libpq clients read for connection settings.
// Parameters:
//   - database: database to connect to.
//
// Return values:
//   - string: PG* environment assignments to prefix a client command with.
func (parts compactReplPGParts) psql(database string) string {
	return "PGPASSWORD=" + parts.password + " PGHOST=" + parts.host + " PGPORT=" + parts.port +
		" PGUSER=" + parts.user + " PGDATABASE=" + database
}

// compactReplMySQLDSN rewrites a go-sql-driver DSN's port and database.
// Parameters:
//   - t: test handle used for assertions.
//   - dsn: workflow-form MySQL DSN.
//   - port: host port to address, or empty to keep the DSN's own port.
//   - database: database name to address.
//
// Return values:
//   - string: rewritten DSN.
func compactReplMySQLDSN(t *testing.T, dsn string, port string, database string) string {
	t.Helper()
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err, "parse MySQL DSN %q", dsn)
	host, current, _ := strings.Cut(config.Addr, ":")
	if port == "" {
		port = current
	}
	config.Addr = host + ":" + port
	config.DBName = database
	return config.FormatDSN()
}

// compactReplMySQLClient renders a warning-free `docker exec ... mysql` command prefix.
//
// MYSQL_PWD rather than -p, because the client writes "Using a password on the command line
// interface can be insecure" to stderr and this suite parses GTID sets out of that same output.
// Parameters:
//   - container: container running the server.
//   - config: parsed DSN carrying the credentials.
//
// Return values:
//   - string: prefix ready for an `-e "..."` argument.
func compactReplMySQLClient(container string, config *mysqldriver.Config) string {
	return "docker exec -e MYSQL_PWD=" + config.Passwd + " " + container + " mysql -u" + config.User + " -N"
}

// =============================================================================
// RESTORE MODES
// =============================================================================

// compactReplRestore is one engine's dump/restore tooling for the three supported modes.
type compactReplRestore struct {
	// dialect is the engine descriptor.
	dialect compactLiveDialect
	// syncTriggerName is the object name a completed dump must be shown to carry.
	syncTriggerName string
	// dsnFor renders a DSN for one database on the primary server.
	dsnFor func(database string) string
	// freshServerDSN provisions a brand-new empty server and returns a DSN for one database.
	freshServerDSN func(t *testing.T, database string) string
	// resetDatabase drops and recreates one empty database on the primary server.
	resetDatabase func(t *testing.T, database string)
	// dumpFull writes a complete schema+data+trigger+marker dump.
	dumpFull func(t *testing.T, database string, file string)
	// dumpUsersData writes a named-column, trigger-disabled data-only dump of users.
	dumpUsersData func(t *testing.T, database string, file string)
	// restoreInto applies a dump file to the database its DSN addresses.
	restoreInto func(t *testing.T, dsn string, file string)
	// prepareDataOnlyTarget clears users and suppresses the sync trigger on the target.
	prepareDataOnlyTarget func(t *testing.T, db *gorm.DB)
}

// compactReplPGRestore builds the PostgreSQL restore tooling.
// Parameters:
//   - parts: primary connection settings.
//
// Return values:
//   - compactReplRestore: PostgreSQL tooling.
func compactReplPGRestore(parts compactReplPGParts) compactReplRestore {
	fresh := parts.at(compactReplPGRestorePort, parts.database)
	return compactReplRestore{
		dialect:         compactLiveDialects()[1],
		syncTriggerName: compactSyncTriggerName("users"),
		dsnFor:          func(database string) string { return parts.at(parts.port, database).dsn() },
		freshServerDSN: func(t *testing.T, database string) string {
			compactReplStartContainer(t, "cuuid-pg-restore",
				"-e POSTGRES_PASSWORD="+parts.password+" -p "+compactReplPGRestorePort+":5432 postgres:17")
			compactReplWaitUntil(t, "the fresh PostgreSQL server to accept connections", func() bool {
				_, err := compactReplTryShell(fresh.psql("postgres") + " psql -Atc 'SELECT 1'")
				return err == nil
			})
			compactReplShell(t, fresh.psql("postgres")+" psql -Atc 'CREATE DATABASE "+database+"'")
			return fresh.at(compactReplPGRestorePort, database).dsn()
		},
		resetDatabase: func(t *testing.T, database string) {
			compactReplShell(t, parts.psql("postgres")+" psql -At -c 'DROP DATABASE IF EXISTS "+
				database+" WITH (FORCE)' -c 'CREATE DATABASE "+database+"'")
		},
		dumpFull: func(t *testing.T, database string, file string) {
			compactReplShell(t, parts.psql(database)+" pg_dump --no-owner --no-privileges -f "+file)
		},
		dumpUsersData: func(t *testing.T, database string, file string) {
			// pg_dump's data-only output names every column it emits, which is exactly what
			// separates the approved restore from the unsupported positional one.
			compactReplShell(t, parts.psql(database)+
				" pg_dump --data-only --disable-triggers --table=public.users -f "+file)
		},
		restoreInto: func(t *testing.T, dsn string, file string) {
			target := compactReplParsePG(t, dsn)
			compactReplShell(t, target.psql(target.database)+" psql -v ON_ERROR_STOP=1 -q -f "+file)
		},
		prepareDataOnlyTarget: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Exec("TRUNCATE users CASCADE").Error)
		},
	}
}

// compactReplMySQLRestore builds the MySQL restore tooling.
// Parameters:
//   - t: test handle used for assertions.
//   - dsn: primary DSN.
//
// Return values:
//   - compactReplRestore: MySQL tooling.
func compactReplMySQLRestore(t *testing.T, dsn string) compactReplRestore {
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	container := compactReplContainer(compactReplMySQLContainerEnv, "cuuid-mysql")
	client := compactReplMySQLClient(container, config)
	dump := "docker exec -e MYSQL_PWD=" + config.Passwd + " " + container + " mysqldump -u" + config.User
	_, primaryPort, _ := strings.Cut(config.Addr, ":")

	return compactReplRestore{
		dialect:         compactLiveDialects()[0],
		syncTriggerName: compactInsertTriggerName("users"),
		dsnFor:          func(database string) string { return compactReplMySQLDSN(t, dsn, "", database) },
		freshServerDSN: func(t *testing.T, database string) string {
			compactReplStartContainer(t, "cuuid-mysql-restore",
				"-e MYSQL_ROOT_PASSWORD="+config.Passwd+" -p "+compactReplMySQLRestorePort+":3306"+
					" mysql:8.4 --server-id=3")
			restoreClient := compactReplMySQLClient("cuuid-mysql-restore", config)
			compactReplWaitUntil(t, "the fresh MySQL server to finish initializing", func() bool {
				_, err := compactReplTryShell(restoreClient + " -e 'SELECT 1'")
				return err == nil
			})
			compactReplShell(t, restoreClient+" -e \"CREATE DATABASE "+database+" CHARACTER SET utf8mb4\"")
			return compactReplMySQLDSN(t, dsn, compactReplMySQLRestorePort, database)
		},
		resetDatabase: func(t *testing.T, database string) {
			compactReplShell(t, client+" -e \"DROP DATABASE IF EXISTS "+database+
				"; CREATE DATABASE "+database+" CHARACTER SET utf8mb4\"")
		},
		dumpFull: func(t *testing.T, database string, file string) {
			// --set-gtid-purged=OFF because this dump is restored into servers with their own
			// GTID history; --triggers is what carries the sync contract across the restore.
			compactReplShell(t, dump+" --single-transaction --triggers --routines --events"+
				" --set-gtid-purged=OFF "+database+" > "+file)
		},
		dumpUsersData: func(t *testing.T, database string, file string) {
			// --complete-insert is mandatory: mysqldump's DEFAULT data-only output is positional
			// (`INSERT INTO users VALUES (...)`), which is precisely the restore shape section
			// 6.3 declares unsupported against an expanded schema.
			compactReplShell(t, dump+" --single-transaction --no-create-info --skip-triggers"+
				" --complete-insert --set-gtid-purged=OFF "+database+" users > "+file)
		},
		restoreInto: func(t *testing.T, dsn string, file string) {
			target, err := mysqldriver.ParseDSN(dsn)
			require.NoError(t, err)
			name := "cuuid-mysql-restore"
			if strings.HasSuffix(target.Addr, ":"+primaryPort) {
				name = container
			}
			compactReplShell(t, "docker exec -i -e MYSQL_PWD="+config.Passwd+" "+name+
				" mysql -u"+config.User+" "+target.DBName+" < "+file)
		},
		prepareDataOnlyTarget: func(t *testing.T, db *gorm.DB) {
			// MySQL has no DISABLE TRIGGER, so the trigger-disabled restore shape is literally a
			// restore into a schema whose sync triggers are absent.
			dropCompactSyncTriggers(t, db, "users")
			require.NoError(t, db.Exec("TRUNCATE users").Error)
		},
	}
}

// compactReplRunRestores qualifies the three supported restore modes for one engine.
//
// The fixtures are real artifacts of the real coordinator: a pre-migration dump taken before any
// compact object exists, and a completed dump taken after it reaches ready.
// Parameters:
//   - t: test handle used for assertions.
//   - tooling: engine restore tooling.
//   - source: database name used to build the fixtures.
//
// Return values: none.
func compactReplRunRestores(t *testing.T, tooling compactReplRestore, source string) {
	compactReplUseDialect(t, tooling.dialect)
	directory := t.TempDir()
	legacyDump := filepath.Join(directory, "legacy.sql")
	legacyData := filepath.Join(directory, "legacy-users.sql")
	completeDump := filepath.Join(directory, "complete.sql")

	tooling.resetDatabase(t, source)
	sourceDB, sourceTopology := compactReplOpenTopology(t, tooling.dialect, tooling.dsnFor(source), true)
	requireV3Markers(t, sourceTopology)
	compactReplSeedUsers(t, sourceDB)
	tooling.dumpFull(t, source, legacyDump)
	tooling.dumpUsersData(t, source, legacyData)
	require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(sourceTopology)).state)
	tooling.dumpFull(t, source, completeDump)
	require.NotContains(t, compactReplReadFile(t, legacyDump), "uuid_compact",
		"the pre-migration dump must genuinely predate expansion")
	require.Contains(t, compactReplReadFile(t, completeDump), tooling.syncTriggerName,
		"the completed dump must genuinely carry the sync trigger")

	t.Run("pre-migration dump into a fresh legacy schema completes automatically", func(t *testing.T) {
		target := source + "_pre"
		tooling.resetDatabase(t, target)
		tooling.restoreInto(t, tooling.dsnFor(target), legacyDump)

		db, topology := compactReplOpenTopology(t, tooling.dialect, tooling.dsnFor(target), true)
		require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)
		for index := 1; index <= compactReplRows; index++ {
			requireLiveShadowMatches(t, db, tooling.dialect, index, compactUUIDTextFor(index))
		}
	})

	t.Run("completed dump into a fresh provisioned server keeps triggers and markers", func(t *testing.T) {
		dsn := tooling.freshServerDSN(t, source)
		tooling.restoreInto(t, dsn, completeDump)

		db, topology := compactReplOpenTopology(t, tooling.dialect, dsn, true)
		ctx := compactTestContext(t)
		complete, err := isDataMigrationComplete(ctx, topology.primary, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.True(t, complete, "the completion marker must survive dump/restore")

		verified, reason, err := validateCompactObjects(ctx, topology)
		require.NoError(t, err)
		require.True(t, verified, "restored objects did not verify on the fresh server: %s", reason)

		// Surviving in the catalog is not the same as still working, so the restored trigger has
		// to actually derive a value on the fresh server.
		seedCompactUser(t, db, 800, compactUUIDTextFor(800))
		requireLiveShadowMatches(t, db, tooling.dialect, 800, compactUUIDTextFor(800))

		// "Without redoing the migration" is asserted by the state the FIRST cycle reports. A
		// restore that lost a column, trigger, or index would report expanding; one that lost the
		// shadows would report backfilling. Reaching validating means neither happened. Ready then
		// takes one more cycle by design: a freshly started process must observe two clean passes
		// in its own epoch before it trusts a marker, which is exactly what a restore should face.
		coordinator := newCompactCoordinator(topology)
		before := readMarkerTimestamp(t, db, compactPrimaryMigrationKey)
		first := runCompactCycleForTest(t, coordinator)
		require.Equal(t, compactStateValidating, first.state,
			"a restored complete database must go straight to validation: %s", first.reason)
		require.Zero(t, first.updated, "a restored complete database must need no shadow rewrite")
		require.Equal(t, compactStateReady, driveCompactToReady(t, coordinator).state,
			"a restored complete database must reach ready with no command")
		require.Equal(t, before, readMarkerTimestamp(t, db, compactPrimaryMigrationKey),
			"restore must not rewrite the completion marker's timestamp")
	})

	t.Run("trigger-disabled data-only restore is repaired automatically", func(t *testing.T) {
		target := source + "_data"
		tooling.resetDatabase(t, target)
		tooling.restoreInto(t, tooling.dsnFor(target), completeDump)

		db, topology := compactReplOpenTopology(t, tooling.dialect, tooling.dsnFor(target), false)
		before := readMarkerTimestamp(t, db, compactPrimaryMigrationKey)
		tooling.prepareDataOnlyTarget(t, db)
		tooling.restoreInto(t, tooling.dsnFor(target), legacyData)

		// If the shadows were already derived, the restore did not really bypass the trigger and
		// the repair below would prove nothing.
		rows := compactReplReadShadows(t, db, tooling.dialect)
		require.Len(t, rows, compactReplRows, "the data-only restore must have landed its rows")
		for _, row := range rows {
			require.Empty(t, row.Shadow,
				"a trigger-disabled restore must land rows with a NULL shadow (row %d)", row.ID)
		}

		require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state,
			"automatic repair must recover a trigger-disabled restore with no command")
		for index := 1; index <= compactReplRows; index++ {
			requireLiveShadowMatches(t, db, tooling.dialect, index, compactUUIDTextFor(index))
		}
		require.Equal(t, before, readMarkerTimestamp(t, db, compactPrimaryMigrationKey),
			"repair must not rewrite the completion marker's timestamp")
	})
}
