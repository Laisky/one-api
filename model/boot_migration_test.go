package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// runBootMigration replays the exact migration sequence that InitDB performs
// after the Plan-D refactor: a single AutoMigrate pass followed by idempotent
// STEP 1 / STEP 3 custom migrations. Keeping this mirrored in tests prevents
// future reintroduction of the redundant second AutoMigrate call that caused
// the PR #336 "duplicate column name" regression on SQLite.
func runBootMigration(t *testing.T) {
	t.Helper()
	require.NoError(t, migrateDB(), "STEP 1: migrateDB must succeed")
	require.NoError(t, MigrateAbilitySuspendUntilColumn(), "STEP 2a: ability suspend_until migration")
	require.NoError(t, MigrateChannelFieldsToText(), "STEP 2b: channel field type migration")
	require.NoError(t, MigrateTraceURLColumnToText(), "STEP 2c: trace URL migration")
	require.NoError(t, MigrateUserRequestCostEnsureUniqueRequestID(), "STEP 2d: user_request_costs unique index")
	require.NoError(t, MigrateCustomChannelsToOpenAICompatible(), "STEP 3: custom channel migration")
}

// TestBootMigration_FreshInstall_SingleAutoMigrateCreatesFullSchema proves
// that a single migrateDB() invocation is sufficient to create every column
// in the Log struct — which is the architectural justification for removing
// the second migrateDB() call in InitDB (Plan D).
func TestBootMigration_FreshInstall_SingleAutoMigrateCreatesFullSchema(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	require.NoError(t, migrateDB(), "single migrateDB must succeed on fresh DB")

	// Verify every Log column is present with one single AutoMigrate call.
	expectedLogColumns := []string{
		"id", "user_id", "created_at", "type", "content", "username",
		"token_name", "model_name", "origin_model_name", "quota",
		"prompt_tokens", "completion_tokens", "channel_id",
		"request_id", "trace_id", "updated_at", "elapsed_time",
		"is_stream", "system_prompt_reset", "cached_prompt_tokens", "metadata",
	}
	for _, col := range expectedLogColumns {
		require.True(t,
			db.Migrator().HasColumn(&Log{}, col),
			"column %q must be present after one migrateDB() call", col,
		)
	}

	// Verify the newly-added index is there too (regression for PR #336).
	require.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_origin_model_name"),
		"idx_logs_origin_model_name index must be created on fresh install")
}

// TestBootMigration_Idempotency_FiveRestarts runs the full boot sequence five
// times in a row against the same DB. This is the critical regression guard:
// before Plan D, *two* migrateDB() calls in a single process would crash on
// SQLite with "duplicate column name: origin_model_name". After Plan D we only
// call migrateDB() once per startup, but we still want to make sure *repeated
// full boot cycles* are safe — i.e. no migration step leaves lingering state
// that later cycles trip over.
func TestBootMigration_Idempotency_FiveRestarts(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	for i := 0; i < 5; i++ {
		runBootMigration(t)
	}

	// Schema should still be correct after 5 simulated boots.
	require.True(t, db.Migrator().HasColumn(&Log{}, "origin_model_name"),
		"origin_model_name must still exist after 5 migration cycles")
	require.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_origin_model_name"),
		"idx_logs_origin_model_name must still exist after 5 migration cycles")
}

// TestBootMigration_BackwardCompat_LegacyDBWithoutOriginModelName simulates an
// existing installation that predates PR #336: the `logs` table exists but the
// `origin_model_name` column has never been added, and there is legacy data
// in the table. On the upgraded build, a single boot migration must add the
// new column, create its index, and leave the legacy rows intact (with the
// new column populated from the struct tag's default value).
func TestBootMigration_BackwardCompat_LegacyDBWithoutOriginModelName(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	// Build the "pre-PR-336" logs table DDL by hand — no origin_model_name column.
	legacyDDL := `CREATE TABLE logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
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
		is_stream BOOLEAN DEFAULT 0,
		system_prompt_reset BOOLEAN DEFAULT 0,
		cached_prompt_tokens INTEGER DEFAULT 0,
		metadata TEXT
	)`
	require.NoError(t, db.Exec(legacyDDL).Error, "seeding legacy logs schema")

	// Pre-existing data a real user would have.
	seedSQL := `INSERT INTO logs (user_id, created_at, type, content, username,
		token_name, model_name, quota, prompt_tokens, completion_tokens,
		channel_id, request_id, trace_id, elapsed_time, is_stream,
		system_prompt_reset, cached_prompt_tokens)
		VALUES (42, 1700000000, 2, 'legacy content', 'alice',
			'tok1', 'gpt-4', 1000, 100, 200,
			3, 'req-xyz', 'trace-abc', 500, 0,
			0, 10)`
	require.NoError(t, db.Exec(seedSQL).Error, "seeding legacy log row")

	// Upgrade: run the full boot migration with the new code (single migrateDB call).
	runBootMigration(t)

	// The new column must exist and be indexed.
	require.True(t, db.Migrator().HasColumn(&Log{}, "origin_model_name"),
		"origin_model_name must be added during upgrade")
	require.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_origin_model_name"),
		"idx_logs_origin_model_name must be created during upgrade")

	// Pre-existing row must survive untouched.
	var row struct {
		UserId          int
		ModelName       string
		Username        string
		Quota           int
		OriginModelName string
	}
	require.NoError(t, db.Raw(`SELECT user_id, model_name, username, quota, origin_model_name FROM logs WHERE user_id = 42`).Row().Scan(
		&row.UserId, &row.ModelName, &row.Username, &row.Quota, &row.OriginModelName,
	))
	require.Equal(t, 42, row.UserId)
	require.Equal(t, "gpt-4", row.ModelName)
	require.Equal(t, "alice", row.Username)
	require.Equal(t, 1000, row.Quota)
	// New column picks up the tag default '' for rows that existed before the ALTER.
	require.Equal(t, "", row.OriginModelName,
		"legacy rows must get the struct-default value for the newly-added column")

	// A second boot cycle on the upgraded DB must also succeed (idempotency).
	runBootMigration(t)
	require.True(t, db.Migrator().HasColumn(&Log{}, "origin_model_name"),
		"origin_model_name must persist across restarts")
}

