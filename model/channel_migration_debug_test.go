package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationDebug_ChannelSpecificModels tests that migration only uses channel-specific data
func TestMigrationDebug_ChannelSpecificModels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		channelType     int
		modelRatio      *string
		completionRatio *string
		expectedModels  []string
		shouldMigrate   bool
	}{
		{
			name:            "OpenAI channel with OpenAI models",
			channelType:     1, // OpenAI
			modelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
			completionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "gpt-4": 2.0}`),
			expectedModels:  []string{"gpt-3.5-turbo", "gpt-4"},
			shouldMigrate:   true,
		},
		{
			name:            "Claude channel with Claude models",
			channelType:     8, // Claude
			modelRatio:      stringPtr(`{"claude-3-opus": 0.015, "claude-3-sonnet": 0.003}`),
			completionRatio: stringPtr(`{"claude-3-opus": 5.0, "claude-3-sonnet": 5.0}`),
			expectedModels:  []string{"claude-3-opus", "claude-3-sonnet"},
			shouldMigrate:   true,
		},
		{
			name:            "Mixed models - should migrate all (user responsibility)",
			channelType:     1, // OpenAI
			modelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "claude-3-opus": 0.015}`),
			completionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "claude-3-opus": 5.0}`),
			expectedModels:  []string{"gpt-3.5-turbo", "claude-3-opus"},
			shouldMigrate:   true,
		},
		{
			name:            "Empty pricing data",
			channelType:     1, // OpenAI
			modelRatio:      nil,
			completionRatio: nil,
			expectedModels:  []string{},
			shouldMigrate:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:              1,
				Type:            tt.channelType,
				ModelRatio:      tt.modelRatio,
				CompletionRatio: tt.completionRatio,
			}

			// Run migration
			err := channel.MigrateHistoricalPricingToModelConfigs()
			require.NoError(t, err)

			if tt.shouldMigrate {
				// Verify migration occurred
				configs := channel.GetModelPriceConfigs()
				require.NotNil(t, configs)
				assert.Equal(t, len(tt.expectedModels), len(configs))

				// Verify expected models are present
				for _, expectedModel := range tt.expectedModels {
					assert.Contains(t, configs, expectedModel, "Expected model %s not found in migrated configs", expectedModel)
				}

				// Verify no unexpected models
				for modelName := range configs {
					assert.Contains(t, tt.expectedModels, modelName, "Unexpected model %s found in migrated configs", modelName)
				}
			} else {
				// Verify no migration occurred
				configs := channel.GetModelPriceConfigs()
				assert.Nil(t, configs)
			}
		})
	}
}

// TestMigrationDebug_DataIntegrity tests that migrated data maintains integrity
func TestMigrationDebug_DataIntegrity(t *testing.T) {
	t.Parallel()
	channel := &Channel{
		Id:   1,
		Type: 1, // OpenAI
		ModelRatio: stringPtr(`{
			"gpt-3.5-turbo": 0.0015,
			"gpt-4": 0.03,
			"gpt-4-turbo": 0.01
		}`),
		CompletionRatio: stringPtr(`{
			"gpt-3.5-turbo": 2.0,
			"gpt-4": 2.0
		}`),
	}

	// Run migration
	err := channel.MigrateHistoricalPricingToModelConfigs()
	require.NoError(t, err)

	// Verify migrated data
	configs := channel.GetModelPriceConfigs()
	require.NotNil(t, configs)
	assert.Equal(t, 3, len(configs))

	// Check gpt-3.5-turbo (has both ratios)
	gpt35Config, exists := configs["gpt-3.5-turbo"]
	assert.True(t, exists)
	assert.Equal(t, float64(0.0015), gpt35Config.Ratio)
	assert.Equal(t, float64(2.0), gpt35Config.CompletionRatio)
	assert.Equal(t, int32(0), gpt35Config.MaxTokens)

	// Check gpt-4 (has both ratios)
	gpt4Config, exists := configs["gpt-4"]
	assert.True(t, exists)
	assert.Equal(t, float64(0.03), gpt4Config.Ratio)
	assert.Equal(t, float64(2.0), gpt4Config.CompletionRatio)
	assert.Equal(t, int32(0), gpt4Config.MaxTokens)

	// Check gpt-4-turbo (has only model ratio)
	gpt4TurboConfig, exists := configs["gpt-4-turbo"]
	assert.True(t, exists)
	assert.Equal(t, float64(0.01), gpt4TurboConfig.Ratio)
	assert.Equal(t, float64(0), gpt4TurboConfig.CompletionRatio)
	assert.Equal(t, int32(0), gpt4TurboConfig.MaxTokens)

	// Verify JSON serialization
	require.NotNil(t, channel.ModelConfigs)
	var serializedConfigs map[string]ModelConfigLocal
	err = json.Unmarshal([]byte(*channel.ModelConfigs), &serializedConfigs)
	require.NoError(t, err)
	assert.Equal(t, configs, serializedConfigs)
}

// TestMigrationDebug_CrossContamination tests that channels don't contaminate each other
func TestMigrationDebug_CrossContamination(t *testing.T) {
	// Create two different channels with different model sets
	openaiChannel := &Channel{
		Id:              1,
		Type:            1, // OpenAI
		ModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
		CompletionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "gpt-4": 2.0}`),
	}

	claudeChannel := &Channel{
		Id:              2,
		Type:            8, // Claude
		ModelRatio:      stringPtr(`{"claude-3-opus": 0.015, "claude-3-sonnet": 0.003}`),
		CompletionRatio: stringPtr(`{"claude-3-opus": 5.0, "claude-3-sonnet": 5.0}`),
	}

	// Migrate both channels
	err := openaiChannel.MigrateHistoricalPricingToModelConfigs()
	require.NoError(t, err)

	err = claudeChannel.MigrateHistoricalPricingToModelConfigs()
	require.NoError(t, err)

	// Verify OpenAI channel only has OpenAI models
	openaiConfigs := openaiChannel.GetModelPriceConfigs()
	require.NotNil(t, openaiConfigs)
	assert.Equal(t, 2, len(openaiConfigs))
	assert.Contains(t, openaiConfigs, "gpt-3.5-turbo")
	assert.Contains(t, openaiConfigs, "gpt-4")
	assert.NotContains(t, openaiConfigs, "claude-3-opus")
	assert.NotContains(t, openaiConfigs, "claude-3-sonnet")

	// Verify Claude channel only has Claude models
	claudeConfigs := claudeChannel.GetModelPriceConfigs()
	require.NotNil(t, claudeConfigs)
	assert.Equal(t, 2, len(claudeConfigs))
	assert.Contains(t, claudeConfigs, "claude-3-opus")
	assert.Contains(t, claudeConfigs, "claude-3-sonnet")
	assert.NotContains(t, claudeConfigs, "gpt-3.5-turbo")
	assert.NotContains(t, claudeConfigs, "gpt-4")
}

