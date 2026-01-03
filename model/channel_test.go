package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelSupportsModel(t *testing.T) {
	t.Parallel()
	priority := int64(10)
	channel := &Channel{
		Id:       1,
		Name:     "test",
		Models:   "gpt-4,gpt-4o-mini",
		Group:    "default",
		Priority: &priority,
	}

	t.Run("listed model", func(t *testing.T) {
		t.Parallel()
		assert.True(t, channel.SupportsModel("gpt-4"))
	})

	t.Run("case insensitive match", func(t *testing.T) {
		t.Parallel()
		assert.True(t, channel.SupportsModel("GPT-4O-MINI"))
	})

	t.Run("unlisted model", func(t *testing.T) {
		t.Parallel()
		assert.False(t, channel.SupportsModel("gpt-5"))
	})

	t.Run("empty restriction", func(t *testing.T) {
		t.Parallel()
		openChannel := &Channel{Models: ""}
		assert.True(t, openChannel.SupportsModel("anything"))
	})

	t.Run("empty model name", func(t *testing.T) {
		t.Parallel()
		assert.True(t, channel.SupportsModel(""))
	})
}

func TestChannel_MigrateModelConfigsToModelPrice(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		modelConfigs    *string
		modelRatio      *string
		completionRatio *string
		expectedResult  map[string]ModelConfigLocal
		expectError     bool
	}{
		{
			name:           "empty ModelConfigs",
			modelConfigs:   nil,
			expectedResult: nil,
			expectError:    false,
		},
		{
			name:           "empty string ModelConfigs",
			modelConfigs:   stringPtr(""),
			expectedResult: nil,
			expectError:    false,
		},
		{
			name:           "empty JSON ModelConfigs",
			modelConfigs:   stringPtr("{}"),
			expectedResult: nil,
			expectError:    false,
		},
		{
			name:           "already in new format",
			modelConfigs:   stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0, "max_tokens": 65536}}`),
			expectedResult: nil,
			expectError:    false,
		},
		{
			name:            "old format with MaxTokens only",
			modelConfigs:    stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}, "gpt-4": {"max_tokens": 128000}}`),
			modelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
			completionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "gpt-4": 2.0}`),
			expectedResult: map[string]ModelConfigLocal{
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
			name:         "old format with partial pricing data",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}}`),
			modelRatio:   stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
			expectedResult: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 0,
					MaxTokens:       65536,
				},
			},
			expectError: false,
		},
		{
			name:         "invalid JSON",
			modelConfigs: stringPtr(`{"invalid": json}`),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:              1,
				ModelConfigs:    tt.modelConfigs,
				ModelRatio:      tt.modelRatio,
				CompletionRatio: tt.completionRatio,
			}

			err := channel.MigrateModelConfigsToModelPrice()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.expectedResult == nil {
				return
			}

			// Verify the migrated data
			require.NotNil(t, channel.ModelConfigs)
			var result map[string]ModelConfigLocal
			err = json.Unmarshal([]byte(*channel.ModelConfigs), &result)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestChannel_MigrateHistoricalPricingToModelConfigs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		modelConfigs    *string
		modelRatio      *string
		completionRatio *string
		expectedResult  map[string]ModelConfigLocal
		expectError     bool
	}{
		{
			name:           "skip if ModelConfigs already has data",
			modelConfigs:   stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015}}`),
			modelRatio:     stringPtr(`{"gpt-3.5-turbo": 0.002}`),
			expectedResult: nil,
			expectError:    false,
		},
		{
			name:            "migrate from ModelRatio and CompletionRatio",
			modelConfigs:    nil,
			modelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015, "gpt-4": 0.03}`),
			completionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0, "gpt-4": 2.0}`),
			expectedResult: map[string]ModelConfigLocal{
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
			name:         "migrate from ModelRatio only",
			modelConfigs: stringPtr(""),
			modelRatio:   stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
			expectedResult: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 0,
					MaxTokens:       0,
				},
			},
			expectError: false,
		},
		{
			name:            "migrate from CompletionRatio only",
			modelConfigs:    stringPtr("{}"),
			completionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0}`),
			expectedResult: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0,
					CompletionRatio: 2.0,
					MaxTokens:       0,
				},
			},
			expectError: false,
		},
		{
			name:            "no historical data to migrate",
			modelConfigs:    nil,
			modelRatio:      nil,
			completionRatio: nil,
			expectedResult:  nil,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:              1,
				ModelConfigs:    tt.modelConfigs,
				ModelRatio:      tt.modelRatio,
				CompletionRatio: tt.completionRatio,
			}

			err := channel.MigrateHistoricalPricingToModelConfigs()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.expectedResult == nil {
				return
			}

			// Verify the migrated data
			require.NotNil(t, channel.ModelConfigs)
			var result map[string]ModelConfigLocal
			err = json.Unmarshal([]byte(*channel.ModelConfigs), &result)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestChannel_GetModelPriceConfigs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		modelConfigs *string
		expected     map[string]ModelConfigLocal
	}{
		{
			name:         "nil ModelConfigs",
			modelConfigs: nil,
			expected:     nil,
		},
		{
			name:         "empty ModelConfigs",
			modelConfigs: stringPtr(""),
			expected:     nil,
		},
		{
			name:         "empty JSON ModelConfigs",
			modelConfigs: stringPtr("{}"),
			expected:     nil,
		},
		{
			name:         "valid ModelConfigs",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0, "max_tokens": 65536}}`),
			expected: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 2.0,
					MaxTokens:       65536,
				},
			},
		},
		{
			name:         "invalid JSON",
			modelConfigs: stringPtr(`{"invalid": json}`),
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:           1,
				ModelConfigs: tt.modelConfigs,
			}

			result := channel.GetModelPriceConfigs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChannel_GetModelPriceConfig(t *testing.T) {
	channel := &Channel{
		Id:           1,
		ModelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0, "max_tokens": 65536}}`),
	}

	// Test existing model
	config := channel.GetModelPriceConfig("gpt-3.5-turbo")
	require.NotNil(t, config)
	assert.Equal(t, float64(0.0015), config.Ratio)
	assert.Equal(t, float64(2.0), config.CompletionRatio)
	assert.Equal(t, int32(65536), config.MaxTokens)

	// Test non-existing model
	config = channel.GetModelPriceConfig("non-existing-model")
	assert.Nil(t, config)
}

