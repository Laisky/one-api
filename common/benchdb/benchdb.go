// Package benchdb provisions the database handles the observability benchmarks
// measure against.
//
// The Phase-0 and Phase-1 optimizations in
// docs/proposals/20260905_observability-data-tiering.md are about database and
// disk pressure, so a benchmark that only ran against SQLite would mislead:
// SQLite serializes writers, which exaggerates the win from removing write
// statements, and it cannot show the WAL-burst or gap-locking behavior the
// chunked-retention work targets. Real PostgreSQL and MySQL engines are
// therefore first-class targets here.
//
// Real-engine benchmarks are opt-in through environment variables so an
// ordinary `go test ./...` never depends on a container being up.
package benchdb

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// Engine identifies a database engine a benchmark can target.
type Engine string

const (
	// EngineSQLite is the embedded default deployment target.
	EngineSQLite Engine = "sqlite"
	// EnginePostgres is the PostgreSQL target, enabled by ONEAPI_BENCH_PG_DSN.
	EnginePostgres Engine = "postgres"
	// EngineMySQL is the MySQL target, enabled by ONEAPI_BENCH_MYSQL_DSN.
	EngineMySQL Engine = "mysql"
)

// EnvPostgresDSN names the environment variable holding the PostgreSQL DSN.
const EnvPostgresDSN = "ONEAPI_BENCH_PG_DSN"

// EnvMySQLDSN names the environment variable holding the MySQL DSN.
const EnvMySQLDSN = "ONEAPI_BENCH_MYSQL_DSN"

// Target describes one engine a benchmark should run against.
type Target struct {
	// Engine is the engine identifier, used in benchmark names.
	Engine Engine
	// DSN is the connection string; empty for SQLite, which is in-process.
	DSN string
}

// Targets returns every engine the current environment can exercise.
//
// SQLite is always present. PostgreSQL and MySQL appear only when their DSN
// environment variable is set, so the benchmarks degrade to the embedded engine
// rather than failing when no container is running.
//
// Parameters: none.
//
// Return values:
//   - []Target: the available targets, SQLite first.
func Targets() []Target {
	targets := []Target{{Engine: EngineSQLite}}
	if dsn := strings.TrimSpace(os.Getenv(EnvPostgresDSN)); dsn != "" {
		targets = append(targets, Target{Engine: EnginePostgres, DSN: dsn})
	}
	if dsn := strings.TrimSpace(os.Getenv(EnvMySQLDSN)); dsn != "" {
		targets = append(targets, Target{Engine: EngineMySQL, DSN: dsn})
	}
	return targets
}

// OpenPair opens two independent handles to the SAME benchmark database.
//
// A benchmark that runs a concurrent workload alongside the system under test
// must not instrument both through one handle: gorm callbacks are per-handle,
// so a statement timer registered for the sweep would also time every
// foreground query and contaminate the percentiles. Two handles also model
// production more closely, where background workers and request handlers draw
// from the same pool but are separate logical clients.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast and to scope temporary files.
//   - target: the engine to open.
//
// Return values:
//   - *gorm.DB: the primary handle, for the system under test.
//   - *gorm.DB: a second handle to the same database, for concurrent load.
//   - func(): restores the previously active dialect flags.
func OpenPair(tb testing.TB, target Target) (*gorm.DB, *gorm.DB, func()) {
	tb.Helper()

	primary, restore := Open(tb, target)

	sqlitePath := ""
	if target.Engine == EngineSQLite || target.Engine == "" {
		sqlitePath = sqlitePathOf(tb, primary)
	}
	secondary := openSecondary(tb, target, sqlitePath)

	return primary, secondary, restore
}

// sqlitePathOf recovers the file a SQLite handle was opened against, so a second
// handle can attach to the same database rather than a fresh one.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast.
//   - db: the handle to inspect.
//
// Return values:
//   - string: the database file path.
func sqlitePathOf(tb testing.TB, db *gorm.DB) string {
	tb.Helper()
	var rows []struct {
		Seq  int
		Name string
		File string
	}
	if err := db.Raw("PRAGMA database_list").Scan(&rows).Error; err != nil {
		tb.Fatalf("resolve sqlite path: %+v", errors.WithStack(err))
	}
	for _, r := range rows {
		if r.Name == "main" && r.File != "" {
			return r.File
		}
	}
	tb.Fatal("sqlite main database has no file path")
	return ""
}

// openSecondary opens an additional handle to an already-provisioned database.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast.
//   - target: the engine to open.
//   - sqlitePath: the existing SQLite file, ignored for other engines.
//
// Return values:
//   - *gorm.DB: the additional handle.
func openSecondary(tb testing.TB, target Target, sqlitePath string) *gorm.DB {
	tb.Helper()

	cfg := &gorm.Config{Logger: glogger.Discard, PrepareStmt: true}

	var (
		db  *gorm.DB
		err error
	)
	switch target.Engine {
	case EnginePostgres:
		db, err = gorm.Open(postgres.New(postgres.Config{DSN: target.DSN, PreferSimpleProtocol: true}), cfg)
	case EngineMySQL:
		db, err = gorm.Open(mysql.Open(target.DSN), cfg)
	default:
		db, err = gorm.Open(sqlite.Open(sqlitePath+"?_busy_timeout=10000&_journal_mode=WAL&_synchronous=NORMAL"), cfg)
	}
	if err != nil {
		tb.Fatalf("open secondary %s handle: %+v", target.Engine, errors.WithStack(err))
	}

	applyProductionPool(tb, db)
	closeOnCleanup(tb, db)
	return db
}

