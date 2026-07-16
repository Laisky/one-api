package model

// Live replication and restore qualification for compact UUID storage (AUTO-T28).
//
// Proposal section 6.3 makes claims only a real engine pair can answer:
//
//   - "PostgreSQL physical streaming replication and MySQL row-based replication are required
//     modes." Physical streaming ships bytes, so a standby's shadows must be byte-identical;
//     MySQL ROW replication ships post-trigger row images, which is a different mechanism with
//     a different failure mode, so both are qualified separately.
//   - "Compact reads on a replica remain disabled until a replica-local equality audit passes."
//   - "Supported restore cases are pre-migration full dump to a fresh legacy schema, completed
//     full dump to a provisioned fresh server, and approved data-only/trigger-disabled restore
//     followed by automatic repair."
//
// This file owns the entry point and the replication half; compact_uuid_replication_restore_test.go
// owns the docker/dump infrastructure and the three restore modes. They split because one file
// carrying both exceeds the project's Go file-length limit.
//
// Nothing below emulates a trigger, a catalog read, or a replication apply. The suite builds a
// genuine pg_basebackup streaming standby and a genuine GTID row-based MySQL replica out of
// docker, drives the REAL coordinator on the primary, and then asks the replica's own engine
// what it holds. Shelling out to docker is infrastructure setup, never expected-value computation.
//
// Gated on COMPACT_UUID_TEST_REPLICATION=1 plus the live DSNs, so an ordinary `go test ./...`
// on a laptop still passes; CI's no-skip guard is what enforces the matrix.

import (
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

const (
	// compactReplEnableEnv gates this suite; the containers it builds are too heavy by default.
	compactReplEnableEnv = "COMPACT_UUID_TEST_REPLICATION"
	// compactReplPGContainerEnv names the docker container running the PostgreSQL primary.
	compactReplPGContainerEnv = "COMPACT_UUID_TEST_PG_CONTAINER"
	// compactReplMySQLContainerEnv names the docker container running the MySQL primary.
	compactReplMySQLContainerEnv = "COMPACT_UUID_TEST_MYSQL_CONTAINER"
	// compactReplPGReplicaPort is the host port for the streaming standby.
	compactReplPGReplicaPort = "15532"
	// compactReplPGRestorePort is the host port for the freshly provisioned PostgreSQL server.
	compactReplPGRestorePort = "15533"
	// compactReplMySQLReplicaPort is the host port for the row-based replica.
	compactReplMySQLReplicaPort = "13406"
	// compactReplMySQLRestorePort is the host port for the freshly provisioned MySQL server.
	compactReplMySQLRestorePort = "13407"
	// compactReplRows is the seeded fixture size; the contract under test is per-row derivation
	// rather than throughput, so a small deterministic fixture is enough and stays bounded.
	compactReplRows = 5
	// compactReplSentinelHex is what a replica-local trigger writes if the applier re-fires it.
	compactReplSentinelHex = "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"
	// compactReplWait bounds every convergence and container-readiness wait.
	compactReplWait = 120 * time.Second
)

// =============================================================================
// TOPOLOGY AND FIXTURES OVER AN ARBITRARY LIVE DATABASE
// =============================================================================

// compactReplUseDialect installs the process globals and fast intervals for one live engine.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - dialect: engine descriptor.
//
// Return values: none.
func compactReplUseDialect(t *testing.T, dialect compactLiveDialect) {
	t.Helper()
	withCompactTestSettings(t)
	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(dialect.name == "mysql")
	common.UsingPostgreSQL.Store(dialect.name == "postgres")
	t.Cleanup(func() {
		common.UsingSQLite.Store(true)
		common.UsingMySQL.Store(false)
		common.UsingPostgreSQL.Store(false)
	})
}

// compactReplOpenTopology opens a unified topology over one database WITHOUT resetting it.
//
// Deliberately not newLiveCompactTopology: that helper drops every compact and legacy object to
// make the live matrix repeatable, which would destroy the very restore under test here.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - dialect: engine descriptor.
//   - dsn: data source name.
//   - migrate: true to run the ordinary bootstrap AutoMigrate, as a real process would.
//
// Return values:
//   - *gorm.DB: open handle.
//   - *databaseTopology: unified topology over that handle.
func compactReplOpenTopology(t *testing.T, dialect compactLiveDialect, dsn string,
	migrate bool) (*gorm.DB, *databaseTopology) {
	t.Helper()
	db, err := gorm.Open(dialect.open(dsn), &gorm.Config{})
	require.NoError(t, err, "open %s handle", dialect.name)
	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			_ = pool.Close()
		}
	})
	if migrate {
		withTestDBGlobals(t, db, db)
		require.NoError(t, migrateDB())
	}
	topology, err := newUnifiedTopology(db)
	require.NoError(t, err)
	return db, topology
}

