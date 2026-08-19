package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestMigrateCustomChannelsToOpenAICompatible verifies that legacy custom channels are upgraded
// to the OpenAI-compatible type and that the migration remains idempotent across subsequent runs.
func TestMigrateCustomChannelsToOpenAICompatible(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	require.NoError(t, db.AutoMigrate(&Channel{}))

	legacy := Channel{Id: 1, Name: "legacy-custom", Type: channeltype.Custom}
	modern := Channel{Id: 2, Name: "already-compatible", Type: channeltype.OpenAICompatible}
	untouched := Channel{Id: 3, Name: "openai", Type: channeltype.OpenAI}

	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Create(&modern).Error)
	require.NoError(t, db.Create(&untouched).Error)

	require.NoError(t, MigrateCustomChannelsToOpenAICompatible())

	var migrated Channel
	require.NoError(t, db.First(&migrated, legacy.Id).Error)
	require.Equal(t, channeltype.OpenAICompatible, migrated.Type)

	// Ensure other channel types remain unchanged
	var stillCompatible, stillOpenAI Channel
	require.NoError(t, db.First(&stillCompatible, modern.Id).Error)
	require.Equal(t, channeltype.OpenAICompatible, stillCompatible.Type)
	require.NoError(t, db.First(&stillOpenAI, untouched.Id).Error)
	require.Equal(t, channeltype.OpenAI, stillOpenAI.Type)

	// Second run should be a no-op
	require.NoError(t, MigrateCustomChannelsToOpenAICompatible())
	require.NoError(t, db.First(&migrated, legacy.Id).Error)
	require.Equal(t, channeltype.OpenAICompatible, migrated.Type)
}
