package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestMigrateExternalUUIDsBackfillsLegacyRows verifies own UUID and FK UUID backfills for legacy rows.
func TestMigrateExternalUUIDsBackfillsLegacyRows(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password, inviter_id) VALUES (1, 'root', 'password-hash', 0), (2, 'child', 'password-hash', 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'primary', 'gpt-4o', '{}')").Error)
	require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (1, 1, 'legacy-token-key', 'default')").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, channel_id, type, token_name, content) VALUES (1, 1, 1, 1, 'default', 'legacy log')").Error)
	require.NoError(t, db.Exec("INSERT INTO redemptions (id, user_id, `key`, name) VALUES (1, 1, 'legacy-redemption-key', 'gift')").Error)

	require.NoError(t, MigrateExternalUUIDs(context.Background()))

	var user User
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	requireHyphenatedUUID(t, user.UUID)

	var child User
	require.NoError(t, db.First(&child, "id = ?", 2).Error)
	require.NotNil(t, child.InviterUUID)
	require.Equal(t, user.UUID, *child.InviterUUID)

	var channel Channel
	require.NoError(t, db.First(&channel, "id = ?", 1).Error)
	requireHyphenatedUUID(t, channel.UUID)

	var token Token
	require.NoError(t, db.First(&token, "id = ?", 1).Error)
	requireHyphenatedUUID(t, token.UUID)
	require.NotNil(t, token.UserUUID)
	require.Equal(t, user.UUID, *token.UserUUID)

	var log Log
	require.NoError(t, db.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID)
	require.Equal(t, user.UUID, *log.UserUUID)
	require.NotNil(t, log.ChannelUUID)
	require.Equal(t, channel.UUID, *log.ChannelUUID)
	require.NotNil(t, log.TokenUUID)
	require.Equal(t, token.UUID, *log.TokenUUID)

	created := &Channel{Name: "created", Type: 1, Models: "gpt-4o", Config: "{}"}
	require.NoError(t, db.Create(created).Error)
	requireHyphenatedUUID(t, created.UUID)

	for _, target := range primaryUUIDBackfillTargets() {
		requireUUIDUniqueIndex(t, db, target)
	}
	require.Error(t, db.Model(&User{}).Where("id = ?", child.Id).Update("uuid", user.UUID).Error)
}

// TestMigrateLogExternalUUIDsBackfillsTokenTransactionLogUUIDFromSplitLogDB verifies split LOG_DB log references.
func TestMigrateLogExternalUUIDsBackfillsTokenTransactionLogUUIDFromSplitLogDB(t *testing.T) {
	primaryDB := setupMigrationTestDB(t)
	logDB := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = primaryDB
	LOG_DB = logDB
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	require.NoError(t, primaryDB.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash')").Error)
	require.NoError(t, primaryDB.Exec("INSERT INTO channels (id, uuid, type, name, models, config) VALUES (1, '018f0000-0000-7000-8000-000000000002', 1, 'primary', 'gpt-4o', '{}')").Error)
	require.NoError(t, primaryDB.Exec("INSERT INTO token_transactions (id, transaction_id, token_id, user_id, status, pre_quota, log_id) VALUES (1, 'txn-split-log', 1, 1, 1, 10, 77)").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, user_id, channel_id, type, content) VALUES (77, 1, 1, 1, 'split log')").Error)

	require.NoError(t, MigrateLogExternalUUIDs(context.Background(), logDB))

	var log Log
	require.NoError(t, logDB.First(&log, "id = ?", 77).Error)
	requireHyphenatedUUID(t, log.UUID)

	var txn TokenTransaction
	require.NoError(t, primaryDB.First(&txn, "id = ?", 1).Error)
	require.NotNil(t, txn.LogUUID)
	require.Equal(t, log.UUID, *txn.LogUUID)
	requireUUIDUniqueIndex(t, logDB, uuidBackfillTarget{table: "logs", model: &Log{}})
	require.Error(t, logDB.Exec("INSERT INTO logs (id, uuid, user_id, channel_id, type, content) VALUES (78, ?, 1, 1, 1, 'duplicate log')", log.UUID).Error)
}

// TestEnsureUUIDUniqueIndexesDefersMissingUUIDs verifies unique promotion is gated by completed backfill.
func TestEnsureUUIDUniqueIndexesDefersMissingUUIDs(t *testing.T) {
	db := setupMigrationTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)

	err := ensureUUIDUniqueIndexes(context.Background(), db, []uuidBackfillTarget{{table: "users", model: &User{}}})
	require.NoError(t, err)
	require.False(t, db.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")))

	require.NoError(t, db.Model(&User{}).Where("id = ?", 1).Update("uuid", "018f0000-0000-7000-8000-000000000001").Error)
	require.NoError(t, ensureUUIDUniqueIndexes(context.Background(), db, []uuidBackfillTarget{{table: "users", model: &User{}}}))
	requireUUIDUniqueIndex(t, db, uuidBackfillTarget{table: "users", model: &User{}})
}

// TestMigrateExternalUUIDsRollingWindowT18 verifies old-writer rows are tolerated and swept.
func TestMigrateExternalUUIDsRollingWindowT18(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())
	userUUID := "018f0000-0000-7000-8000-000000000101"
	require.NoError(t, db.Table("users").Create(map[string]any{
		"id": 1, "uuid": userUUID, "username": "root", "password": "password-hash",
	}).Error)
	require.NoError(t, db.Table("tokens").Create(map[string]any{
		"id": 1, "uuid": "018f0000-0000-7000-8000-000000000102", "user_id": 1, "user_uuid": userUUID, "key": "new-token-key", "name": "new",
	}).Error)
	require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (2, 1, 'old-token-key', 'old-slave')").Error)

	missing, err := countMissingUUIDs(context.Background(), db, "tokens")
	require.NoError(t, err)
	require.EqualValues(t, 1, missing)
	require.NoError(t, ensureUUIDUniqueIndexes(context.Background(), db, []uuidBackfillTarget{{table: "tokens", model: &Token{}}}))
	require.False(t, db.Migrator().HasIndex(&Token{}, uuidUniqueIndexName("tokens")))

	require.NoError(t, MigrateExternalUUIDs(context.Background()))

	var oldToken Token
	require.NoError(t, db.First(&oldToken, "id = ?", 2).Error)
	requireHyphenatedUUID(t, oldToken.UUID)
	require.NotNil(t, oldToken.UserUUID)
	require.Equal(t, userUUID, *oldToken.UserUUID)
	requireUUIDUniqueIndex(t, db, uuidBackfillTarget{table: "tokens", model: &Token{}})
}