// compactReplSeedUsers writes the deterministic legacy fixture through ordinary SQL.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle to insert into.
//
// Return values: none.
func compactReplSeedUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	for index := 1; index <= compactReplRows; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}
}

// compactReplShadowRow is one users row's primary key and hex shadow.
type compactReplShadowRow struct {
	// ID is the primary key.
	ID int64 `gorm:"column:id"`
	// Shadow is the uppercase hex shadow, empty for SQL NULL.
	Shadow string `gorm:"column:shadow"`
}

// compactReplReadShadows returns every users row's shadow from one handle.
//
// The rendering is per-dialect because the physical types differ: a native uuid on PostgreSQL
// and a BINARY(16) on MySQL have no portable "show me these bytes" expression.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle to read from.
//   - dialect: engine descriptor.
//
// Return values:
//   - []compactReplShadowRow: rows in ascending primary-key order.
func compactReplReadShadows(t *testing.T, db *gorm.DB, dialect compactLiveDialect) []compactReplShadowRow {
	t.Helper()
	sql := "SELECT id, COALESCE(HEX(uuid_compact), '') AS shadow FROM users ORDER BY id"
	if dialect.name == "postgres" {
		sql = "SELECT id, COALESCE(UPPER(REPLACE(uuid_compact::text, '-', '')), '') AS shadow" +
			" FROM users ORDER BY id"
	}
	rows := []compactReplShadowRow{}
	require.NoError(t, db.Raw(sql).Scan(&rows).Error)
	return rows
}

// compactReplRequireConverged asserts a replica's shadows equal the primary's, byte for byte.
// Parameters:
//   - t: test handle used for assertions.
//   - primary: primary handle.
//   - replica: replica handle.
//   - dialect: engine descriptor.
//
// Return values: none.
func compactReplRequireConverged(t *testing.T, primary *gorm.DB, replica *gorm.DB,
	dialect compactLiveDialect) {
	t.Helper()
	expected := compactReplReadShadows(t, primary, dialect)
	require.Len(t, expected, compactReplRows, "the primary fixture must be intact")
	require.Equal(t, expected, compactReplReadShadows(t, replica, dialect),
		"every replicated shadow must equal the primary's")
	for index := 1; index <= compactReplRows; index++ {
		requireLiveShadowMatches(t, replica, dialect, index, compactUUIDTextFor(index))
	}
}

// compactReplRequireReplicaAudit exercises the process-local health gate against a replica.
//
// HONESTY NOTE, because it bounds what this proves: the gate is a package-level atomic per role,
// so one test binary has exactly one gate. This cannot start a genuinely separate OS process and
// observe ITS gate. What it does assert, against the real replica, is the gate's semantics — a
// process that has never audited keeps compact reads off and still resolves correctly through
// legacy text on the replica handle, and the replica's OWN read-only audit is what turns compact
// reads on. resetCompactHealthForTest stands in for "this process has not audited yet"; without
// it the primary's coordinator, which ran in this same binary, would have already published
// health and the assertion would be vacuous.
// Parameters:
//   - t: test handle used for assertions.
//   - replica: handle pointing at the replica.
//   - topology: unified topology over the replica handle.
//
// Return values: none.
func compactReplRequireReplicaAudit(t *testing.T, replica *gorm.DB, topology *databaseTopology) {
	ctx := compactTestContext(t)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)

	resetCompactHealthForTest()
	enabled, reason := compactReadsEnabled(target.role)
	require.False(t, enabled,
		"a process that has not run its own audit must not use compact predicates on a replica")
	require.NotEmpty(t, reason, "the disabled gate must report a bounded reason")

	id, err := resolveIDByUUID(ctx, replica, target, compactUUIDTextFor(1))
	require.NoError(t, err, "legacy text must resolve on the replica while compact reads are gated")
	require.Equal(t, int64(1), id)

	runCompactHealthAudit(ctx, topology)
	enabled, reason = compactReadsEnabled(target.role)
	require.True(t, enabled,
		"the replica's own read-only audit must enable compact predicates: %s", reason)

	id, err = resolveIDByUUID(ctx, replica, target, compactUUIDTextFor(1))
	require.NoError(t, err, "the compact path must resolve on the replica after its own audit")
	require.Equal(t, int64(1), id)
}

