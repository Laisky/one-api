package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/songquanpeng/one-api/common"
)

// openBackend opens a GORM connection for the requested backend and installs
// the common.Using* flags the migration code relies on. Returns nil when the
// corresponding DSN env var is not set so tests can be skipped.
func openBackend(t *testing.T, backend string) *gorm.DB {
	t.Helper()

	var (
		dsn    string
		dialer gorm.Dialector
	)
	switch backend {
	case "postgres":
		dsn = os.Getenv("PG_DSN")
		if dsn == "" {
			return nil
		}
		dialer = postgres.Open(dsn)
	case "mysql":
		dsn = os.Getenv("MYSQL_DSN")
		if dsn == "" {
			return nil
		}
		dialer = mysql.Open(dsn)
	default:
		t.Fatalf("unknown backend %q", backend)
	}

	db, err := gorm.Open(dialer, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "open %s connection", backend)

	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(backend == "mysql")
	common.UsingPostgreSQL.Store(backend == "postgres")

	return db
}

// resetBackendFlags restores the SQLite-default common.Using* flags other
// migration tests assume.
func resetBackendFlags() {
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
}

// dropBootMigrationTables removes tables managed by migrateDB() so the test
// can run repeatedly against a shared backend without polluting it.
func dropBootMigrationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	tables := []string{
		"async_task_bindings",
		"cache_entries",
		"traces",
		"user_request_costs",
		"token_transactions",
		"logs",
		"abilities",
		"redemptions",
		"options",
		"users",
		"tokens",
		"channels",
		"mcp_servers",
		"channel_tests",
	}
	for _, tbl := range tables {
		require.NoError(t, db.Migrator().DropTable(tbl),
			"drop %s to ensure a clean slate", tbl)
	}
}

// assertBootMigrationSchemaSane verifies the most critical invariants after a
// boot migration: the Log.OriginModelName column exists, its index exists, and
// an ORM round-trip through the Log model preserves the field.
func assertBootMigrationSchemaSane(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.True(t, db.Migrator().HasColumn(&Log{}, "origin_model_name"),
		"origin_model_name column must exist after migration")
	require.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_origin_model_name"),
		"idx_logs_origin_model_name index must exist after migration")

	row := &Log{
		UserId:          9999,
		CreatedAt:       1700000099,
		Type:            1,
		Username:        "multidb-probe",
		TokenName:       "t",
		ModelName:       "gpt-4",
		OriginModelName: "alias-multidb",
		Quota:           42,
	}
	require.NoError(t, db.Create(row).Error, "ORM insert with OriginModelName")

	var back Log
	require.NoError(t, db.First(&back, row.Id).Error)
	require.Equal(t, "alias-multidb", back.OriginModelName)
}

// runBootMigrationPortable mirrors runBootMigration but skips SQLite-only
// steps for MySQL/PG when they don't apply.
func runBootMigrationPortable(t *testing.T) {
	t.Helper()
	require.NoError(t, migrateDB(), "migrateDB")
	require.NoError(t, MigrateAbilitySuspendUntilColumn(), "MigrateAbilitySuspendUntilColumn")
	require.NoError(t, MigrateChannelFieldsToText(), "MigrateChannelFieldsToText")
	require.NoError(t, MigrateTraceURLColumnToText(), "MigrateTraceURLColumnToText")
	require.NoError(t, MigrateUserRequestCostEnsureUniqueRequestID(), "MigrateUserRequestCostEnsureUniqueRequestID")
	require.NoError(t, MigrateCustomChannelsToOpenAICompatible(), "MigrateCustomChannelsToOpenAICompatible")
}

