package model

import (
	"encoding/json"
	"slices"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/channeltype"
)

// DebugChannelModelConfigs prints detailed information about a channel's model configuration
func DebugChannelModelConfigs(channelId int) error {
	var channel Channel
	err := DB.Where("id = ?", channelId).First(&channel).Error
	if err != nil {
		return errors.Wrapf(err, "failed to find channel %d", channelId)
	}

	logger.Logger.Info("=== DEBUG CHANNEL ===",
		zap.Int("channel_id", channelId),
		zap.String("name", channel.Name),
		zap.Int("type", channel.Type),
		zap.Int("status", channel.Status))

	// Check ModelConfigs
	if channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}" {
		logger.Logger.Info("ModelConfigs (raw)", zap.String("raw", *channel.ModelConfigs))

		// Try to parse as new format
		var newFormatConfigs map[string]ModelConfigLocal
		if err := json.Unmarshal([]byte(*channel.ModelConfigs), &newFormatConfigs); err == nil {
			logger.Logger.Info("ModelConfigs (new format)", zap.Int("model_count", len(newFormatConfigs)))
			for modelName, config := range newFormatConfigs {
				logger.Logger.Info("ModelConfig",
					zap.String("model", modelName),
					zap.Float64("ratio", config.Ratio),
					zap.Float64("completion_ratio", config.CompletionRatio),
					zap.Int("max_tokens", int(config.MaxTokens)))
			}
		} else {
			// Try to parse as old format
			var oldFormatConfigs map[string]ModelConfig
			if err := json.Unmarshal([]byte(*channel.ModelConfigs), &oldFormatConfigs); err == nil {
				logger.Logger.Info("ModelConfigs (old format)", zap.Int("model_count", len(oldFormatConfigs)))
				for modelName, config := range oldFormatConfigs {
					logger.Logger.Info("ModelConfig (legacy)", zap.String("model", modelName), zap.Int("max_tokens", int(config.MaxTokens)))
				}
			} else {
				logger.Logger.Error("ModelConfigs parsing failed", zap.Error(err))
			}
		}
	} else {
		logger.Logger.Info("ModelConfigs: empty or null")
	}

	// Check ModelRatio
	if channel.ModelRatio != nil && *channel.ModelRatio != "" && *channel.ModelRatio != "{}" {
		logger.Logger.Info("ModelRatio (raw)", zap.String("raw", *channel.ModelRatio))
		var modelRatios map[string]float64
		if err := json.Unmarshal([]byte(*channel.ModelRatio), &modelRatios); err == nil {
			logger.Logger.Info("ModelRatio (parsed)", zap.Int("model_count", len(modelRatios)))
			for modelName, ratio := range modelRatios {
				logger.Logger.Info("ModelRatio",
					zap.String("model", modelName),
					zap.Float64("ratio", ratio))
			}
		} else {
			logger.Logger.Error("ModelRatio parsing failed", zap.Error(err))
		}
	} else {
		logger.Logger.Info("ModelRatio: empty or null")
	}

	// Check CompletionRatio
	if channel.CompletionRatio != nil && *channel.CompletionRatio != "" && *channel.CompletionRatio != "{}" {
		logger.Logger.Info("CompletionRatio (raw)", zap.String("raw", *channel.CompletionRatio))
		var completionRatios map[string]float64
		if err := json.Unmarshal([]byte(*channel.CompletionRatio), &completionRatios); err == nil {
			logger.Logger.Info("CompletionRatio (parsed)", zap.Int("model_count", len(completionRatios)))
			for modelName, ratio := range completionRatios {
				logger.Logger.Info("CompletionRatio",
					zap.String("model", modelName),
					zap.Float64("completion_ratio", ratio))
			}
		} else {
			logger.Logger.Error("CompletionRatio parsing failed", zap.Error(err))
		}
	} else {
		logger.Logger.Info("CompletionRatio: empty or null")
	}

	logger.Logger.Info("=== END DEBUG ===")
	return nil
}