// =============================================================================
// POSTGRESQL PHYSICAL STREAMING REPLICATION
// =============================================================================

// compactReplStartPGStandby builds a real pg_basebackup streaming standby of the primary.
//
// The standby is created BEFORE the compact migration runs, so every compact column, trigger,
// index, backfilled byte, and marker must arrive over the WAL stream. A standby taken afterwards
// would prove nothing about streaming.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - parts: primary connection settings.
//
// Return values:
//   - compactReplPGParts: connection settings addressing the standby.
func compactReplStartPGStandby(t *testing.T, parts compactReplPGParts) compactReplPGParts {
	t.Helper()
	container := compactReplContainer(compactReplPGContainerEnv, "cuuid-pg")
	// The stock image's pg_hba.conf carries no `replication` entry, so no standby can connect at
	// all until one exists. Appending it and reloading is additive and needs no restart, which is
	// what makes this safe on a primary other suites are using.
	compactReplShell(t, "docker exec "+container+" bash -c "+
		"\"grep -q compact-uuid-replication /var/lib/postgresql/data/pg_hba.conf || "+
		"echo 'host replication all 0.0.0.0/0 scram-sha-256 # compact-uuid-replication' "+
		">> /var/lib/postgresql/data/pg_hba.conf\"")
	compactReplShell(t, parts.psql(parts.database)+" psql -Atc 'SELECT pg_reload_conf()'")

	address := compactReplDockerIP(t, container)
	volume := "cuuid-pg-replica-data"
	_, _ = compactReplTryShell("docker rm -f cuuid-pg-replica; docker volume rm " + volume)
	t.Cleanup(func() { _, _ = compactReplTryShell("docker volume rm " + volume) })
	compactReplShell(t, "docker volume create "+volume)
	// -R writes standby.signal and primary_conninfo; -Xs streams WAL alongside the copy so the
	// backup is self-consistent without a slot that would retain WAL on the shared primary.
	compactReplShell(t, "docker run --rm --user postgres -v "+volume+":/var/lib/postgresql/data postgres:17 "+
		"bash -c \"rm -rf /var/lib/postgresql/data/* && pg_basebackup -D /var/lib/postgresql/data "+
		"-Fp -Xs -R -c fast -d 'host="+address+" port=5432 user="+parts.user+" password="+parts.password+
		" dbname=postgres' && chmod 0700 /var/lib/postgresql/data\"")
	compactReplStartContainer(t, "cuuid-pg-replica", "-v "+volume+":/var/lib/postgresql/data -p "+
		compactReplPGReplicaPort+":5432 postgres:17 -c hot_standby=on")

	replica := parts.at(compactReplPGReplicaPort, parts.database)
	compactReplWaitUntil(t, "the standby to accept read-only connections", func() bool {
		output, err := compactReplTryShell(replica.psql("postgres") + " psql -Atc 'SELECT pg_is_in_recovery()'")
		return err == nil && strings.TrimSpace(output) == "t"
	})
	// A standby that is up but not streaming would still converge from the base backup alone, so
	// the WAL sender must exist before anything read there is trusted.
	compactReplWaitUntil(t, "the primary to report a streaming walreceiver", func() bool {
		output, err := compactReplTryShell(parts.psql(parts.database) +
			" psql -Atc 'SELECT state FROM pg_stat_replication'")
		return err == nil && strings.Contains(output, "streaming")
	})
	return replica
}

// compactReplWaitPGCatchUp blocks until the standby has replayed everything the primary wrote.
// Parameters:
//   - t: test handle used for assertions.
//   - primary: primary connection settings.
//   - replica: standby connection settings.
//
// Return values: none.
func compactReplWaitPGCatchUp(t *testing.T, primary compactReplPGParts, replica compactReplPGParts) {
	t.Helper()
	lsn := strings.TrimSpace(compactReplShell(t,
		primary.psql(primary.database)+" psql -Atc 'SELECT pg_current_wal_lsn()'"))
	require.NotEmpty(t, lsn)
	compactReplWaitUntil(t, "the standby to replay LSN "+lsn, func() bool {
		output, err := compactReplTryShell(replica.psql("postgres") +
			" psql -Atc \"SELECT pg_last_wal_replay_lsn() >= '" + lsn + "'::pg_lsn\"")
		return err == nil && strings.TrimSpace(output) == "t"
	})
}

