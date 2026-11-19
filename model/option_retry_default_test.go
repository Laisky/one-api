package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// TestInitOptionMapPreservesExplicitZeroRetries verifies the saved zero wins
// over the default when startup reloads the actual options table.
func TestInitOptionMapPreservesExplicitZeroRetries(t *testing.T) {
	withInitializedOptionMap(t)
	oldDB, oldRetries := DB, config.RetryTimes
	t.Cleanup(func() {
		DB, config.RetryTimes = oldDB, oldRetries
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db

	config.RetryTimes = config.DefaultRetryTimes
	InitOptionMap()
	require.Equal(t, 3, config.RetryTimes)
	require.Equal(t, "3", config.OptionMap["RetryTimes"])

	require.NoError(t, db.Create(&Option{Key: "RetryTimes", Value: "0"}).Error)
	InitOptionMap()
	require.Zero(t, config.RetryTimes)
	require.Equal(t, "0", config.OptionMap["RetryTimes"])
}
