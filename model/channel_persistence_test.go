package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useChannelPersistenceTestDB installs an isolated database for a test and restores
// the package-global database after cleanup. It returns the installed database.
func useChannelPersistenceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTestDB(t)
	originalDB := DB
	DB = db
	t.Cleanup(func() {
		DB = originalDB
	})
	return db
}

// rejectAbilityInserts installs a SQLite trigger that makes ability creation fail.
// The trigger lets transaction tests verify rollback after channel writes succeed.
func rejectAbilityInserts(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`
		CREATE TRIGGER reject_ability_insert
		BEFORE INSERT ON abilities
		BEGIN
			SELECT RAISE(FAIL, 'forced ability insert failure');
		END
	`).Error
	require.NoError(t, err)
}

// TestBatchInsertChannelsRejectsMalformedHiddenModels verifies that batch creation
// applies the same hidden-model validation as single-channel creation.
func TestBatchInsertChannelsRejectsMalformedHiddenModels(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	malformed := `{"not":"an array"}`
	channels := []Channel{{
		Name:         "malformed-hidden",
		Status:       ChannelStatusEnabled,
		Models:       "Foo",
		Group:        "default",
		HiddenModels: &malformed,
	}}

	err := BatchInsertChannels(channels)
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&Channel{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestBatchInsertChannelsRollsBackChannelOnAbilityFailure verifies that a failed
// ability write cannot leave any channel from the batch persisted.
func TestBatchInsertChannelsRollsBackChannelOnAbilityFailure(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	rejectAbilityInserts(t, db)
	channels := []Channel{{
		Name:   "batch-rollback",
		Status: ChannelStatusEnabled,
		Models: "Foo",
		Group:  "default",
	}}

	err := BatchInsertChannels(channels)
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&Channel{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestChannelInsertRollsBackChannelOnAbilityFailure verifies that single-channel
// creation atomically persists the channel and its generated abilities.
func TestChannelInsertRollsBackChannelOnAbilityFailure(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	rejectAbilityInserts(t, db)
	channel := &Channel{
		Name:   "insert-rollback",
		Status: ChannelStatusEnabled,
		Models: "Foo",
		Group:  "default",
	}

	err := channel.Insert()
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&Channel{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestChannelUpdateRollsBackChannelAndAbilities verifies that channel fields and
// the prior ability set survive when rebuilding the replacement abilities fails.
func TestChannelUpdateRollsBackChannelAndAbilities(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channel := &Channel{
		Name:   "before-update",
		Status: ChannelStatusEnabled,
		Models: "Foo",
		Group:  "default",
	}
	require.NoError(t, channel.Insert())
	rejectAbilityInserts(t, db)

	channel.Name = "after-update"
	channel.Models = "Bar"
	err := channel.Update()
	require.Error(t, err)

	var stored Channel
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	require.Equal(t, "before-update", stored.Name)
	require.Equal(t, "Foo", stored.Models)

	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, "Foo", abilities[0].Model)
}