// DebugAllChannelModelConfigs prints summary information about all channels
func DebugAllChannelModelConfigs() error {
	var channels []Channel
	err := DB.Select("id, name, type, status").Find(&channels).Error
	if err != nil {
		return errors.Wrapf(err, "failed to fetch channels")
	}

	logger.Logger.Info("=== ALL CHANNELS SUMMARY ===")
	for _, channel := range channels {
		// Get full channel data
		var fullChannel Channel
		err := DB.Where("id = ?", channel.Id).First(&fullChannel).Error
		if err != nil {
			logger.Logger.Error("Failed to load channel", zap.Int("channel_id", channel.Id), zap.Error(err))
			continue
		}

		hasModelConfigs := fullChannel.ModelConfigs != nil && *fullChannel.ModelConfigs != "" && *fullChannel.ModelConfigs != "{}"
		hasModelRatio := fullChannel.ModelRatio != nil && *fullChannel.ModelRatio != "" && *fullChannel.ModelRatio != "{}"
		hasCompletionRatio := fullChannel.CompletionRatio != nil && *fullChannel.CompletionRatio != "" && *fullChannel.CompletionRatio != "{}"

		status := "EMPTY"
		if hasModelConfigs {
			status = "UNIFIED"
		} else if hasModelRatio || hasCompletionRatio {
			status = "LEGACY"
		}

		logger.Logger.Info("Channel summary",
			zap.Int("channel_id", channel.Id),
			zap.String("name", channel.Name),
			zap.Int("type", channel.Type),
			zap.String("status", status))

		if hasModelConfigs {
			// Count models in unified format
			var configs map[string]ModelConfigLocal
			if err := json.Unmarshal([]byte(*fullChannel.ModelConfigs), &configs); err == nil {
				var modelNames []string
				for modelName := range configs {
					modelNames = append(modelNames, modelName)
				}
				logger.Logger.Info("Unified models",
					zap.Int("model_count", len(configs)),
					zap.Strings("models", modelNames))
			}
		}

		if hasModelRatio {
			// Count models in legacy format
			var ratios map[string]float64
			if err := json.Unmarshal([]byte(*fullChannel.ModelRatio), &ratios); err == nil {
				var modelNames []string
				for modelName := range ratios {
					modelNames = append(modelNames, modelName)
				}
				logger.Logger.Info("Legacy models",
					zap.Int("model_count", len(ratios)),
					zap.Strings("models", modelNames))
			}
		}
	}
	logger.Logger.Info("=== END SUMMARY ===")
	return nil
}

// FixChannelModelConfigs attempts to fix a specific channel's model configuration
func FixChannelModelConfigs(channelId int) error {
	var channel Channel
	err := DB.Where("id = ?", channelId).First(&channel).Error
	if err != nil {
		return errors.Wrapf(err, "failed to find channel %d", channelId)
	}

	logger.Logger.Info("=== FIXING CHANNEL ===", zap.Int("channel_id", channelId))

	// First, debug current state
	DebugChannelModelConfigs(channelId)

	// Clear any mixed model data and regenerate from adapter defaults
	logger.Logger.Info("Clearing mixed model data and regenerating from adapter defaults...")

	// Clear existing model configs
	emptyConfigs := "{}"
	channel.ModelConfigs = &emptyConfigs

	// Clear legacy data
	channel.ModelRatio = &emptyConfigs
	channel.CompletionRatio = &emptyConfigs

	// Get default pricing for this channel type from adapter
	logger.Logger.Info("Loading default pricing", zap.Int("channel_type", channel.Type))
	defaultPricing := getChannelDefaultPricing(channel.Type)

	if defaultPricing != "" {
		logger.Logger.Info("Setting default model configs", zap.String("default_pricing", defaultPricing))
		channel.ModelConfigs = &defaultPricing
	} else {
		logger.Logger.Info("No default pricing available for this channel type")
	}

	// Save changes
	err = DB.Model(&channel).Updates(map[string]any{
		"model_configs":    channel.ModelConfigs,
		"model_ratio":      channel.ModelRatio,
		"completion_ratio": channel.CompletionRatio,
	}).Error
	if err != nil {
		return errors.Wrapf(err, "failed to save fixed data for channel %d", channelId)
	}
	logger.Logger.Info("Fixed data saved to database")

	// Debug final state
	logger.Logger.Info("Final state after fix:")
	DebugChannelModelConfigs(channelId)

	logger.Logger.Info("=== FIX COMPLETED ===")
	return nil
}

// CleanAllMixedModelData cleans all channels that have mixed model data
func CleanAllMixedModelData() error {
	logger.Logger.Info("=== CLEANING ALL MIXED MODEL DATA ===")

	var channels []Channel
	err := DB.Find(&channels).Error
	if err != nil {
		return errors.Wrapf(err, "failed to fetch channels")
	}

	cleanedCount := 0
	for _, channel := range channels {
		if channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}" {
			// Check if this channel has mixed model data
			configs := channel.GetModelPriceConfigs()
			if len(configs) > 0 {
				hasMixedData := false
				channelTypeModels := getExpectedModelsForChannelType(channel.Type)

				for modelName := range configs {
					if !contains(channelTypeModels, modelName) {
						logger.Logger.Info("Channel has unexpected model",
							zap.Int("channel_id", channel.Id),
							zap.Int("channel_type", channel.Type),
							zap.String("model", modelName))
						hasMixedData = true
						break
					}
				}

				if hasMixedData {
					logger.Logger.Info("Cleaning mixed data for channel", zap.Int("channel_id", channel.Id))
					err := FixChannelModelConfigs(channel.Id)
					if err != nil {
						logger.Logger.Error("Failed to clean channel", zap.Int("channel_id", channel.Id), zap.Error(err))
					} else {
						cleanedCount++
					}
				}
			}
		}
	}

	logger.Logger.Info("Cleaned channels with mixed model data", zap.Int("cleaned_count", cleanedCount))
	logger.Logger.Info("=== CLEANING COMPLETED ===")
	return nil
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}

