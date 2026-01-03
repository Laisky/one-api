package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationIntegration_CompleteWorkflow tests the complete migration workflow
func TestMigrationIntegration_CompleteWorkflow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		initialModelConfigs    *string
		initialModelRatio      *string
		initialCompletionRatio *string
		expectedFinalConfigs   map[string]ModelConfigLocal
		expectError            bool
	}{
		{
			name:                   "complete migration from separate fields",
			initialModelConfigs:    nil,
			initialModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
			initialCompletionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "gpt-4": 2.0}`),
			expectedFinalConfigs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 2.0,
					MaxTokens:       0,
				},
				"gpt-4": {
					Ratio:           0.03,
					CompletionRatio: 2.0,
					MaxTokens:       0,
				},
			},
			expectError: false,
		},
		{
			name:                   "migration from old ModelConfigs format with pricing merge",
			initialModelConfigs:    stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}, "gpt-4": {"max_tokens": 128000}}`),
			initialModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
			initialCompletionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "gpt-4": 2.0}`),
			expectedFinalConfigs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 2.0,
					MaxTokens:       65536,
				},
				"gpt-4": {
					Ratio:           0.03,
					CompletionRatio: 2.0,
					MaxTokens:       128000,
				},
			},
			expectError: false,
		},
		{
			name:                   "already in new format - no migration needed",
			initialModelConfigs:    stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0, "max_tokens": 65536}}`),
			initialModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.002}`), // Should be ignored
			initialCompletionRatio: stringPtr(`{"gpt-3.5-turbo": 1.5}`),   // Should be ignored
			expectedFinalConfigs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 2.0,
					MaxTokens:       65536,
				},
			},
			expectError: false,
		},
		{
			name:                   "partial data migration",
			initialModelConfigs:    nil,
			initialModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
			initialCompletionRatio: nil,
			expectedFinalConfigs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 0,
					MaxTokens:       0,
				},
			},
			expectError: false,
		},
		{
			name:                   "mixed model coverage",
			initialModelConfigs:    stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}}`),
			initialModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
			initialCompletionRatio: stringPtr(`{"gpt-4": 2.0}`),
			expectedFinalConfigs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 0,
					MaxTokens:       65536,
				},
				"gpt-4": {
					Ratio:           0.03,
					CompletionRatio: 2.0,
					MaxTokens:       0,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:              1,
				ModelConfigs:    tt.initialModelConfigs,
				ModelRatio:      tt.initialModelRatio,
				CompletionRatio: tt.initialCompletionRatio,
			}

			// Run the complete migration workflow
			err := channel.MigrateModelConfigsToModelPrice()
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			err = channel.MigrateHistoricalPricingToModelConfigs()
			require.NoError(t, err)

			// Verify the final result
			finalConfigs := channel.GetModelPriceConfigs()
			assert.Equal(t, tt.expectedFinalConfigs, finalConfigs)

			// Verify that the data is properly serialized
			if channel.ModelConfigs != nil {
				var serializedConfigs map[string]ModelConfigLocal
				err = json.Unmarshal([]byte(*channel.ModelConfigs), &serializedConfigs)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedFinalConfigs, serializedConfigs)
			}
		})
	}
}

func TestMigrateLegacyImagePriceInConfigs(t *testing.T) {
	t.Parallel()
	input := `{"gpt-image-1":{"ratio":0.0,"image_price_usd":0.04}}`
	updated, changed, err := migrateLegacyImagePriceInConfigs(input)
	require.NoError(t, err)
	require.True(t, changed)
	var configs map[string]ModelConfigLocal
	require.NoError(t, json.Unmarshal([]byte(updated), &configs))
	cfg, ok := configs["gpt-image-1"]
	require.True(t, ok)
	require.NotNil(t, cfg.Image)
	require.InDelta(t, 0.04, cfg.Image.PricePerImageUsd, 1e-9)
	// idempotent re-run should report no further change
	updatedAgain, changedAgain, err := migrateLegacyImagePriceInConfigs(updated)
	require.NoError(t, err)
	require.False(t, changedAgain)
	require.Equal(t, updated, updatedAgain)
}

