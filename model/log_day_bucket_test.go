package model

// UTC day-bucket regression tests for the dashboard aggregates
// (docs/proposals/20260905_observability-data-tiering.md, section 1 and W2.1).
//
// Two defects are pinned here. The day expression used to select its dialect
// from process-global engine flags although the queries run on LOG_DB, so a
// deployment whose LOG_SQL_DSN names a different engine from SQL_DSN could emit
// the wrong engine's syntax. And the PostgreSQL and MySQL forms resolved in the
// database session time zone while SQLite's was unconditionally UTC, so the same
// row could land in different days on different engines.

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// TestDayAggregationSelectUsesHandleDialect verifies the expression follows the
// handle rather than the process-global engine flags.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestDayAggregationSelectUsesHandleDialect(t *testing.T) {
	// Deliberately set the globals to disagree with every handle below: if the
	// implementation consulted them, each assertion would fail.
	prevPG, prevMySQL, prevSQLite := common.UsingPostgreSQL.Load(), common.UsingMySQL.Load(), common.UsingSQLite.Load()
	common.UsingPostgreSQL.Store(true)
	common.UsingMySQL.Store(true)
	common.UsingSQLite.Store(true)
	t.Cleanup(func() {
		common.UsingPostgreSQL.Store(prevPG)
		common.UsingMySQL.Store(prevMySQL)
		common.UsingSQLite.Store(prevSQLite)
	})

	pg, err := gorm.Open(postgres.New(postgres.Config{DSN: "postgres://u:p@127.0.0.1:1/none", PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	require.Contains(t, dayAggregationSelect(pg), "TIMESTAMP 'epoch'")

	my, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/none", SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	require.Contains(t, dayAggregationSelect(my), "DATE_ADD('1970-01-01'")

	lite, err := gorm.Open(sqlite.Open(t.TempDir()+"/day.db"), &gorm.Config{})
	require.NoError(t, err)
	require.Contains(t, dayAggregationSelect(lite), "unixepoch")
}

// TestDayAggregationSelectIsTimezoneIndependent verifies no expression mentions
// a construct that resolves in the database session time zone.
//
// `to_timestamp()` on PostgreSQL and `FROM_UNIXTIME()` on MySQL both do, and
// both silently shift every day boundary on a server whose zone is not UTC.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestDayAggregationSelectIsTimezoneIndependent(t *testing.T) {
	pg, err := gorm.Open(postgres.New(postgres.Config{DSN: "postgres://u:p@127.0.0.1:1/none", PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	require.NotContains(t, dayAggregationSelect(pg), "to_timestamp",
		"to_timestamp returns timestamptz, so date_trunc on it resolves in the session zone")

	my, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/none", SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	require.NotContains(t, dayAggregationSelect(my), "FROM_UNIXTIME",
		"FROM_UNIXTIME converts using the session time zone")
}

// TestDayAggregationBucketsAgreeAcrossEngines verifies every engine assigns the
// same UTC day to the same epoch second, including the boundary second.
//
// The SQLite arm always runs; the PostgreSQL and MySQL arms run only when their
// DSN is provided, and they deliberately set a non-UTC session zone so a
// zone-dependent expression would fail.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestDayAggregationBucketsAgreeAcrossEngines(t *testing.T) {
	// 1767225540 is 2025-12-31T23:59:00Z: inside 2025-12-31 in UTC, but inside
	// 2026-01-01 at +09:00.
	cases := []struct {
		epoch int64
		want  string
	}{
		{1767225540, "2025-12-31"},
		{1767225599, "2025-12-31"},
		{1767225600, "2026-01-01"},
		{0, "1970-01-01"},
	}

	// Probe against a real BIGINT column rather than a bind parameter: an
	// untyped parameter is text on PostgreSQL, which is not how the production
	// query reads logs.created_at.
	run := func(t *testing.T, db *gorm.DB, setZone string) {
		t.Helper()
		if setZone != "" {
			require.NoError(t, db.Exec(setZone).Error)
		}

		require.NoError(t, db.Exec("DROP TABLE IF EXISTS day_bucket_probe").Error)
		require.NoError(t, db.Exec("CREATE TABLE day_bucket_probe (created_at BIGINT)").Error)
		t.Cleanup(func() {
			if err := db.Exec("DROP TABLE IF EXISTS day_bucket_probe").Error; err != nil {
				t.Logf("drop probe table: %+v", err)
			}
		})

		expr := dayAggregationSelect(db)
		for _, tc := range cases {
			require.NoError(t, db.Exec("DELETE FROM day_bucket_probe").Error)
			require.NoError(t, db.Exec("INSERT INTO day_bucket_probe (created_at) VALUES (?)", tc.epoch).Error)

			var got string
			require.NoError(t, db.Raw("SELECT "+expr+" FROM day_bucket_probe").Scan(&got).Error)
			require.Equal(t, tc.want, got,
				"epoch %d must bucket to the UTC day regardless of session time zone", tc.epoch)
		}
	}

	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(t.TempDir()+"/buckets.db"), &gorm.Config{})
		require.NoError(t, err)
		run(t, db, "")
	})

	t.Run("postgres", func(t *testing.T) {
		dsn := benchPostgresDSN()
		if dsn == "" {
			t.Skip("set ONEAPI_BENCH_PG_DSN to run the PostgreSQL arm")
		}
		db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
		require.NoError(t, err)
		run(t, db, "SET TIME ZONE 'Asia/Tokyo'")
	})

	t.Run("mysql", func(t *testing.T) {
		dsn := benchMySQLDSN()
		if dsn == "" {
			t.Skip("set ONEAPI_BENCH_MYSQL_DSN to run the MySQL arm")
		}
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		run(t, db, "SET time_zone = '+09:00'")
	})
}

// benchPostgresDSN returns the optional PostgreSQL DSN for engine-specific tests.
//
// Parameters: none.
//
// Return values:
//   - string: the DSN, or "" when the engine is not available.
func benchPostgresDSN() string { return envOrEmpty("ONEAPI_BENCH_PG_DSN") }

// benchMySQLDSN returns the optional MySQL DSN for engine-specific tests.
//
// Parameters: none.
//
// Return values:
//   - string: the DSN, or "" when the engine is not available.
func benchMySQLDSN() string { return envOrEmpty("ONEAPI_BENCH_MYSQL_DSN") }

// envOrEmpty reads an environment variable with the surrounding space trimmed.
//
// Parameters:
//   - name: the variable to read.
//
// Return values:
//   - string: the trimmed value, or "" when unset.
func envOrEmpty(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}
