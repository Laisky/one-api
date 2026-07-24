package model

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// newBackfillTestDB opens an isolated in-memory SQLite database. MaxOpenConns(1)
// pins every query to the same connection so the shared-nothing ":memory:" DB is
// not silently recreated per pooled connection.
func newBackfillTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() { common.UsingSQLite.Store(originalSQLite) })

	return db
}

func abilityWeights(t *testing.T, db *gorm.DB) map[int]sql.NullInt64 {
	t.Helper()
	type row struct {
		ChannelId int           `gorm:"column:channel_id"`
		Weight    sql.NullInt64 `gorm:"column:weight"`
	}
	var rows []row
	require.NoError(t, db.Raw("SELECT channel_id, weight FROM abilities ORDER BY channel_id").Scan(&rows).Error)
	out := make(map[int]sql.NullInt64, len(rows))
	for _, r := range rows {
		out[r.ChannelId] = r.Weight
	}
	return out
}

// TestMigrateAbilityWeightBackfill_UpgradeFromOldSchema is the regression guard for
// the "backfill is a silent no-op" bug: on an upgrade, AutoMigrate must add
// abilities.weight as NULL for historical rows so the backfill can copy the parent
// channel's weight. If the column is (re)introduced with a non-NULL default, this
// test fails because every historical row already reads 0.
func TestMigrateAbilityWeightBackfill_UpgradeFromOldSchema(t *testing.T) {
	db := newBackfillTestDB(t)

	// Old abilities schema: no weight column.
	require.NoError(t, db.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, enabled INTEGER, priority INTEGER, suspend_until DATETIME, created_at INTEGER, updated_at INTEGER, PRIMARY KEY (`group`, model, channel_id))").Error)
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES ('default','m1',1,1,0)").Error)
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES ('default','m1',2,1,0)").Error)
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES ('default','m1',3,1,0)").Error)
	// Orphan ability: no matching channel row.
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES ('default','m1',99,1,0)").Error)

	// channels: weight 7, weight 3, and NULL weight.
	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (1, 7, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (2, 3, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (3, NULL, 1)").Error)

	// Upgrade: AutoMigrate adds the weight column, then the backfill runs.
	require.NoError(t, db.AutoMigrate(&Ability{}))

	// Historical rows must be NULL immediately after the schema migration; a 0
	// default here would defeat the backfill.
	before := abilityWeights(t, db)
	for id, w := range before {
		require.Falsef(t, w.Valid, "channel %d ability weight should be NULL before backfill, got %d", id, w.Int64)
	}

	require.NoError(t, MigrateAbilityWeightBackfill())

	after := abilityWeights(t, db)
	require.Equal(t, int64(7), after[1].Int64, "weight 7 must be backfilled from channel 1")
	require.Equal(t, int64(3), after[2].Int64, "weight 3 must be backfilled from channel 2")
	require.Equal(t, int64(0), after[3].Int64, "NULL channel weight coalesces to 0")
	require.Equal(t, int64(0), after[99].Int64, "orphan ability (no channel) coalesces to 0")
	for id, w := range after {
		require.Truef(t, w.Valid, "channel %d ability weight must be non-NULL after backfill", id)
	}
}

// TestMigrateAbilityWeightBackfill_HealsStaleZeroFromDefaultColumn is the
// regression guard for the "already-default:0" upgrade shape: an environment that
// booted an intermediate build which declared abilities.weight WITH a database
// default of 0. Such rows physically store 0 (never NULL), so a NULL-only backfill
// silently skips them and the DB routing path disagrees with the cache path
// forever. The reconciling backfill must heal them to the parent channel's weight.
func TestMigrateAbilityWeightBackfill_HealsStaleZeroFromDefaultColumn(t *testing.T) {
	db := newBackfillTestDB(t)

	// Intermediate schema: weight column present WITH a DB default of 0, so inserted
	// rows that omit weight store 0 (not NULL) — exactly what default:0 produced.
	require.NoError(t, db.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, enabled INTEGER, priority INTEGER, weight INTEGER DEFAULT 0, suspend_until DATETIME, created_at INTEGER, updated_at INTEGER, PRIMARY KEY (`group`, model, channel_id))").Error)
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES ('default','m1',1,1,0)").Error)
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES ('default','m1',2,1,0)").Error)

	// Confirm the precondition: rows are 0, NOT NULL (so a NULL-only backfill is a no-op).
	before := abilityWeights(t, db)
	require.True(t, before[1].Valid)
	require.Equal(t, int64(0), before[1].Int64)

	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (1, 7, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (2, 3, 1)").Error)

	// AutoMigrate on the working-tree struct must NOT invent a NULL; it leaves the
	// stale 0s in place, so the reconcile is what actually fixes them.
	require.NoError(t, db.AutoMigrate(&Ability{}))
	require.NoError(t, MigrateAbilityWeightBackfill())

	after := abilityWeights(t, db)
	require.Equal(t, int64(7), after[1].Int64, "stale 0 must be reconciled to channel weight 7")
	require.Equal(t, int64(3), after[2].Int64, "stale 0 must be reconciled to channel weight 3")

	// Second run is a no-op (already converged).
	require.NoError(t, MigrateAbilityWeightBackfill())
	after2 := abilityWeights(t, db)
	require.Equal(t, int64(7), after2[1].Int64)
	require.Equal(t, int64(3), after2[2].Int64)
}

// TestMigrateAbilityWeightBackfill_MixedNullAndStale exercises a table holding a
// mix of NULL, stale-nonzero, correct, and channel-is-0 rows in one pass.
func TestMigrateAbilityWeightBackfill_MixedNullAndStale(t *testing.T) {
	db := newBackfillTestDB(t)

	require.NoError(t, db.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, enabled INTEGER, priority INTEGER, weight INTEGER, suspend_until DATETIME, created_at INTEGER, updated_at INTEGER, PRIMARY KEY (`group`, model, channel_id))").Error)
	// ch1: NULL ability, channel weight 7  -> heal to 7
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority, weight) VALUES ('default','m1',1,1,0,NULL)").Error)
	// ch2: stale 99 ability, channel weight 3 -> heal to 3
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority, weight) VALUES ('default','m1',2,1,0,99)").Error)
	// ch3: already-correct 5, channel weight 5 -> untouched
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority, weight) VALUES ('default','m1',3,1,0,5)").Error)
	// ch4: stale 8 ability, channel weight NULL -> heal to 0
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority, weight) VALUES ('default','m1',4,1,0,8)").Error)

	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (1, 7, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (2, 3, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (3, 5, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (4, NULL, 1)").Error)

	require.NoError(t, MigrateAbilityWeightBackfill())

	after := abilityWeights(t, db)
	require.Equal(t, int64(7), after[1].Int64)
	require.Equal(t, int64(3), after[2].Int64)
	require.Equal(t, int64(5), after[3].Int64)
	require.Equal(t, int64(0), after[4].Int64, "channel with NULL weight coalesces to 0")
}

// TestMigrateAbilityWeightBackfill_Idempotent verifies re-running the backfill does
// not change already-populated weights (no NULL rows remain to touch).
func TestMigrateAbilityWeightBackfill_Idempotent(t *testing.T) {
	db := newBackfillTestDB(t)

	require.NoError(t, db.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, enabled INTEGER, priority INTEGER, suspend_until DATETIME, created_at INTEGER, updated_at INTEGER, PRIMARY KEY (`group`, model, channel_id))").Error)
	require.NoError(t, db.Exec("INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES ('default','m1',1,1,0)").Error)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, db.Exec("INSERT INTO channels (id, weight, status) VALUES (1, 42, 1)").Error)
	require.NoError(t, db.AutoMigrate(&Ability{}))

	require.NoError(t, MigrateAbilityWeightBackfill())
	require.Equal(t, int64(42), abilityWeights(t, db)[1].Int64)

	// Re-running while already in sync is a true no-op: no rows to touch.
	require.NoError(t, MigrateAbilityWeightBackfill())
	require.Equal(t, int64(42), abilityWeights(t, db)[1].Int64)

	// If the channel weight drifts out of sync with the projection (e.g. a stale
	// row left by an interrupted upgrade), the reconcile heals abilities to match
	// the channel — channels.weight is the single source of truth.
	require.NoError(t, db.Exec("UPDATE channels SET weight = 100 WHERE id = 1").Error)
	require.NoError(t, MigrateAbilityWeightBackfill())
	require.Equal(t, int64(100), abilityWeights(t, db)[1].Int64,
		"reconcile must sync a drifted projection to the channel's weight")
}

// TestMigrateAbilityWeightBackfill_NoAbilitiesTable is a safety check for a DB where
// the abilities table does not yet exist.
func TestMigrateAbilityWeightBackfill_NoAbilitiesTable(t *testing.T) {
	newBackfillTestDB(t)
	require.NoError(t, MigrateAbilityWeightBackfill())
}

// TestAddAbilities_WritesExplicitWeight verifies that on a fresh install new
// abilities receive a concrete (non-NULL) weight mirroring the parent channel, so
// they are never in the backfill's NULL set.
func TestAddAbilities_WritesExplicitWeight(t *testing.T) {
	db := newBackfillTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	withWeight := &Channel{Id: 1, Name: "w", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Weight: uintPtr(5)}
	require.NoError(t, db.Create(withWeight).Error)
	require.NoError(t, withWeight.AddAbilities())

	noWeight := &Channel{Id: 2, Name: "nw", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Weight: nil}
	require.NoError(t, db.Create(noWeight).Error)
	require.NoError(t, noWeight.AddAbilities())

	weights := abilityWeights(t, db)
	require.True(t, weights[1].Valid)
	require.Equal(t, int64(5), weights[1].Int64)
	require.True(t, weights[2].Valid, "channel with nil weight must still persist an explicit 0, not NULL")
	require.Equal(t, int64(0), weights[2].Int64)
}

// TestChannelUpdate_ResyncsAbilityWeight verifies the projection contract: when a
// channel's weight changes via Update(), the abilities.weight projection is rebuilt
// to match — including the subtle case of updating the weight down to 0 (a non-nil
// *uint pointing at 0 must persist as 0, not be skipped as a zero value).
func TestChannelUpdate_ResyncsAbilityWeight(t *testing.T) {
	db := newBackfillTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	ch := &Channel{Name: "resync", Type: 1, Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Weight: uintPtr(5)}
	require.NoError(t, ch.Insert())
	require.Equal(t, int64(5), abilityWeights(t, db)[ch.Id].Int64, "insert should project weight 5")

	// Update weight down to 0.
	ch.Weight = uintPtr(0)
	require.NoError(t, ch.Update())
	require.Equal(t, int64(0), abilityWeights(t, db)[ch.Id].Int64,
		"weight-only update to 0 must re-sync the ability projection to 0, not leave it stale at 5")

	// Update weight up to 9.
	ch.Weight = uintPtr(9)
	require.NoError(t, ch.Update())
	require.Equal(t, int64(9), abilityWeights(t, db)[ch.Id].Int64,
		"weight-only update to 9 must re-sync the ability projection")
}