// TestHasPrimaryExternalUUIDBackfillIgnoresOrphanFKRows verifies orphan FK rows do not trigger repeated startup backfills.
func TestHasPrimaryExternalUUIDBackfillIgnoresOrphanFKRows(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())
	require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash')").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, uuid, user_id, type, content) VALUES (1, '018f0000-0000-7000-8000-000000000002', 999, 1, 'orphan log')").Error)

	needsBackfill, err := hasPrimaryExternalUUIDBackfill(context.Background())
	require.NoError(t, err)
	require.False(t, needsBackfill)
}

// TestHasPrimaryExternalUUIDBackfillDetectsFillableFKRows verifies valid missing FK UUID rows trigger backfill work.
func TestHasPrimaryExternalUUIDBackfillDetectsFillableFKRows(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())
	require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash')").Error)
	require.NoError(t, db.Exec("INSERT INTO tokens (id, uuid, user_id, `key`, name) VALUES (1, '018f0000-0000-7000-8000-000000000002', 1, 'legacy-token-key', 'default')").Error)

	needsBackfill, err := hasPrimaryExternalUUIDBackfill(context.Background())
	require.NoError(t, err)
	require.True(t, needsBackfill)

	require.NoError(t, MigrateExternalUUIDs(context.Background()))
	var token Token
	require.NoError(t, db.First(&token, "id = ?", 1).Error)
	require.NotNil(t, token.UserUUID)
	require.Equal(t, "018f0000-0000-7000-8000-000000000001", *token.UserUUID)
}

// TestHasMissingFKUUIDCandidateIgnoresZeroNullableFK verifies zero nullable FKs do not trigger backfill work.
func TestHasMissingFKUUIDCandidateIgnoresZeroNullableFK(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())
	require.NoError(t, db.Exec("INSERT INTO token_transactions (id, uuid, transaction_id, status, pre_quota, log_id) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'zero-log-id', 1, 10, 0)").Error)

	hasCandidate, err := hasMissingFKUUIDCandidate(context.Background(), db, uuidRefProbeTarget{
		table:      "token_transactions",
		model:      &TokenTransaction{},
		fkColumn:   "log_id",
		uuidColumn: "log_uuid",
		nullableFK: true,
	})
	require.NoError(t, err)
	require.False(t, hasCandidate)
}

// TestNewBinaryBeforeMasterMigrationFailsT18 documents the master-first rolling-upgrade rule.
func TestNewBinaryBeforeMasterMigrationFailsT18(t *testing.T) {
	db := setupMigrationTestDB(t)
	require.NoError(t, db.Exec("CREATE TABLE users (id integer primary key autoincrement, username text, password text)").Error)

	err := db.Create(&User{Username: "new-node", Password: "password-hash"}).Error
	require.Error(t, err)

	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'old-node', 'password-hash')").Error)
	var users []User
	err = db.Omit("password").Find(&users).Error
	require.Error(t, err)
}