func TestChannel_GetModelRatioFromConfigs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		modelConfigs *string
		expected     map[string]float64
	}{
		{
			name:         "nil ModelConfigs",
			modelConfigs: nil,
			expected:     nil,
		},
		{
			name:         "empty ModelConfigs",
			modelConfigs: stringPtr(""),
			expected:     nil,
		},
		{
			name:         "ModelConfigs with ratios",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0}, "gpt-4": {"ratio": 0.03}}`),
			expected: map[string]float64{
				"gpt-3.5-turbo": 0.0015,
				"gpt-4":         0.03,
			},
		},
		{
			name:         "ModelConfigs with zero ratios",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0, "completion_ratio": 2.0}}`),
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:           1,
				ModelConfigs: tt.modelConfigs,
			}

			result := channel.GetModelRatioFromConfigs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChannel_GetCompletionRatioFromConfigs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		modelConfigs *string
		expected     map[string]float64
	}{
		{
			name:         "nil ModelConfigs",
			modelConfigs: nil,
			expected:     nil,
		},
		{
			name:         "empty ModelConfigs",
			modelConfigs: stringPtr(""),
			expected:     nil,
		},
		{
			name:         "ModelConfigs with completion ratios",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0}, "gpt-4": {"completion_ratio": 1.5}}`),
			expected: map[string]float64{
				"gpt-3.5-turbo": 2.0,
				"gpt-4":         1.5,
			},
		},
		{
			name:         "ModelConfigs with zero completion ratios",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 0}}`),
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:           1,
				ModelConfigs: tt.modelConfigs,
			}

			result := channel.GetCompletionRatioFromConfigs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChannel_GetModelConfig_UnifiedOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		modelConfigs *string
		modelName    string
		expected     *ModelConfig
	}{
		{
			name:         "get from unified format",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0, "max_tokens": 65536}}`),
			modelName:    "gpt-3.5-turbo",
			expected: &ModelConfig{
				MaxTokens: 65536,
			},
		},
		{
			name:         "get MaxTokens only from unified format",
			modelConfigs: stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}}`),
			modelName:    "gpt-3.5-turbo",
			expected: &ModelConfig{
				MaxTokens: 65536,
			},
		},
		{
			name:         "model not found",
			modelConfigs: stringPtr(`{"gpt-4": {"ratio": 0.03, "max_tokens": 128000}}`),
			modelName:    "gpt-3.5-turbo",
			expected:     nil,
		},
		{
			name:         "nil ModelConfigs",
			modelConfigs: nil,
			modelName:    "gpt-3.5-turbo",
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:           1,
				ModelConfigs: tt.modelConfigs,
			}

			result := channel.GetModelConfig(tt.modelName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChannel_SetModelPriceConfigs(t *testing.T) {
	t.Parallel()
	channel := &Channel{Id: 1}

	// Test setting valid configs
	configs := map[string]ModelConfigLocal{
		" gpt-3.5-turbo ": {
			Ratio:           0.0015,
			CompletionRatio: 2.0,
			MaxTokens:       65536,
		},
	}

	expected := map[string]ModelConfigLocal{
		"gpt-3.5-turbo": {
			Ratio:           0.0015,
			CompletionRatio: 2.0,
			MaxTokens:       65536,
		},
	}

	err := channel.SetModelPriceConfigs(configs)
	assert.NoError(t, err)
	assert.NotNil(t, channel.ModelConfigs)

	// Verify the data was set correctly
	var result map[string]ModelConfigLocal
	err = json.Unmarshal([]byte(*channel.ModelConfigs), &result)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)

	// Test setting nil configs
	err = channel.SetModelPriceConfigs(nil)
	assert.NoError(t, err)
	assert.Nil(t, channel.ModelConfigs)

	// Test setting empty configs
	err = channel.SetModelPriceConfigs(map[string]ModelConfigLocal{})
	assert.NoError(t, err)
	assert.Nil(t, channel.ModelConfigs)
}

