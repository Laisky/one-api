package controller

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// openSettlementTestDatabase provides a durable SQLite fixture by default.
// ONEAPI_SETTLEMENT_TEST_BACKEND=mysql/postgres and ONEAPI_SETTLEMENT_TEST_DSN
// instead exercise a live disposable engine. Each live subtest creates and
// drops ONLY its own randomly named database, never resets an existing one,
// and fails rather than skips when the requested backend is unavailable.
func openSettlementTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	backend := os.Getenv("ONEAPI_SETTLEMENT_TEST_BACKEND")
	if backend == "" || backend == "sqlite" {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "settlement.db")+"?_busy_timeout=10000"), &gorm.Config{})
		require.NoError(t, err)
		return db
	}
	require.Contains(t, []string{"mysql", "postgres"}, backend)
	dsn := os.Getenv("ONEAPI_SETTLEMENT_TEST_DSN")
	require.NotEmpty(t, dsn, "live engine qualification requires an explicit disposable-server DSN")
	name := "settlement_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var admin, db *gorm.DB
	var err error
	if backend == "mysql" {
		configuration, parseErr := gomysql.ParseDSN(dsn)
		require.NoError(t, parseErr)
		configuration.ParseTime = true
		admin, err = gorm.Open(mysql.Open(configuration.FormatDSN()), &gorm.Config{})
		require.NoError(t, err)
		configuration.DBName = name
		dsn = configuration.FormatDSN()
	} else {
		parsed, parseErr := url.Parse(dsn)
		require.NoError(t, parseErr)
		require.Contains(t, []string{"postgres", "postgresql"}, parsed.Scheme, "use a PostgreSQL URI for live qualification")
		admin, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		parsed.Path = "/" + name
		dsn = parsed.String()
	}
	adminPool, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminPool.Close()) })
	// name consists solely of this function's fixed prefix and random hex.
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name).Error) })
	if backend == "mysql" {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	} else {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	}
	require.NoError(t, err)
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()
	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(backend == "mysql")
	common.UsingPostgreSQL.Store(backend == "postgres")
	t.Cleanup(func() {
		common.UsingSQLite.Store(oldSQLite)
		common.UsingMySQL.Store(oldMySQL)
		common.UsingPostgreSQL.Store(oldPostgres)
	})
	t.Logf("executing external settlement against live %s", db.Dialector.Name())
	return db
}