// TestMigrateExternalUUIDsMultiDB verifies UUID backfill and unique promotion on live MySQL/PostgreSQL.
func TestMigrateExternalUUIDsMultiDB(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		dialect := dialect
		t.Run(dialect, func(t *testing.T) {
			originalDB := DB
			originalLOGDB := LOG_DB
			db := openBackend(t, dialect)
			if db == nil {
				t.Skipf("%s DSN not set, skipping UUID migration matrix test", dialect)
			}
			DB = db
			LOG_DB = db
			t.Cleanup(func() {
				DB = originalDB
				LOG_DB = originalLOGDB
				resetBackendFlags()
			})

			dropUUIDMigrationTables(t, db)
			require.NoError(t, migrateDB())
			seedLegacyUUIDRows(t, db)

			require.NoError(t, MigrateExternalUUIDs(context.Background()))

			var user User
			require.NoError(t, db.First(&user, "id = ?", 1).Error)
			requireHyphenatedUUID(t, user.UUID)

			var token Token
			require.NoError(t, db.First(&token, "id = ?", 1).Error)
			require.NotNil(t, token.UserUUID)
			require.Equal(t, user.UUID, *token.UserUUID)

			for _, target := range primaryUUIDBackfillTargets() {
				requireUUIDUniqueIndex(t, db, target)
			}
			require.Error(t, db.Model(&User{}).Where("id = ?", 2).Update("uuid", user.UUID).Error)
		})
	}
}

// TestBackfillFKUUIDsSkipsOrphanRows verifies orphaned integer references do not loop forever.
func TestBackfillFKUUIDsSkipsOrphanRows(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())
	require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash')").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (1, 999, 1, 'orphan log'), (2, 1, 1, 'fillable log')").Error)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := backfillFKUUIDs(ctx, db, uuidRefTarget{
		table:      "logs",
		model:      &Log{},
		fkColumn:   "user_id",
		uuidColumn: "user_uuid",
		refs:       map[int]string{1: "018f0000-0000-7000-8000-000000000001"},
	})
	require.NoError(t, err)

	var orphan Log
	require.NoError(t, db.First(&orphan, "id = ?", 1).Error)
	require.Nil(t, orphan.UserUUID)

	var fillable Log
	require.NoError(t, db.First(&fillable, "id = ?", 2).Error)
	require.NotNil(t, fillable.UserUUID)
	require.Equal(t, "018f0000-0000-7000-8000-000000000001", *fillable.UserUUID)
}

// requireHyphenatedUUID asserts that uuid looks like the canonical external UUID form.
func requireHyphenatedUUID(t *testing.T, uuid string) {
	t.Helper()
	require.Len(t, uuid, 36)
	require.Equal(t, 4, strings.Count(uuid, "-"))
}

// requireUUIDUniqueIndex asserts that the target table has its explicit UUID unique index.
func requireUUIDUniqueIndex(t *testing.T, db *gorm.DB, target uuidBackfillTarget) {
	t.Helper()
	require.True(t, db.Migrator().HasIndex(target.model, uuidUniqueIndexName(target.table)), "missing UUID unique index for %s", target.table)
}

// dropUUIDMigrationTables clears UUID-migrated tables for live-database matrix tests.
func dropUUIDMigrationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Migrator().DropTable(
		&PasskeyCredential{},
		&MCPTool{},
		&MCPServer{},
		&AsyncTaskBinding{},
		&Trace{},
		&UserRequestCost{},
		&TokenTransaction{},
		&Log{},
		&Redemption{},
		&Ability{},
		&Option{},
		&Token{},
		&Channel{},
		&User{},
	))
}

// seedLegacyUUIDRows inserts rows without UUID values to simulate a pre-migration database.
func seedLegacyUUIDRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Table("users").Create([]map[string]any{
		{"id": 1, "username": "root", "password": "password-hash", "inviter_id": 0},
		{"id": 2, "username": "child", "password": "password-hash", "inviter_id": 1},
	}).Error)
	require.NoError(t, db.Table("channels").Create(map[string]any{
		"id": 1, "type": 1, "name": "primary", "models": "gpt-4o", "config": "{}",
	}).Error)
	require.NoError(t, db.Table("tokens").Create(map[string]any{
		"id": 1, "user_id": 1, "key": "legacy-token-key", "name": "default",
	}).Error)
	require.NoError(t, db.Table("logs").Create(map[string]any{
		"id": 1, "user_id": 1, "channel_id": 1, "type": 1, "token_name": "default", "content": "legacy log",
	}).Error)
	require.NoError(t, db.Table("redemptions").Create(map[string]any{
		"id": 1, "user_id": 1, "key": "legacy-redemption-key", "name": "gift",
	}).Error)
}