// compactReplRunPGStreaming qualifies PostgreSQL physical streaming replication.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func compactReplRunPGStreaming(t *testing.T) {
	dialect := compactLiveDialects()[1]
	primaryDB, topology, ok := newLiveCompactTopology(t, dialect, false)
	if !ok {
		t.Skipf("%s is not configured", dialect.primaryEnv)
	}
	parts := compactReplParsePG(t, strings.TrimSpace(os.Getenv(dialect.primaryEnv)))
	compactReplSeedUsers(t, primaryDB)

	replicaParts := compactReplStartPGStandby(t, parts)

	// Everything from here on has to reach the standby as WAL.
	require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)
	compactReplWaitPGCatchUp(t, parts, replicaParts)

	replicaDB, replicaTopology := compactReplOpenTopology(t, dialect, replicaParts.dsn(), false)
	ctx := compactTestContext(t)

	// Without this, a handle that silently addressed the PRIMARY would pass every assertion
	// below while proving nothing about replication.
	inRecovery := false
	require.NoError(t, replicaDB.Raw("SELECT pg_is_in_recovery()").Scan(&inRecovery).Error)
	require.True(t, inRecovery, "the replica handle must address the standby, not the primary")

	t.Run("columns triggers indexes and markers arrive over the stream", func(t *testing.T) {
		verified, reason, err := validateCompactObjects(ctx, replicaTopology)
		require.NoError(t, err)
		require.True(t, verified, "the standby's own catalog did not verify: %s", reason)
		for _, target := range compactTargetsForTopology(replicaTopology) {
			ok, err := verifyCompactColumnType(ctx, replicaDB, target)
			require.NoError(t, err)
			require.True(t, ok, "%s lacks the native uuid shadow type on the standby", target.id())
		}
		complete, err := isDataMigrationComplete(ctx, replicaTopology.primary, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.True(t, complete, "the completion marker must replicate")
	})

	t.Run("compact values converge and are byte-identical to the primary", func(t *testing.T) {
		compactReplRequireConverged(t, primaryDB, replicaDB, dialect)
	})

	t.Run("a post-completion primary write converges too", func(t *testing.T) {
		seedCompactUser(t, primaryDB, 600, compactUUIDTextFor(600))
		compactReplWaitPGCatchUp(t, parts, replicaParts)
		requireLiveShadowMatches(t, replicaDB, dialect, 600, compactUUIDTextFor(600))
		require.NoError(t, primaryDB.Exec("DELETE FROM users WHERE id = ?", 600).Error)
		compactReplWaitPGCatchUp(t, parts, replicaParts)
	})

	t.Run("replica compact reads await a replica-local audit", func(t *testing.T) {
		compactReplRequireReplicaAudit(t, replicaDB, replicaTopology)
	})
}

// =============================================================================
// MYSQL ROW-BASED REPLICATION
// =============================================================================