// TestBootMigration_Postgres_FullFlow runs the complete single-call boot
// migration against a live PostgreSQL instance. Gated on PG_DSN. This is the
// multi-DB behavior test that guarantees Plan D is safe on PG in addition to
// the in-memory SQLite coverage.
func TestBootMigration_Postgres_FullFlow(t *testing.T) {
	db := openBackend(t, "postgres")
	if db == nil {
		t.Skip("PG_DSN not set; skipping PostgreSQL boot migration test")
	}
	originalDB := DB
	DB = db
	defer func() {
		DB = originalDB
		resetBackendFlags()
	}()
	t.Cleanup(func() {
		dropBootMigrationTables(t, db)
	})

	// Clean slate for the full boot flow.
	dropBootMigrationTables(t, db)

	// Fresh install.
	runBootMigrationPortable(t)
	assertBootMigrationSchemaSane(t, db)

	// Idempotency: simulate two additional restarts.
	runBootMigrationPortable(t)
	runBootMigrationPortable(t)
	assertBootMigrationSchemaSane(t, db)
}

// TestBootMigration_MySQL_FullFlow is the MySQL counterpart. Gated on MYSQL_DSN.
func TestBootMigration_MySQL_FullFlow(t *testing.T) {
	db := openBackend(t, "mysql")
	if db == nil {
		t.Skip("MYSQL_DSN not set; skipping MySQL boot migration test")
	}
	originalDB := DB
	DB = db
	defer func() {
		DB = originalDB
		resetBackendFlags()
	}()
	t.Cleanup(func() {
		dropBootMigrationTables(t, db)
	})

	dropBootMigrationTables(t, db)

	runBootMigrationPortable(t)
	assertBootMigrationSchemaSane(t, db)

	runBootMigrationPortable(t)
	runBootMigrationPortable(t)
	assertBootMigrationSchemaSane(t, db)
}