// TestMigrationDebug_InvalidData tests handling of invalid data
func TestMigrationDebug_InvalidData(t *testing.T) {
	tests := []struct {
		name            string
		modelRatio      *string
		completionRatio *string
		expectError     bool
		expectMigration bool
	}{
		{
			name:            "Invalid JSON in ModelRatio",
			modelRatio:      stringPtr(`{"invalid": json}`),
			completionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0}`),
			expectError:     false, // Should continue with valid data
			expectMigration: true,  // Should migrate completion_ratio
		},
		{
			name:            "Invalid JSON in CompletionRatio",
			modelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
			completionRatio: stringPtr(`{"invalid": json}`),
			expectError:     false, // Should continue with valid data
			expectMigration: true,  // Should migrate model_ratio
		},
		{
			name:            "Both invalid",
			modelRatio:      stringPtr(`{"invalid": json}`),
			completionRatio: stringPtr(`{"also": invalid}`),
			expectError:     false, // Should not error but also not migrate
			expectMigration: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{
				Id:              1,
				Type:            1,
				ModelRatio:      tt.modelRatio,
				CompletionRatio: tt.completionRatio,
			}

			err := channel.MigrateHistoricalPricingToModelConfigs()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			configs := channel.GetModelPriceConfigs()
			if tt.expectMigration {
				assert.NotNil(t, configs)
				assert.Greater(t, len(configs), 0)
			} else {
				assert.Nil(t, configs)
			}
		})
	}
}