// Helper function to get expected models for a channel type
func getExpectedModelsForChannelType(channelType int) []string {
	// This is a simplified version - in practice, you'd want to get this from the adapters
	switch channelType {
	case channeltype.OpenAI: // OpenAI
		return []string{"gpt-3.5-turbo", "gpt-4", "gpt-4-turbo", "gpt-4o", "gpt-4o-mini", "text-embedding-ada-002", "text-embedding-3-small", "text-embedding-3-large"}
	case channeltype.OpenAICompatible: // Legacy custom/OpenAI-compatible migration path (commonly used for Claude)
		return []string{"claude-instant-1.2", "claude-2", "claude-2.0", "claude-2.1", "claude-3-opus-20240229", "claude-3-sonnet-20240229", "claude-3-haiku-20240307", "claude-3-5-haiku-20241022", "claude-3-5-sonnet-20240620", "claude-3-5-sonnet-20241022"}
	default:
		return []string{} // Allow all models for unknown types
	}
}

// Helper function to get default pricing for a channel type
func getChannelDefaultPricing(channelType int) string {
	// Generate appropriate default model configs for the channel type
	switch channelType {
	case channeltype.OpenAI: // OpenAI
		return `{
  "gpt-3.5-turbo": {
    "ratio": 0.0015,
    "completion_ratio": 2.0,
    "max_tokens": 16385
  },
  "gpt-4": {
    "ratio": 0.03,
    "completion_ratio": 2.0,
    "max_tokens": 8192
  },
  "gpt-4-turbo": {
    "ratio": 0.01,
    "completion_ratio": 3.0,
    "max_tokens": 128000
  },
  "gpt-4o": {
    "ratio": 0.005,
    "completion_ratio": 3.0,
    "max_tokens": 128000
  },
  "gpt-4o-mini": {
    "ratio": 0.00015,
    "completion_ratio": 4.0,
    "max_tokens": 128000
  }
}`
	case channeltype.OpenAICompatible: // Legacy custom/OpenAI-compatible (historically used for Claude)
		return `{
  "claude-3-haiku-20240307": {
    "ratio": 0.00000025,
    "completion_ratio": 5.0,
    "max_tokens": 200000
  },
  "claude-3-sonnet-20240229": {
    "ratio": 0.000003,
    "completion_ratio": 5.0,
    "max_tokens": 200000
  },
  "claude-3-opus-20240229": {
    "ratio": 0.000015,
    "completion_ratio": 5.0,
    "max_tokens": 200000
  },
  "claude-3-5-sonnet-20240620": {
    "ratio": 0.000003,
    "completion_ratio": 5.0,
    "max_tokens": 200000
  },
  "claude-3-5-sonnet-20241022": {
    "ratio": 0.000003,
    "completion_ratio": 5.0,
    "max_tokens": 200000
  }
}`
	default:
		return "" // No default pricing for unknown types
	}
}

// ValidateAllChannelModelConfigs validates all channels and reports issues
func ValidateAllChannelModelConfigs() error {
	var channels []Channel
	err := DB.Find(&channels).Error
	if err != nil {
		return errors.Wrapf(err, "failed to fetch channels")
	}

	logger.Logger.Info("=== VALIDATION REPORT ===")

	validCount := 0
	issueCount := 0
	emptyCount := 0

	for _, channel := range channels {
		hasModelConfigs := channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}"
		hasLegacyData := (channel.ModelRatio != nil && *channel.ModelRatio != "" && *channel.ModelRatio != "{}") ||
			(channel.CompletionRatio != nil && *channel.CompletionRatio != "" && *channel.CompletionRatio != "{}")

		if !hasModelConfigs && !hasLegacyData {
			emptyCount++
			continue
		}

		if hasModelConfigs {
			// Validate unified format
			var configs map[string]ModelConfigLocal
			if err := json.Unmarshal([]byte(*channel.ModelConfigs), &configs); err != nil {
				logger.Logger.Error("Channel: Invalid ModelConfigs JSON", zap.Int("channel_id", channel.Id), zap.Error(err))
				issueCount++
				continue
			}

			// Validate each model config
			if err := channel.validateModelPriceConfigs(configs); err != nil {
				logger.Logger.Error("Channel: Invalid ModelConfigs data", zap.Int("channel_id", channel.Id), zap.Error(err))
				issueCount++
				continue
			}

			validCount++
		} else if hasLegacyData {
			logger.Logger.Info("Channel has legacy data, needs migration", zap.Int("channel_id", channel.Id))
			issueCount++
		}
	}

	logger.Logger.Info("Validation Summary",
		zap.Int("valid", validCount),
		zap.Int("issues", issueCount),
		zap.Int("empty", emptyCount),
		zap.Int("total", len(channels)))
	logger.Logger.Info("=== END VALIDATION ===")

	return nil
}
