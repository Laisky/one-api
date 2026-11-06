package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
)

// probeBenchDatabaseName creates an unpredictable SQL identifier for a disposable
// benchmark database. Parameters: none. Returns: the identifier and any random-source error.
func probeBenchDatabaseName() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "oneapi_probe_bench_" + hex.EncodeToString(random[:]), nil
}

// openProbeBenchDB opens an isolated database and migrates the benchmark tables.
// Server DSNs identify an administrative connection; the benchmark creates and
// removes only a fresh, randomly named database, never tables in the DSN database.
// Parameters: tb is the test handle and backend is sqlite, mysql, or postgres.
// Returns: the isolated handle; server arms skip when their DSN is not set.
func openProbeBenchDB(tb testing.TB, backend string) *gorm.DB {
	tb.Helper()
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	if backend == "sqlite" {
		db, err := gorm.Open(sqlite.Open(tb.TempDir()+"/probe-bench.db"), cfg)
		require.NoError(tb, err)
		sqlDB, err := db.DB()
		require.NoError(tb, err)
		sqlDB.SetMaxOpenConns(1)
		tb.Cleanup(func() { require.NoError(tb, sqlDB.Close()) })
		require.NoError(tb, db.AutoMigrate(&Channel{}, &ChannelGroup{}, &Ability{}))
		return db
	}

	env := "MYSQL_DSN"
	if backend == "postgres" {
		env = "PG_DSN"
	} else if backend != "mysql" {
		tb.Fatalf("unsupported benchmark backend %q", backend)
	}
	dsn := os.Getenv(env)
	if dsn == "" {
		tb.Skipf("%s not set", env)
	}
	name, err := probeBenchDatabaseName()
	require.NoError(tb, err)

	var admin, db *gorm.DB
	switch backend {
	case "mysql":
		parsed, parseErr := mysqlDriver.ParseDSN(dsn)
		require.NoError(tb, parseErr)
		admin, err = gorm.Open(mysql.Open(dsn), cfg)
		require.NoError(tb, err)
		adminSQL, adminErr := admin.DB()
		require.NoError(tb, adminErr)
		tb.Cleanup(func() { require.NoError(tb, adminSQL.Close()) })
		// The name is generated from a fixed prefix and hexadecimal random bytes.
		require.NoError(tb, admin.Exec("CREATE DATABASE `"+name+"`").Error)
		tb.Cleanup(func() {
			if db != nil {
				sqlDB, closeErr := db.DB()
				require.NoError(tb, closeErr)
				require.NoError(tb, sqlDB.Close())
			}
			require.NoError(tb, admin.Exec("DROP DATABASE `"+name+"`").Error)
		})
		parsed.DBName = name
		db, err = gorm.Open(mysql.Open(parsed.FormatDSN()), cfg)
	case "postgres":
		parsed, parseErr := pgx.ParseConfig(dsn)
		require.NoError(tb, parseErr)
		admin, err = gorm.Open(postgres.Open(dsn), cfg)
		require.NoError(tb, err)
		adminSQL, adminErr := admin.DB()
		require.NoError(tb, adminErr)
		tb.Cleanup(func() { require.NoError(tb, adminSQL.Close()) })
		require.NoError(tb, admin.Exec("CREATE DATABASE \""+name+"\"").Error)
		tb.Cleanup(func() {
			if db != nil {
				sqlDB, closeErr := db.DB()
				require.NoError(tb, closeErr)
				require.NoError(tb, sqlDB.Close())
			}
			require.NoError(tb, admin.Exec("DROP DATABASE \""+name+"\"").Error)
		})
		parsed.Database = name
		db, err = gorm.Open(postgres.New(postgres.Config{Conn: stdlib.OpenDB(*parsed)}), cfg)
	}
	require.NoError(tb, err)
	require.NoError(tb, db.AutoMigrate(&Channel{}, &ChannelGroup{}, &Ability{}))
	return db
}

// BenchmarkChannelProbeWrites measures the per-channel database work of one sweep
// iteration for a healthy channel: main's path (an unconditional response-time write)
// against the current path (a full reload plus the atomic, revision-checked apply, whose
// response-time write also takes the row lock). It excludes the upstream probe itself.
// Run: MYSQL_DSN=... PG_DSN=... go test ./model/ -run '^$' -bench BenchmarkChannelProbeWrites
// Parameters: b is the benchmark handle. Returns: no values.
func BenchmarkChannelProbeWrites(b *testing.B) {
	for _, backend := range []string{"sqlite", "mysql", "postgres"} {
		b.Run(backend, func(b *testing.B) {
			originalDB := DB
			originalSQLite, originalMySQL, originalPostgreSQL := common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()
			b.Cleanup(func() {
				DB = originalDB
				common.UsingSQLite.Store(originalSQLite)
				common.UsingMySQL.Store(originalMySQL)
				common.UsingPostgreSQL.Store(originalPostgreSQL)
			})
			DB = openProbeBenchDB(b, backend)
			common.UsingSQLite.Store(backend == "sqlite")
			common.UsingMySQL.Store(backend == "mysql")
			common.UsingPostgreSQL.Store(backend == "postgres")
			ctx := context.Background()
			channel := &Channel{Name: "probe-bench", Key: "sk-bench", Models: "gpt-4o-mini,gpt-4o", Group: "default",
				Status: ChannelStatusEnabled}
			require.NoError(b, channel.Insert())

			b.Run("main-unconditional-write", func(b *testing.B) {
				for b.Loop() {
					channel.UpdateResponseTimeWithContext(ctx, 42)
				}
			})
			b.Run("current-reload-then-atomic-apply", func(b *testing.B) {
				for b.Loop() {
					probed := &Channel{Id: channel.Id}
					require.NoError(b, probed.ReloadForProbeWithContext(ctx))
					_, err := ApplyChannelProbeResultWithContext(ctx, channel.Id, ChannelProbeResult{
						Revision: probed.ProbeConfigRevision(), ObservedStatus: probed.Status, ResponseTimeMs: 42})
					require.NoError(b, err)
				}
			})
		})
	}
}