// Open opens a handle for the target and sets the process dialect flags the
// model layer branches on.
//
// The dialect flags are global state, so a benchmark must run one target at a
// time; the returned restore function puts them back.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast and to scope temporary files.
//   - target: the engine to open.
//
// Return values:
//   - *gorm.DB: the opened handle, with gorm logging silenced so the benchmark
//     measures the system rather than log formatting.
//   - func(): restores the previously active dialect flags.
func Open(tb testing.TB, target Target) (*gorm.DB, func()) {
	tb.Helper()

	prevSQLite := common.UsingSQLite.Load()
	prevPG := common.UsingPostgreSQL.Load()
	prevMySQL := common.UsingMySQL.Load()
	restore := func() {
		common.UsingSQLite.Store(prevSQLite)
		common.UsingPostgreSQL.Store(prevPG)
		common.UsingMySQL.Store(prevMySQL)
	}

	// Match production: model.chooseDB opens every handle with PrepareStmt
	// enabled. Leaving it off would penalize the write-heavy baseline variants
	// relative to how one-api actually runs.
	cfg := &gorm.Config{Logger: glogger.Discard, PrepareStmt: true}

	var (
		db  *gorm.DB
		err error
	)
	switch target.Engine {
	case EnginePostgres:
		common.UsingSQLite.Store(false)
		common.UsingPostgreSQL.Store(true)
		common.UsingMySQL.Store(false)
		db, err = gorm.Open(postgres.New(postgres.Config{DSN: target.DSN, PreferSimpleProtocol: true}), cfg)
	case EngineMySQL:
		common.UsingSQLite.Store(false)
		common.UsingPostgreSQL.Store(false)
		common.UsingMySQL.Store(true)
		db, err = gorm.Open(mysql.Open(target.DSN), cfg)
	default:
		common.UsingSQLite.Store(true)
		common.UsingPostgreSQL.Store(false)
		common.UsingMySQL.Store(false)
		// A file-backed database with WAL, matching how one-api actually runs
		// SQLite. An in-memory database would remove the write-serialization
		// behavior that makes SQLite results what they are.
		path := tb.TempDir() + "/bench.db"
		db, err = gorm.Open(sqlite.Open(path+"?_busy_timeout=10000&_journal_mode=WAL&_synchronous=NORMAL"), cfg)
	}

	if err != nil {
		restore()
		tb.Fatalf("open %s benchmark database: %+v", target.Engine, errors.WithStack(err))
	}

	applyProductionPool(tb, db)
	closeOnCleanup(tb, db)

	return db, restore
}

// applyProductionPool sizes the connection pool the way model.setDBConns sizes
// it in production.
//
// This matters for fairness, not for speed: a benchmark variant that issues
// twelve statements per request needs far more concurrent connections than one
// that issues a batched INSERT every few hundred requests. Leaving the
// database/sql defaults in place (2 idle connections) would starve the baseline
// and manufacture a win.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast.
//   - db: the handle to configure.
//
// Return values: none.
func applyProductionPool(tb testing.TB, db *gorm.DB) {
	tb.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		tb.Fatalf("resolve sql handle: %+v", errors.WithStack(err))
	}
	// Production allows 2000 open connections, but a benchmark container is
	// configured for far fewer and a suite that opens many handles would
	// exhaust the server. The cap is generous relative to the concurrency any
	// arm actually uses (GOMAXPROCS-way parallelism plus a couple of writers),
	// so no variant is starved and the comparison stays fair.
	maxOpen := min(config.SQLMaxOpenConns, benchMaxOpenConns)
	sqlDB.SetMaxIdleConns(min(config.SQLMaxIdleConns, maxOpen))
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(time.Second * time.Duration(config.SQLMaxLifetimeSeconds))
}

// benchMaxOpenConns bounds the pool so a suite of benchmarks cannot exhaust the
// database server's connection limit.
const benchMaxOpenConns = 32

// closeOnCleanup releases a handle's connections when the benchmark ends.
//
// Without this, every sub-benchmark leaks a full pool and a multi-count run
// fails with "too many clients already" partway through, silently corrupting
// the arms that follow.
//
// Parameters:
//   - tb: the test or benchmark, used to register cleanup.
//   - db: the handle to close.
//
// Return values: none.
func closeOnCleanup(tb testing.TB, db *gorm.DB) {
	tb.Helper()
	tb.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			return
		}
		if err := sqlDB.Close(); err != nil {
			tb.Logf("close benchmark database handle: %+v", err)
		}
	})
}

// Reset removes every row from the named tables so consecutive benchmark
// iterations start from the same state.
//
// PostgreSQL and MySQL use TRUNCATE rather than DELETE. That is not a
// performance preference: DELETE leaves dead tuples behind, so a benchmark that
// seeds a million rows and then "resets" to a hundred thousand actually
// measures a scan over 1.1 million tuples. That contaminated an earlier version
// of the dashboard cold-miss measurement, which reported the same query over
// the same nominal table 8x apart from the scaling arm.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast.
//   - db: the handle owning the tables.
//   - tables: physical table names to clear.
//
// Return values: none.
func Reset(tb testing.TB, db *gorm.DB, tables ...string) {
	tb.Helper()
	for _, table := range tables {
		var stmt string
		switch {
		case common.UsingPostgreSQL.Load():
			stmt = "TRUNCATE TABLE " + table
		case common.UsingMySQL.Load():
			stmt = "TRUNCATE TABLE " + table
		default:
			stmt = "DELETE FROM " + table
		}
		if err := db.Exec(stmt).Error; err != nil {
			tb.Fatalf("reset table %s: %+v", table, errors.WithStack(err))
		}
	}
}