// TestMigrationIntegration_DataConsistency tests data consistency after migration
func TestMigrationIntegration_DataConsistency(t *testing.T) {
	t.Parallel()
	channel := &Channel{
		Id:              1,
		ModelConfigs:    stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}}`),
		ModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
		CompletionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "gpt-4": 2.0}`),
	}

	// Run migration
	err := channel.MigrateModelConfigsToModelPrice()
	require.NoError(t, err)

	err = channel.MigrateHistoricalPricingToModelConfigs()
	require.NoError(t, err)

	// Test that all access methods return consistent data
	configs := channel.GetModelPriceConfigs()
	require.NotNil(t, configs)

	// Test individual model access
	gpt35Config := channel.GetModelPriceConfig("gpt-3.5-turbo")
	require.NotNil(t, gpt35Config)
	assert.Equal(t, configs["gpt-3.5-turbo"], *gpt35Config)

	gpt4Config := channel.GetModelPriceConfig("gpt-4")
	require.NotNil(t, gpt4Config)
	assert.Equal(t, configs["gpt-4"], *gpt4Config)

	// Test backward compatibility methods
	modelRatios := channel.GetModelRatioFromConfigs()
	expectedModelRatios := map[string]float64{
		"gpt-3.5-turbo": 0.0015,
		"gpt-4":         0.03,
	}
	assert.Equal(t, expectedModelRatios, modelRatios)

	completionRatios := channel.GetCompletionRatioFromConfigs()
	expectedCompletionRatios := map[string]float64{
		"gpt-3.5-turbo": 2.0,
		"gpt-4":         2.0,
	}
	assert.Equal(t, expectedCompletionRatios, completionRatios)

	// Test GetModelConfig backward compatibility
	modelConfig := channel.GetModelConfig("gpt-3.5-turbo")
	require.NotNil(t, modelConfig)
	assert.Equal(t, int32(65536), modelConfig.MaxTokens)
}

// TestMigrationIntegration_Idempotency tests that migration is idempotent
func TestMigrationIntegration_Idempotency(t *testing.T) {
	t.Parallel()
	channel := &Channel{
		Id:              1,
		ModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
		CompletionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0}`),
	}

	// Run migration first time
	err := channel.MigrateHistoricalPricingToModelConfigs()
	require.NoError(t, err)

	firstResult := channel.GetModelPriceConfigs()
	require.NotNil(t, firstResult)

	// Run migration second time
	err = channel.MigrateHistoricalPricingToModelConfigs()
	require.NoError(t, err)

	secondResult := channel.GetModelPriceConfigs()
	require.NotNil(t, secondResult)

	// Results should be identical
	assert.Equal(t, firstResult, secondResult)

	// Run migration third time with different historical data (should be ignored)
	channel.ModelRatio = stringPtr(`{"gpt-3.5-turbo": 0.002}`)
	channel.CompletionRatio = stringPtr(`{"gpt-3.5-turbo": 3.0}`)

	err = channel.MigrateHistoricalPricingToModelConfigs()
	require.NoError(t, err)

	thirdResult := channel.GetModelPriceConfigs()
	require.NotNil(t, thirdResult)

	// Should still be the same as first migration (historical data ignored)
	assert.Equal(t, firstResult, thirdResult)
}

// TestMigrationIntegration_ErrorRecovery tests error recovery scenarios
func TestMigrationIntegration_ErrorRecovery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		initialModelConfigs  *string
		initialModelRatio    *string
		expectMigrationError bool
		expectDataCorruption bool
	}{
		{
			name:                 "invalid JSON in ModelConfigs",
			initialModelConfigs:  stringPtr(`{"invalid": json}`),
			expectMigrationError: true,
			expectDataCorruption: false,
		},
		{
			name:                 "invalid JSON in ModelRatio",
			initialModelRatio:    stringPtr(`{"invalid": json}`),
			expectMigrationError: false, // Should continue with valid data
			expectDataCorruption: false,
		},
		{
			name:                 "corrupted data structure",
			initialModelConfigs:  stringPtr(`["not", "an", "object"]`),
			expectMigrationError: true,
			expectDataCorruption: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:           1,
				ModelConfigs: tt.initialModelConfigs,
				ModelRatio:   tt.initialModelRatio,
			}

			originalModelConfigs := ""
			if channel.ModelConfigs != nil {
				originalModelConfigs = *channel.ModelConfigs
			}

			// Run migration
			err1 := channel.MigrateModelConfigsToModelPrice()
			err2 := channel.MigrateHistoricalPricingToModelConfigs()

			if tt.expectMigrationError {
				assert.True(t, err1 != nil || err2 != nil, "Expected at least one migration to fail")
			} else {
				assert.NoError(t, err1)
				assert.NoError(t, err2)
			}

			if !tt.expectDataCorruption {
				// Verify that either migration succeeded or data is unchanged
				if err1 != nil && err2 != nil {
					// Both failed - data should be unchanged
					if originalModelConfigs == "" {
						assert.Nil(t, channel.ModelConfigs)
					} else {
						assert.Equal(t, originalModelConfigs, *channel.ModelConfigs)
					}
				} else {
					// At least one succeeded - data should be valid
					configs := channel.GetModelPriceConfigs()
					if configs != nil {
						// If we have configs, they should be valid
						err := channel.validateModelPriceConfigs(configs)
						assert.NoError(t, err)
					}
				}
			}
		})
	}
}