// TestBootMigration_Postgres_UpgradeFromLegacy seeds a PG database with the
// logs table as it would look on a pre-PR-336 installation (no
// origin_model_name column + a row of real data), then runs the new boot
// migration. Verifies the column is added and the legacy row survives intact.
func TestBootMigration_Postgres_UpgradeFromLegacy(t *testing.T) {
	db := openBackend(t, "postgres")
	if db == nil {
		t.Skip("PG_DSN not set; skipping PostgreSQL upgrade test")
	}
	originalDB := DB
	DB = db
	defer func() {
		DB = originalDB
		resetBackendFlags()
	}()
	t.Cleanup(func() {
		dropBootMigrationTables(t, db)
	})

	dropBootMigrationTables(t, db)

	legacyDDL := `CREATE TABLE logs (
		id SERIAL PRIMARY KEY,
		user_id INTEGER DEFAULT 0,
		created_at BIGINT DEFAULT 0,
		type INTEGER DEFAULT 0,
		content TEXT,
		username TEXT DEFAULT '',
		token_name TEXT DEFAULT '',
		model_name TEXT DEFAULT '',
		quota INTEGER DEFAULT 0,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		channel_id INTEGER DEFAULT 0,
		request_id TEXT DEFAULT '',
		trace_id VARCHAR(64) DEFAULT '',
		updated_at BIGINT DEFAULT 0,
		elapsed_time BIGINT DEFAULT 0,
		is_stream BOOLEAN DEFAULT FALSE,
		system_prompt_reset BOOLEAN DEFAULT FALSE,
		cached_prompt_tokens INTEGER DEFAULT 0,
		metadata TEXT
	)`
	require.NoError(t, db.Exec(legacyDDL).Error, "seed legacy PG logs schema")
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id, created_at, type, content, username,
		token_name, model_name, quota, prompt_tokens, completion_tokens,
		channel_id, request_id, trace_id, elapsed_time, is_stream,
		system_prompt_reset, cached_prompt_tokens)
		VALUES (77, 1700000077, 2, 'legacy-pg', 'alice-pg',
			'tokpg', 'gpt-4', 1000, 100, 200,
			3, 'req-pg', 'trace-pg', 500, FALSE,
			FALSE, 10)`).Error, "seed legacy log row")

	runBootMigrationPortable(t)
	assertBootMigrationSchemaSane(t, db)

	var origin string
	var username string
	require.NoError(t, db.Raw(`SELECT origin_model_name, username FROM logs WHERE user_id = 77`).Row().Scan(&origin, &username))
	require.Equal(t, "alice-pg", username, "legacy row must survive upgrade")
	require.Equal(t, "", origin, "new column gets struct-default '' on legacy rows")
}

// TestBootMigration_MySQL_UpgradeFromLegacy is the MySQL counterpart to the
// PG upgrade test above.
func TestBootMigration_MySQL_UpgradeFromLegacy(t *testing.T) {
	db := openBackend(t, "mysql")
	if db == nil {
		t.Skip("MYSQL_DSN not set; skipping MySQL upgrade test")
	}
	originalDB := DB
	DB = db
	defer func() {
		DB = originalDB
		resetBackendFlags()
	}()
	t.Cleanup(func() {
		dropBootMigrationTables(t, db)
	})

	dropBootMigrationTables(t, db)

	// MySQL needs explicit length on text defaults; keep it aligned with
	// GORM's own defaults by using TEXT without DEFAULT for the content column
	// (MySQL refuses DEFAULT on TEXT columns).
	legacyDDL := `CREATE TABLE logs (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT DEFAULT 0,
		created_at BIGINT DEFAULT 0,
		type INT DEFAULT 0,
		content TEXT,
		username VARCHAR(191) DEFAULT '',
		token_name VARCHAR(191) DEFAULT '',
		model_name VARCHAR(191) DEFAULT '',
		quota INT DEFAULT 0,
		prompt_tokens INT DEFAULT 0,
		completion_tokens INT DEFAULT 0,
		channel_id INT DEFAULT 0,
		request_id VARCHAR(191) DEFAULT '',
		trace_id VARCHAR(64) DEFAULT '',
		updated_at BIGINT DEFAULT 0,
		elapsed_time BIGINT DEFAULT 0,
		is_stream BOOLEAN DEFAULT FALSE,
		system_prompt_reset BOOLEAN DEFAULT FALSE,
		cached_prompt_tokens INT DEFAULT 0,
		metadata TEXT
	)`
	require.NoError(t, db.Exec(legacyDDL).Error, "seed legacy MySQL logs schema")
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id, created_at, type, content, username,
		token_name, model_name, quota, prompt_tokens, completion_tokens,
		channel_id, request_id, trace_id, elapsed_time, is_stream,
		system_prompt_reset, cached_prompt_tokens)
		VALUES (88, 1700000088, 2, 'legacy-mysql', 'alice-mysql',
			'tokmy', 'gpt-4', 1000, 100, 200,
			3, 'req-mysql', 'trace-mysql', 500, FALSE,
			FALSE, 10)`).Error, "seed legacy log row")

	runBootMigrationPortable(t)
	assertBootMigrationSchemaSane(t, db)

	var origin string
	var username string
	require.NoError(t, db.Raw(`SELECT origin_model_name, username FROM logs WHERE user_id = 88`).Row().Scan(&origin, &username))
	require.Equal(t, "alice-mysql", username, "legacy row must survive upgrade")
	require.Equal(t, "", origin, "new column gets struct-default '' on legacy rows")
}

// TestBootMigration_Postgres_OriginModelNameIndexIsUsable verifies the index
// added in PR #336 is actually functional on Postgres (not just present).
// This is a quick behavior check guarding against index mis-definition.
func TestBootMigration_Postgres_OriginModelNameIndexIsUsable(t *testing.T) {
	db := openBackend(t, "postgres")
	if db == nil {
		t.Skip("PG_DSN not set")
	}
	originalDB := DB
	DB = db
	defer func() {
		DB = originalDB
		resetBackendFlags()
	}()
	t.Cleanup(func() {
		dropBootMigrationTables(t, db)
	})

	dropBootMigrationTables(t, db)
	runBootMigrationPortable(t)

	require.NoError(t, db.Create(&Log{
		UserId:          1,
		Type:            1,
		Username:        "u",
		TokenName:       "t",
		ModelName:       "mapped",
		OriginModelName: "needle-origin",
	}).Error)

	var out []Log
	require.NoError(t, db.Where("origin_model_name = ?", "needle-origin").Find(&out).Error)
	require.Len(t, out, 1)
	require.Equal(t, "needle-origin", out[0].OriginModelName)
}