// compactReplStartMySQLReplica builds a real GTID row-based replica of the MySQL primary.
//
// The replica is scoped to this suite's database by a replication filter. That is not a
// convenience: the shared primary hosts several suites' databases, the replica's seeded
// gtid_purged means it holds none of them, and an unfiltered applier would fail on the first
// foreign statement. The database itself is created on both sides before replication starts,
// because it falls inside gtid_purged and can never arrive over the binlog.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - dsn: primary DSN, whose database this replica follows.
//
// Return values:
//   - string: DSN addressing the replica.
func compactReplStartMySQLReplica(t *testing.T, dsn string) string {
	t.Helper()
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	database := config.DBName
	container := compactReplContainer(compactReplMySQLContainerEnv, "cuuid-mysql")
	client := compactReplMySQLClient(container, config)

	compactReplShell(t, client+" -e \"CREATE DATABASE IF NOT EXISTS "+database+" CHARACTER SET utf8mb4\"")
	compactReplShell(t, client+" -e \"CREATE USER IF NOT EXISTS 'cuuid_repl'@'%' IDENTIFIED BY 'cuuid_repl';"+
		" GRANT REPLICATION SLAVE ON *.* TO 'cuuid_repl'@'%'\"")

	address := compactReplDockerIP(t, container)
	compactReplStartContainer(t, "cuuid-mysql-replica",
		"-e MYSQL_ROOT_PASSWORD="+config.Passwd+" -p "+compactReplMySQLReplicaPort+":3306 mysql:8.4"+
			" --server-id=2 --log-bin=mysql-bin --binlog-format=ROW --gtid-mode=ON"+
			" --enforce-gtid-consistency=ON --relay-log=relay --skip-replica-start")
	replicaClient := compactReplMySQLClient("cuuid-mysql-replica", config)
	compactReplWaitUntil(t, "the replica server to finish initializing", func() bool {
		_, err := compactReplTryShell(replicaClient + " -e 'SELECT 1'")
		return err == nil
	})

	gtid := strings.ReplaceAll(strings.TrimSpace(
		compactReplShell(t, client+" -e 'SELECT @@GLOBAL.gtid_executed'")), "\n", "")
	require.NotEmpty(t, gtid, "the primary must have GTIDs to seed gtid_purged from")
	compactReplShell(t, replicaClient+" -e \""+
		"CREATE DATABASE IF NOT EXISTS "+database+" CHARACTER SET utf8mb4;"+
		"RESET BINARY LOGS AND GTIDS;"+
		"SET @@GLOBAL.gtid_purged='"+gtid+"';"+
		"CHANGE REPLICATION FILTER REPLICATE_DO_DB=("+database+");"+
		"CHANGE REPLICATION SOURCE TO SOURCE_HOST='"+address+"', SOURCE_PORT=3306,"+
		" SOURCE_USER='cuuid_repl', SOURCE_PASSWORD='cuuid_repl', SOURCE_AUTO_POSITION=1,"+
		" GET_SOURCE_PUBLIC_KEY=1;"+
		"START REPLICA\"")
	compactReplWaitUntil(t, "both replica threads to report ON", func() bool {
		output, err := compactReplTryShell(replicaClient + " -e \"SELECT (SELECT SERVICE_STATE FROM" +
			" performance_schema.replication_connection_status) = 'ON' AND (SELECT SERVICE_STATE FROM" +
			" performance_schema.replication_applier_status) = 'ON'\"")
		return err == nil && strings.TrimSpace(output) == "1"
	})
	return compactReplMySQLDSN(t, dsn, compactReplMySQLReplicaPort, database)
}

// compactReplWaitMySQLCatchUp blocks until the replica has executed the primary's GTID set.
//
// Filtered-out transactions are still recorded on the replica as empty transactions, which is
// why waiting on the whole executed set works even though only one database is really applied.
// Parameters:
//   - t: test handle used for assertions.
//   - primary: primary handle.
//   - replica: replica handle.
//
// Return values: none.
func compactReplWaitMySQLCatchUp(t *testing.T, primary *gorm.DB, replica *gorm.DB) {
	t.Helper()
	gtid := ""
	require.NoError(t, primary.Raw("SELECT @@GLOBAL.gtid_executed").Scan(&gtid).Error)
	gtid = strings.ReplaceAll(strings.TrimSpace(gtid), "\n", "")
	require.NotEmpty(t, gtid)

	var result *int
	require.NoError(t, replica.Raw("SELECT WAIT_FOR_EXECUTED_GTID_SET(?, ?)",
		gtid, int(compactReplWait/time.Second)).Scan(&result).Error)
	require.NotNil(t, result, "WAIT_FOR_EXECUTED_GTID_SET returned NULL")
	require.Equal(t, 0, *result, "the replica did not execute the primary's GTID set in time")
}