func TestChannel_MigrateModelConfigsToModelPrice_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		modelConfigs  *string
		modelRatio    *string
		expectError   bool
		errorContains string
	}{
		{
			name:          "malformed JSON - missing quotes",
			modelConfigs:  stringPtr(`{gpt-3.5-turbo: {max_tokens: 65536}}`),
			expectError:   true,
			errorContains: "invalid JSON",
		},
		{
			name:          "malformed JSON - trailing comma",
			modelConfigs:  stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536,}}`),
			expectError:   true,
			errorContains: "invalid JSON",
		},
		{
			name:          "empty model name",
			modelConfigs:  stringPtr(`{"": {"max_tokens": 65536}}`),
			expectError:   true,
			errorContains: "empty model name",
		},
		{
			name:          "negative MaxTokens",
			modelConfigs:  stringPtr(`{"gpt-3.5-turbo": {"max_tokens": -1000}}`),
			expectError:   true,
			errorContains: "negative MaxTokens",
		},
		{
			name:          "negative ratio in historical data",
			modelConfigs:  stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}}`),
			modelRatio:    stringPtr(`{"gpt-3.5-turbo": -0.001}`),
			expectError:   true,
			errorContains: "negative ratio",
		},
		{
			name:          "array instead of object",
			modelConfigs:  stringPtr(`["gpt-3.5-turbo"]`),
			expectError:   true,
			errorContains: "cannot be parsed",
		},
		{
			name:          "null value",
			modelConfigs:  stringPtr(`null`),
			expectError:   true,
			errorContains: "cannot be parsed",
		},
		{
			name:          "string value",
			modelConfigs:  stringPtr(`"invalid"`),
			expectError:   true,
			errorContains: "cannot be parsed",
		},
		{
			name:          "model config with no meaningful data",
			modelConfigs:  stringPtr(`{"gpt-3.5-turbo": {}}`),
			expectError:   true,
			errorContains: "no meaningful configuration data",
		},
		{
			name:          "model config with zero values only",
			modelConfigs:  stringPtr(`{"gpt-3.5-turbo": {"ratio": 0, "completion_ratio": 0, "max_tokens": 0}}`),
			expectError:   true,
			errorContains: "no meaningful configuration data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:           1,
				ModelConfigs: tt.modelConfigs,
				ModelRatio:   tt.modelRatio,
			}

			err := channel.MigrateModelConfigsToModelPrice()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChannel_MigrateHistoricalPricingToModelConfigs_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		modelRatio      *string
		completionRatio *string
		expectError     bool
		errorContains   string
	}{
		{
			name:        "invalid ModelRatio JSON",
			modelRatio:  stringPtr(`{"invalid": json}`),
			expectError: false, // Should continue with valid data
		},
		{
			name:            "invalid CompletionRatio JSON",
			completionRatio: stringPtr(`{"invalid": json}`),
			expectError:     false, // Should continue with valid data
		},
		{
			name:        "empty model name in ModelRatio",
			modelRatio:  stringPtr(`{"": 0.001}`),
			expectError: false, // Should continue with valid data
		},
		{
			name:        "negative ratio in ModelRatio",
			modelRatio:  stringPtr(`{"gpt-3.5-turbo": -0.001}`),
			expectError: false, // Should continue with valid data
		},
		{
			name:            "negative ratio in CompletionRatio",
			completionRatio: stringPtr(`{"gpt-3.5-turbo": -2.0}`),
			expectError:     false, // Should continue with valid data
		},
		{
			name:        "mixed valid and invalid data",
			modelRatio:  stringPtr(`{"gpt-3.5-turbo": 0.001, "": 0.002, "gpt-4": -0.003}`),
			expectError: false, // Should continue with valid data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{
				Id:              1,
				ModelRatio:      tt.modelRatio,
				CompletionRatio: tt.completionRatio,
			}

			err := channel.MigrateHistoricalPricingToModelConfigs()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChannel_validateModelPriceConfigs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		configs       map[string]ModelConfigLocal
		expectError   bool
		errorContains string
	}{
		{
			name:        "nil configs",
			configs:     nil,
			expectError: false,
		},
		{
			name:        "empty configs",
			configs:     map[string]ModelConfigLocal{},
			expectError: false,
		},
		{
			name: "valid configs",
			configs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0.0015,
					CompletionRatio: 2.0,
					MaxTokens:       65536,
				},
			},
			expectError: false,
		},
		{
			name: "empty model name",
			configs: map[string]ModelConfigLocal{
				"": {
					Ratio: 0.0015,
				},
			},
			expectError:   true,
			errorContains: "empty model name",
		},
		{
			name: "negative ratio",
			configs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio: -0.0015,
				},
			},
			expectError:   true,
			errorContains: "negative ratio",
		},
		{
			name: "negative completion ratio",
			configs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					CompletionRatio: -2.0,
				},
			},
			expectError:   true,
			errorContains: "negative completion ratio",
		},
		{
			name: "negative MaxTokens",
			configs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					MaxTokens: -1000,
				},
			},
			expectError:   true,
			errorContains: "negative MaxTokens",
		},
		{
			name: "no meaningful data",
			configs: map[string]ModelConfigLocal{
				"gpt-3.5-turbo": {
					Ratio:           0,
					CompletionRatio: 0,
					MaxTokens:       0,
				},
			},
			expectError:   true,
			errorContains: "no meaningful configuration data",
		},
		{
			name: "video pricing only",
			configs: map[string]ModelConfigLocal{
				"sora-2": {
					Video: &VideoPricingLocal{PerSecondUsd: 0.1},
				},
			},
			expectError: false,
		},
		{
			name: "video negative price",
			configs: map[string]ModelConfigLocal{
				"sora-2": {
					Video: &VideoPricingLocal{PerSecondUsd: -0.1},
				},
			},
			expectError:   true,
			errorContains: "video per_second_usd cannot be negative",
		},
		{
			name: "video negative multiplier",
			configs: map[string]ModelConfigLocal{
				"sora-2": {
					Video: &VideoPricingLocal{
						PerSecondUsd: 0.1,
						ResolutionMultipliers: map[string]float64{
							"1280x720": -1,
						},
					},
				},
			},
			expectError:   true,
			errorContains: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channel := &Channel{Id: 1}
			err := channel.validateModelPriceConfigs(tt.configs)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
