package model

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
)

// openProbeBenchDB opens and migrates a database for the probe-write benchmark.
// Parameters: b is the benchmark handle and backend is "sqlite", "mysql" (MYSQL_DSN), or
// "postgres" (PG_DSN).
// Returns: the database; the benchmark is skipped when a server DSN is not set.
func openProbeBenchDB(b *testing.B, backend string) *gorm.DB {
	b.Helper()
	var dialector gorm.Dialector
	switch backend {
	case "sqlite":
		dialector = sqlite.Open(b.TempDir() + "/probe-bench.db")
	case "mysql":
		if os.Getenv("MYSQL_DSN") == "" {
			b.Skip("MYSQL_DSN not set")
		}
		dialector = mysql.Open(os.Getenv("MYSQL_DSN"))
	case "postgres":
		if os.Getenv("PG_DSN") == "" {
			b.Skip("PG_DSN not set")
		}
		dialector = postgres.Open(os.Getenv("PG_DSN"))
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(b, err)
	if backend == "sqlite" {
		sqlDB, err := db.DB()
		require.NoError(b, err)
		sqlDB.SetMaxOpenConns(1)
	} else {
		_ = db.Migrator().DropTable("abilities", "channels")
		b.Cleanup(func() { _ = db.Migrator().DropTable("abilities", "channels") })
	}
	require.NoError(b, db.AutoMigrate(&Channel{}, &Ability{}))
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
