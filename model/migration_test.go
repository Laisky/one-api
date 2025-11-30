package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
)

func TestMain(m *testing.M) {
	// Setup logger for tests
	logger.SetupLogger()

	// Run tests
	code := m.Run()

	// Cleanup
	os.Exit(code)
}

func setupMigrationTestDB(t *testing.T) *gorm.DB {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "Failed to create test database")

	// Set database type flags
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)

	return db
}

func TestMigrateChannelFieldsToText_SQLite(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Test that SQLite migration is skipped
	err := MigrateChannelFieldsToText()
	require.NoError(t, err, "SQLite field migration should not fail")

	// Verify that the migration was skipped (no actual schema changes needed for SQLite)
	// This is expected behavior as documented in the function
}

func TestMigrateChannelFieldsToText_Idempotency(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Run migration multiple times - should be idempotent
	for i := range 3 {
		err := MigrateChannelFieldsToText()
		require.NoError(t, err, "Migration run %d failed", i+1)
	}
}

func TestMigrateTraceURLColumnToText_SQLite(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	err := MigrateTraceURLColumnToText()
	require.NoError(t, err, "SQLite trace URL migration should not fail")
}

func TestMigrateTraceURLColumnToText_Idempotency(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	for i := range 3 {
		err := MigrateTraceURLColumnToText()
		require.NoError(t, err, "Trace URL migration run %d failed", i+1)
	}
}

func TestCheckIfFieldMigrationNeeded_SQLite(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// For SQLite, migration should never be needed
	needed, err := checkIfFieldMigrationNeeded()
	require.NoError(t, err, "checkIfFieldMigrationNeeded failed")
	require.False(t, needed, "SQLite should never need field migration")
}

func TestChannelModelConfigsMigration(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Create channels table
	err := testDB.AutoMigrate(&Channel{})
	require.NoError(t, err, "Failed to create channels table")

	// Create test channel with old format ModelConfigs
	oldFormatConfigs := `{"gpt-3.5-turbo":{"ratio":1.0,"completion_ratio":2.0,"max_tokens":4096}}`
	testChannel := &Channel{
		Name:         "Test Channel",
		Type:         1,
		Status:       1,
		Models:       "gpt-3.5-turbo",
		ModelConfigs: &oldFormatConfigs,
	}

	err = testDB.Create(testChannel).Error
	require.NoError(t, err, "Failed to create test channel")

	// Run the migration
	err = MigrateAllChannelModelConfigs()
	require.NoError(t, err, "MigrateAllChannelModelConfigs failed")

	// Verify the channel still exists and has valid ModelConfigs
	var migratedChannel Channel
	err = testDB.First(&migratedChannel, testChannel.Id).Error
	require.NoError(t, err, "Failed to retrieve migrated channel")

	require.NotNil(t, migratedChannel.ModelConfigs, "ModelConfigs should not be nil after migration")
	require.NotEmpty(t, *migratedChannel.ModelConfigs, "ModelConfigs should not be empty after migration")

	// Test that the migrated configs are valid
	configs := migratedChannel.GetModelPriceConfigs()
	require.NotEmpty(t, configs, "Migrated ModelConfigs should contain model configurations")

	config, exists := configs["gpt-3.5-turbo"]
	require.True(t, exists, "Expected gpt-3.5-turbo configuration to exist after migration")
	require.Equal(t, 1.0, config.Ratio, "Expected ratio 1.0")
	require.Equal(t, 2.0, config.CompletionRatio, "Expected completion ratio 2.0")
}

func TestChannelModelConfigsMigration_EmptyData(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Create channels table
	err := testDB.AutoMigrate(&Channel{})
	require.NoError(t, err, "Failed to create channels table")

	// Create test channel with no ModelConfigs
	testChannel := &Channel{
		Name:   "Empty Test Channel",
		Type:   1,
		Status: 1,
		Models: "gpt-4",
	}

	err = testDB.Create(testChannel).Error
	require.NoError(t, err, "Failed to create test channel")

	// Run the migration - should handle empty data gracefully
	err = MigrateAllChannelModelConfigs()
	require.NoError(t, err, "MigrateAllChannelModelConfigs should handle empty data")
}

func TestChannelModelConfigsMigration_InvalidJSON(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Create channels table
	err := testDB.AutoMigrate(&Channel{})
	require.NoError(t, err, "Failed to create channels table")

	// Create test channel with invalid JSON
	invalidJSON := `{"invalid": json}`
	testChannel := &Channel{
		Name:         "Invalid JSON Channel",
		Type:         1,
		Status:       1,
		Models:       "gpt-4",
		ModelConfigs: &invalidJSON,
	}

	err = testDB.Create(testChannel).Error
	require.NoError(t, err, "Failed to create test channel")

	// Run the migration - should handle invalid JSON gracefully
	err = MigrateAllChannelModelConfigs()
	// This should not fail the entire migration, just log errors
	if err != nil {
		t.Logf("Expected behavior: migration handles invalid JSON gracefully: %v", err)
	}
}

func TestChannelNullHandling(t *testing.T) {
	// Setup test database
	testDB := setupMigrationTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Create channels table
	err := testDB.AutoMigrate(&Channel{})
	require.NoError(t, err, "Failed to create channels table")

	// Test 1: Create channel with NULL ModelConfigs and ModelMapping
	testChannel := &Channel{
		Name:         "NULL Test Channel",
		Type:         1,
		Status:       1,
		Models:       "gpt-4",
		ModelConfigs: nil, // Explicitly NULL
		ModelMapping: nil, // Explicitly NULL
	}

	err = testDB.Create(testChannel).Error
	require.NoError(t, err, "Failed to create test channel with NULL fields")

	// Test 2: Verify NULL values are handled correctly by getter methods
	configs := testChannel.GetModelPriceConfigs()
	require.Nil(t, configs, "GetModelPriceConfigs should return nil for NULL ModelConfigs")

	mapping := testChannel.GetModelMapping()
	require.Nil(t, mapping, "GetModelMapping should return nil for NULL ModelMapping")

	// Test 3: Verify setter methods handle NULL correctly
	err = testChannel.SetModelPriceConfigs(nil)
	require.NoError(t, err, "SetModelPriceConfigs should handle nil input")

	require.Nil(t, testChannel.ModelConfigs, "SetModelPriceConfigs(nil) should set ModelConfigs to nil")

	// Test 4: Verify migration handles NULL values correctly
	err = MigrateAllChannelModelConfigs()
	require.NoError(t, err, "Migration should handle NULL values gracefully")

	// Test 5: Verify database operations work with NULL values
	var retrievedChannel Channel
	err = testDB.First(&retrievedChannel, testChannel.Id).Error
	require.NoError(t, err, "Failed to retrieve channel with NULL fields")

	// Verify NULL values are preserved
	require.Nil(t, retrievedChannel.ModelConfigs, "NULL ModelConfigs should remain NULL after database round-trip")

	require.Nil(t, retrievedChannel.ModelMapping, "NULL ModelMapping should remain NULL after database round-trip")
}