// TestBootMigration_WriteReadLogWithOriginModelName exercises the ORM write
// path end-to-end against the post-migration schema. This is a behavior test
// that confirms nothing about removing the second AutoMigrate broke ORM-level
// inserts/reads on the Log model — in particular, the new OriginModelName
// field round-trips correctly.
func TestBootMigration_WriteReadLogWithOriginModelName(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	runBootMigration(t)

	// Row with explicit OriginModelName.
	mapped := &Log{
		UserId:          7,
		CreatedAt:       1700000001,
		Type:            1,
		Username:        "bob",
		TokenName:       "tok",
		ModelName:       "gpt-4",
		OriginModelName: "my-alias",
		Quota:           500,
	}
	require.NoError(t, db.Create(mapped).Error, "insert log with origin_model_name")

	// Row without OriginModelName (Go zero value "").
	direct := &Log{
		UserId:    8,
		CreatedAt: 1700000002,
		Type:      1,
		Username:  "carol",
		TokenName: "tok2",
		ModelName: "gpt-4",
		Quota:     300,
	}
	require.NoError(t, db.Create(direct).Error, "insert log without explicit origin_model_name")

	// Read back and verify.
	var gotMapped Log
	require.NoError(t, db.First(&gotMapped, mapped.Id).Error)
	require.Equal(t, "my-alias", gotMapped.OriginModelName)
	require.Equal(t, "gpt-4", gotMapped.ModelName)

	var gotDirect Log
	require.NoError(t, db.First(&gotDirect, direct.Id).Error)
	require.Equal(t, "", gotDirect.OriginModelName,
		"empty origin_model_name round-trips as empty string")
	require.Equal(t, "gpt-4", gotDirect.ModelName)

	// Index should enable efficient lookup by origin_model_name — verify
	// the query actually returns the row.
	var byOrigin Log
	require.NoError(t, db.Where("origin_model_name = ?", "my-alias").First(&byOrigin).Error)
	require.Equal(t, mapped.Id, byOrigin.Id)
}

// TestBootMigration_STEP2MigrationsAreIdempotent runs each STEP 2 custom
// migration ten times on a schema produced by a single migrateDB() call.
// This guards against the concern that Plan D (removing the second migrateDB)
// could have relied on STEP 1.b / 1.c etc. being run before the second
// AutoMigrate — we need each STEP 2 migration to be safe to re-run on a fully
// migrated schema without the old "second AutoMigrate" safety net.
func TestBootMigration_STEP2MigrationsAreIdempotent(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	require.NoError(t, migrateDB(), "initial migrateDB")

	for i := 0; i < 10; i++ {
		require.NoError(t, MigrateAbilitySuspendUntilColumn(), "MigrateAbilitySuspendUntilColumn run %d", i+1)
		require.NoError(t, MigrateChannelFieldsToText(), "MigrateChannelFieldsToText run %d", i+1)
		require.NoError(t, MigrateTraceURLColumnToText(), "MigrateTraceURLColumnToText run %d", i+1)
		require.NoError(t, MigrateUserRequestCostEnsureUniqueRequestID(), "MigrateUserRequestCostEnsureUniqueRequestID run %d", i+1)
		require.NoError(t, MigrateCustomChannelsToOpenAICompatible(), "MigrateCustomChannelsToOpenAICompatible run %d", i+1)
	}
}

// TestBootMigration_ReproScenarioFromPR336 is a direct regression test for the
// exact sequence that crashed in PR #336's bug report. Before Plan D, the
// codepath was: migrateDB() (STEP 0) -> custom migrations (STEP 1) -> migrateDB()
// (STEP 2), and STEP 2's AutoMigrate of *Log on SQLite failed with "duplicate
// column name: origin_model_name". After Plan D there is only a single
// migrateDB() call, so the exact failure cannot recur by construction.
// This test exercises the new single-call flow and additionally calls
// migrateDB() a second time to explicitly confirm that even in the worst case
// (someone accidentally reintroduces a second AutoMigrate), SQLite's parseDDL
// handles the state produced by our current struct definitions. If this test
// ever fails again in the future, it is a signal the underlying GORM / SQLite
// driver bug has re-surfaced under a different column and the fix needs
// revisiting.
func TestBootMigration_ReproScenarioFromPR336(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	// New single-call flow.
	require.NoError(t, migrateDB(), "first migrateDB (the only one InitDB now performs)")

	// Simulated accidental second call — must not crash either. This is not
	// required by the current production code path, but it's a defensive
	// guarantee: if someone ever re-adds a second migrateDB(), the code must
	// still be robust. Any failure here means an upstream driver regression.
	require.NoError(t, db.AutoMigrate(&Log{}), "second AutoMigrate on Log must remain safe")
}