// compactReplRunMySQLReplication qualifies MySQL row-based replication.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values: none.
func compactReplRunMySQLReplication(t *testing.T) {
	dialect := compactLiveDialects()[0]
	dsn := strings.TrimSpace(os.Getenv(dialect.primaryEnv))
	if dsn == "" {
		t.Skipf("%s is not configured", dialect.primaryEnv)
	}
	// Replication starts before any schema exists, so the whole migration flows through the
	// binlog: the CREATE TABLE, the ADD COLUMN, the CREATE TRIGGER, and the backfilled rows.
	replicaDSN := compactReplStartMySQLReplica(t, dsn)

	primaryDB, topology, ok := newLiveCompactTopology(t, dialect, false)
	require.True(t, ok)
	compactReplSeedUsers(t, primaryDB)
	require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)

	replicaDB, replicaTopology := compactReplOpenTopology(t, dialect, replicaDSN, false)
	// Without this, a handle that silently addressed the PRIMARY would pass every assertion below
	// while proving nothing about replication.
	serverID := 0
	require.NoError(t, replicaDB.Raw("SELECT @@server_id").Scan(&serverID).Error)
	require.Equal(t, 2, serverID, "the replica handle must address the replica, not the primary")

	compactReplWaitMySQLCatchUp(t, primaryDB, replicaDB)
	ctx := compactTestContext(t)

	t.Run("row replication carries the whole compact contract", func(t *testing.T) {
		verified, reason, err := validateCompactObjects(ctx, replicaTopology)
		require.NoError(t, err)
		require.True(t, verified, "the replica's own catalog did not verify: %s", reason)
		complete, err := isDataMigrationComplete(ctx, replicaTopology.primary, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.True(t, complete, "the completion marker must replicate")
	})

	t.Run("compact values converge on the replica", func(t *testing.T) {
		compactReplRequireConverged(t, primaryDB, replicaDB, dialect)
	})

	t.Run("replica compact reads await a replica-local audit", func(t *testing.T) {
		compactReplRequireReplicaAudit(t, replicaDB, replicaTopology)
	})

	t.Run("the row applier ships post-trigger images and does not re-fire triggers", func(t *testing.T) {
		// The subtlety worth proving rather than quoting from documentation: with ROW binlog the
		// replica applies the primary's post-trigger row image. Because the sync trigger is
		// deterministic, a re-fire and a shipped image are indistinguishable by value alone — so
		// the replica's own trigger is swapped for one writing a sentinel. If the applier fired
		// it, the replicated row would hold that sentinel.
		require.NoError(t, replicaDB.Exec("DROP TRIGGER IF EXISTS "+compactInsertTriggerName("users")).Error)
		require.NoError(t, replicaDB.Exec("CREATE TRIGGER "+compactInsertTriggerName("users")+
			" BEFORE INSERT ON users FOR EACH ROW SET NEW.uuid_compact = UNHEX(REPEAT('FF', 16))").Error)

		// Control, without which the whole experiment could pass vacuously: a sentinel trigger
		// that silently failed to install would also never write the sentinel. A LOCAL insert on
		// the replica must fire it, proving the trigger is live on this server.
		seedCompactUser(t, replicaDB, 701, compactUUIDTextFor(701))
		require.Equal(t, compactReplSentinelHex,
			readCompactShadowHex(t, replicaDB, "users", "uuid_compact", 701),
			"the sentinel trigger must be live on the replica for this experiment to mean anything")

		seedCompactUser(t, primaryDB, 700, compactUUIDTextFor(700))
		compactReplWaitMySQLCatchUp(t, primaryDB, replicaDB)

		require.NotEqual(t, compactReplSentinelHex,
			readCompactShadowHex(t, replicaDB, "users", "uuid_compact", 700),
			"the ROW applier must not re-fire the replica's own trigger")
		requireLiveShadowMatches(t, replicaDB, dialect, 700, compactUUIDTextFor(700))

		// The sentinel trigger is real drift in the replica's own catalog, so the replica's own
		// audit must now refuse compact predicates. That is what proves the audit reads the
		// REPLICA's catalog rather than inheriting the primary's verdict.
		runCompactHealthAudit(ctx, replicaTopology)
		enabled, reason := compactReadsEnabled(uuidRolePrimary)
		require.False(t, enabled, "a replica whose own trigger drifted must disable compact reads")
		require.Contains(t, reason, "users")
	})
}

// TestCompactUUIDReplicationAndRestore qualifies AUTO-T28's replication and restore contract.
// Parameters:
//   - t: test handle.
//
// Return values: none.
func TestCompactUUIDReplicationAndRestore(t *testing.T) {
	if strings.TrimSpace(os.Getenv(compactReplEnableEnv)) != "1" {
		t.Skipf("%s is not set; CI's no-skip guard enforces this suite", compactReplEnableEnv)
	}

	t.Run("postgres/physical-streaming", compactReplRunPGStreaming)
	t.Run("mysql/row-replication", compactReplRunMySQLReplication)

	t.Run("postgres/restore-modes", func(t *testing.T) {
		dsn := strings.TrimSpace(os.Getenv(compactLiveDialects()[1].primaryEnv))
		if dsn == "" {
			t.Skipf("%s is not configured", compactLiveDialects()[1].primaryEnv)
		}
		parts := compactReplParsePG(t, dsn)
		compactReplRunRestores(t, compactReplPGRestore(parts), parts.database)
	})

	t.Run("mysql/restore-modes", func(t *testing.T) {
		dsn := strings.TrimSpace(os.Getenv(compactLiveDialects()[0].primaryEnv))
		if dsn == "" {
			t.Skipf("%s is not configured", compactLiveDialects()[0].primaryEnv)
		}
		config, err := mysqldriver.ParseDSN(dsn)
		require.NoError(t, err)
		compactReplRunRestores(t, compactReplMySQLRestore(t, dsn), config.DBName)
	})
}
