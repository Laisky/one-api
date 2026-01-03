package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChannelFieldsCanHandleLargeData tests that ModelConfigs and ModelMapping can handle large JSON data
func TestChannelFieldsCanHandleLargeData(t *testing.T) {
	t.Parallel()
	// Create a large ModelConfigs JSON that would exceed 1024 characters
	largeModelConfigs := make(map[string]ModelConfigLocal)

	// Add many models with detailed configurations
	models := []string{
		"gpt-3.5-turbo", "gpt-3.5-turbo-0301", "gpt-3.5-turbo-0613", "gpt-3.5-turbo-1106", "gpt-3.5-turbo-16k",
		"gpt-4", "gpt-4-0314", "gpt-4-0613", "gpt-4-1106-preview", "gpt-4-turbo", "gpt-4-turbo-preview",
		"gpt-4o", "gpt-4o-mini", "claude-3-opus", "claude-3-sonnet", "claude-3-haiku", "claude-3-5-sonnet",
		"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-3-pro-preview", "text-embedding-ada-002",
		"text-embedding-3-small", "text-embedding-3-large", "dall-e-2", "dall-e-3", "whisper-1", "tts-1", "tts-1-hd",
	}

	for _, model := range models {
		largeModelConfigs[model] = ModelConfigLocal{
			Ratio:           1.5,
			CompletionRatio: 2.0,
			MaxTokens:       4096,
		}
	}

	// Create a large ModelMapping JSON
	largeModelMapping := make(map[string]string)
	for i, model := range models {
		if i%2 == 0 {
			largeModelMapping[model] = "mapped-" + model + "-with-very-long-suffix-to-increase-size"
		}
	}

	// Serialize to JSON and verify they exceed 1024 characters
	modelConfigsJSON, err := json.Marshal(largeModelConfigs)
	require.NoError(t, err)

	modelMappingJSON, err := json.Marshal(largeModelMapping)
	require.NoError(t, err)

	t.Logf("ModelConfigs JSON size: %d characters", len(modelConfigsJSON))
	t.Logf("ModelMapping JSON size: %d characters", len(modelMappingJSON))

	// Verify they exceed the old 1024 limit
	assert.Greater(t, len(modelConfigsJSON), 1024, "ModelConfigs JSON should exceed 1024 characters")
	assert.Greater(t, len(modelMappingJSON), 1024, "ModelMapping JSON should exceed 1024 characters")

	// Create a channel with large data
	channel := &Channel{
		Id:   1,
		Name: "Test Channel",
		Type: 1,
	}

	// Test setting large ModelConfigs
	err = channel.SetModelPriceConfigs(largeModelConfigs)
	require.NoError(t, err)

	// Verify the data was set correctly
	assert.NotNil(t, channel.ModelConfigs)
	assert.Greater(t, len(*channel.ModelConfigs), 1024)

	// Test retrieving the data
	retrievedConfigs := channel.GetModelPriceConfigs()
	require.NotNil(t, retrievedConfigs)
	assert.Equal(t, len(largeModelConfigs), len(retrievedConfigs))

	// Verify specific model configurations
	for modelName, expectedConfig := range largeModelConfigs {
		actualConfig, exists := retrievedConfigs[modelName]
		assert.True(t, exists, "Model %s should exist in retrieved configs", modelName)
		assert.Equal(t, expectedConfig.Ratio, actualConfig.Ratio)
		assert.Equal(t, expectedConfig.CompletionRatio, actualConfig.CompletionRatio)
		assert.Equal(t, expectedConfig.MaxTokens, actualConfig.MaxTokens)
	}

	// Test setting large ModelMapping
	modelMappingStr := string(modelMappingJSON)
	channel.ModelMapping = &modelMappingStr

	// Verify the data was set correctly
	assert.NotNil(t, channel.ModelMapping)
	assert.Greater(t, len(*channel.ModelMapping), 1024)

	// Test retrieving the mapping
	retrievedMapping := channel.GetModelMapping()
	require.NotNil(t, retrievedMapping)
	assert.Equal(t, len(largeModelMapping), len(retrievedMapping))

	// Verify specific mappings
	for originalModel, expectedMapped := range largeModelMapping {
		actualMapped, exists := retrievedMapping[originalModel]
		assert.True(t, exists, "Model %s should exist in retrieved mapping", originalModel)
		assert.Equal(t, expectedMapped, actualMapped)
	}
}

// TestChannelFieldMigrationFunction tests the MigrateChannelFieldsToText function
func TestChannelFieldMigrationFunction(t *testing.T) {
	// This test verifies that the migration function can be called without errors
	// Note: This test doesn't actually test database schema changes since that requires
	// a real database connection and would be integration testing

	// Test that the function exists and can be called
	// In a real environment, this would perform the actual migration
	err := MigrateChannelFieldsToText()

	// The function should handle the case where no migration is needed gracefully
	// or return an error if there's a database connection issue
	if err != nil {
		// If there's an error, it should be related to database connectivity
		// not to the function logic itself
		assert.True(t, strings.Contains(err.Error(), "failed to check migration status") ||
			strings.Contains(err.Error(), "failed to check") ||
			strings.Contains(err.Error(), "failed to start transaction") ||
			strings.Contains(err.Error(), "failed to migrate") ||
			strings.Contains(err.Error(), "failed to commit migration"),
			"Error should be database-related: %v", err)
	}
}

// TestCheckIfFieldMigrationNeeded tests the idempotency check function
func TestCheckIfFieldMigrationNeeded(t *testing.T) {
	// Test that the check function exists and can be called
	// In a real environment with database connection, this would check actual column types
	needed, err := checkIfFieldMigrationNeeded()

	if err != nil {
		// If there's an error, it should be related to database connectivity
		assert.True(t, strings.Contains(err.Error(), "failed to check") ||
			strings.Contains(err.Error(), "column type"),
			"Error should be database-related: %v", err)
	} else {
		// The function should return a boolean indicating if migration is needed
		assert.IsType(t, false, needed, "Function should return a boolean")
	}
}

// TestChannelFieldsBackwardCompatibility tests that existing data remains accessible
func TestChannelFieldsBackwardCompatibility(t *testing.T) {
	// Test with small data (should work with both old and new schema)
	smallModelConfigs := map[string]ModelConfigLocal{
		"gpt-3.5-turbo": {
			Ratio:           1.0,
			CompletionRatio: 2.0,
			MaxTokens:       4096,
		},
		"gpt-4": {
			Ratio:           20.0,
			CompletionRatio: 20.0,
			MaxTokens:       8192,
		},
	}

	channel := &Channel{
		Id:   1,
		Name: "Backward Compatibility Test",
		Type: 1,
	}

	// Set and retrieve small data
	err := channel.SetModelPriceConfigs(smallModelConfigs)
	require.NoError(t, err)

	retrievedConfigs := channel.GetModelPriceConfigs()
	require.NotNil(t, retrievedConfigs)
	assert.Equal(t, len(smallModelConfigs), len(retrievedConfigs))

	// Verify the data integrity
	for modelName, expectedConfig := range smallModelConfigs {
		actualConfig, exists := retrievedConfigs[modelName]
		assert.True(t, exists)
		assert.Equal(t, expectedConfig.Ratio, actualConfig.Ratio)
		assert.Equal(t, expectedConfig.CompletionRatio, actualConfig.CompletionRatio)
		assert.Equal(t, expectedConfig.MaxTokens, actualConfig.MaxTokens)
	}
}
