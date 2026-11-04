package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestProbeBenchDatabaseName verifies names are safe SQL identifiers and distinct.
func TestProbeBenchDatabaseName(t *testing.T) {
	a, err := probeBenchDatabaseName()
	require.NoError(t, err)
	b, err := probeBenchDatabaseName()
	require.NoError(t, err)
	require.Regexp(t, "^oneapi_probe_bench_[0-9a-f]{32}$", a)
	require.NotEqual(t, a, b)
}

// TestProbeBenchPreservesSource verifies a disposable server's pre-existing
// sentinel tables remain intact during and after benchmark database cleanup.
// Set ONEAPI_PROBE_BENCH_ISOLATION_TEST=1 and provide server DSNs whose
// channels and abilities tables each contain a single sentinel integer 42.
func TestProbeBenchPreservesSource(t *testing.T) {
	if os.Getenv("ONEAPI_PROBE_BENCH_ISOLATION_TEST") != "1" {
		t.Skip("requires explicitly prepared disposable server fixtures")
	}
	for _, item := range []struct {
		backend, env string
	}{
		{"mysql", "MYSQL_DSN"},
		{"postgres", "PG_DSN"},
	} {
		t.Run(item.backend, func(t *testing.T) {
			dsn := os.Getenv(item.env)
			if dsn == "" {
				t.Skipf("%s not set", item.env)
			}
			var dialector gorm.Dialector
			if item.backend == "mysql" {
				dialector = mysql.Open(dsn)
			} else {
				dialector = postgres.Open(dsn)
			}
			source, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := source.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			check := func(t *testing.T) {
				t.Helper()
				for _, table := range []string{"channels", "abilities"} {
					var values []int
					require.NoError(t, source.Table(table).Pluck("sentinel", &values).Error)
					require.Equal(t, []int{42}, values)
				}
			}
			check(t)
			var isolatedName string
			t.Run("isolated", func(t *testing.T) {
				isolated := openProbeBenchDB(t, item.backend)
				query := "SELECT DATABASE()"
				if item.backend == "postgres" {
					query = "SELECT current_database()"
				}
				require.NoError(t, isolated.Raw(query).Scan(&isolatedName).Error)
				require.Regexp(t, "^oneapi_probe_bench_[0-9a-f]{32}$", isolatedName)
				require.True(t, isolated.Migrator().HasTable(&Channel{}))
				require.True(t, isolated.Migrator().HasTable(&Ability{}))
				check(t)
			})
			check(t)
			var count int64
			query := "SELECT count(*) FROM information_schema.schemata WHERE schema_name = ?"
			if item.backend == "postgres" {
				query = "SELECT count(*) FROM pg_database WHERE datname = ?"
			}
			require.NoError(t, source.Raw(query, isolatedName).Scan(&count).Error)
			require.Zero(t, count, "isolated database must be cleaned up")
		})
	}
}
