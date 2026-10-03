package model

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

// GetModelPriceConfigs returns the channel-specific model price configurations in the new unified format
func (channel *Channel) GetModelPriceConfigs() map[string]ModelConfigLocal {
	return channel.GetModelPriceConfigsWithContext(context.Background())
}

// GetModelPriceConfigsWithContext parses channel pricing with a context-aware
// logger. Parameters: ctx carries request correlation when pricing is resolved
// during an HTTP request. Returns: parsed model pricing or nil when absent or
// malformed.
func (channel *Channel) GetModelPriceConfigsWithContext(ctx context.Context) map[string]ModelConfigLocal {
	if channel.ModelConfigs == nil || *channel.ModelConfigs == "" || *channel.ModelConfigs == "{}" {
		return nil
	}

	modelPriceConfigs := make(map[string]ModelConfigLocal)
	err := json.Unmarshal([]byte(*channel.ModelConfigs), &modelPriceConfigs)
	if err != nil {
		logger.FromContext(ctx).Error("failed to unmarshal model price configs for channel",
			append(channel.Ref().Zap(), zap.Error(err))...)
		return nil
	}

	return modelPriceConfigs
}

// SetModelPriceConfigs sets the channel-specific model price configurations in the new unified format
func (channel *Channel) SetModelPriceConfigs(modelPriceConfigs map[string]ModelConfigLocal) error {
	if len(modelPriceConfigs) == 0 {
		channel.ModelConfigs = nil
		return nil
	}

	cleaned := make(map[string]ModelConfigLocal, len(modelPriceConfigs))
	for rawName, cfg := range modelPriceConfigs {
		trimmedName := strings.TrimSpace(rawName)
		if trimmedName == "" {
			return errors.New("model name cannot be empty")
		}
		normalized, err := normalizeModelConfigLocal(cfg)
		if err != nil {
			return errors.Wrapf(err, "normalize model config for %s", rawName)
		}
		cleaned[trimmedName] = normalized
	}

	// Validate the configurations before setting
	if err := channel.validateModelPriceConfigs(cleaned); err != nil {
		return errors.Wrap(err, "invalid model price configurations")
	}

	jsonBytes, err := json.Marshal(cleaned)
	if err != nil {
		return errors.Wrap(err, "failed to marshal model price configurations")
	}

	jsonStr := string(jsonBytes)
	channel.ModelConfigs = &jsonStr
	return nil
}

// GetModelPriceConfig returns the price configuration for a specific model
func (channel *Channel) GetModelPriceConfig(modelName string) *ModelConfigLocal {
	configs := channel.GetModelPriceConfigs()
	if configs == nil {
		return nil
	}

	if config, exists := configs[modelName]; exists {
		return &config
	}

	return nil
}

// GetModelRatioFromConfigs extracts model ratios from the unified ModelConfigs
func (channel *Channel) GetModelRatioFromConfigs() map[string]float64 {
	return channel.GetModelRatioFromConfigsWithContext(context.Background())
}

// GetModelRatioFromConfigsWithContext extracts model ratios while preserving
// request correlation in ctx. Parameters: ctx carries the request logger.
// Returns: non-zero channel model ratios or nil when none are configured.
func (channel *Channel) GetModelRatioFromConfigsWithContext(ctx context.Context) map[string]float64 {
	configs := channel.GetModelPriceConfigsWithContext(ctx)
	if configs == nil {
		return nil
	}

	modelRatios := make(map[string]float64)
	for modelName, config := range configs {
		if config.Ratio != 0 {
			modelRatios[modelName] = config.Ratio
		}
	}

	if len(modelRatios) == 0 {
		return nil
	}

	return modelRatios
}

// GetCompletionRatioFromConfigs extracts completion ratios from the unified ModelConfigs
func (channel *Channel) GetCompletionRatioFromConfigs() map[string]float64 {
	return channel.GetCompletionRatioFromConfigsWithContext(context.Background())
}

// GetCompletionRatioFromConfigsWithContext extracts completion ratios while
// preserving request correlation in ctx. Parameters: ctx carries the request
// logger. Returns: non-zero completion ratios or nil when none are configured.
func (channel *Channel) GetCompletionRatioFromConfigsWithContext(ctx context.Context) map[string]float64 {
	configs := channel.GetModelPriceConfigsWithContext(ctx)
	if configs == nil {
		return nil
	}

	completionRatios := make(map[string]float64)
	for modelName, config := range configs {
		if config.CompletionRatio != 0 {
			completionRatios[modelName] = config.CompletionRatio
		}
	}

	if len(completionRatios) == 0 {
		return nil
	}

	return completionRatios
}

func (channel *Channel) GetInferenceProfileArnMap() map[string]string {
	return channel.GetInferenceProfileArnMapWithContext(context.Background())
}

// GetInferenceProfileArnMapWithContext parses inference-profile mappings with a
// context-aware logger. Parameters: ctx carries request correlation. Returns:
// the ARN mapping or nil when absent or malformed.
func (channel *Channel) GetInferenceProfileArnMapWithContext(ctx context.Context) map[string]string {
	if channel.InferenceProfileArnMap == nil || *channel.InferenceProfileArnMap == "" || *channel.InferenceProfileArnMap == "{}" {
		return nil
	}
	arnMap := make(map[string]string)
	err := json.Unmarshal([]byte(*channel.InferenceProfileArnMap), &arnMap)
	if err != nil {
		logger.FromContext(ctx).Error("failed to unmarshal inference profile ARN map for channel",
			append(channel.Ref().Zap(), zap.Error(err))...)
		return nil
	}
	return arnMap
}

func (channel *Channel) SetInferenceProfileArnMap(arnMap map[string]string) error {
	if len(arnMap) == 0 {
		channel.InferenceProfileArnMap = nil
		return nil
	}

	// Validate that keys and values are not empty
	for key, value := range arnMap {
		if key == "" || value == "" {
			return errors.New("inference profile ARN map cannot contain empty keys or values")
		}
	}

	jsonBytes, err := json.Marshal(arnMap)
	if err != nil {
		return errors.Wrap(err, "marshal inference profile ARN map")
	}
	jsonStr := string(jsonBytes)
	channel.InferenceProfileArnMap = &jsonStr
	return nil
}

// ValidateInferenceProfileArnMapJSON validates a JSON string for inference profile ARN mapping
func ValidateInferenceProfileArnMapJSON(jsonStr string) error {
	if jsonStr == "" {
		return nil // Empty is allowed
	}

	var arnMap map[string]string
	err := json.Unmarshal([]byte(jsonStr), &arnMap)
	if err != nil {
		return errors.Errorf("invalid JSON format: %v", err)
	}

	// Validate that keys and values are not empty
	for key, value := range arnMap {
		if key == "" {
			return errors.New("inference profile ARN map cannot contain empty keys")
		}
		if value == "" {
			return errors.New("inference profile ARN map cannot contain empty values")
		}
	}

	return nil
}

// GetModelRatio returns the channel-specific model ratio map
// DEPRECATED: Use GetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) GetModelRatio() map[string]float64 {
	return channel.GetModelRatioWithContext(context.Background())
}

// GetModelRatioWithContext returns the legacy channel ratio map with request-scoped diagnostics.
// Parameters: ctx carries request logging.
// Returns: parsed channel ratios, or nil when absent or malformed.
func (channel *Channel) GetModelRatioWithContext(ctx context.Context) map[string]float64 {
	if channel.ModelRatio == nil || *channel.ModelRatio == "" || *channel.ModelRatio == "{}" {
		return nil
	}
	modelRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(*channel.ModelRatio), &modelRatio)
	if err != nil {
		logger.FromContext(ctx).Error("failed to unmarshal model ratio for channel",
			append(channel.Ref().Zap(), zap.Error(err))...)
		return nil
	}
	return modelRatio
}

// GetCompletionRatio returns the channel-specific completion ratio map
// DEPRECATED: Use GetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) GetCompletionRatio() map[string]float64 {
	return channel.GetCompletionRatioWithContext(context.Background())
}

// GetCompletionRatioWithContext returns the legacy completion ratio map with request-scoped diagnostics.
// Parameters: ctx carries request logging.
// Returns: parsed completion ratios, or nil when absent or malformed.
func (channel *Channel) GetCompletionRatioWithContext(ctx context.Context) map[string]float64 {
	if channel.CompletionRatio == nil || *channel.CompletionRatio == "" || *channel.CompletionRatio == "{}" {
		return nil
	}
	completionRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(*channel.CompletionRatio), &completionRatio)
	if err != nil {
		logger.FromContext(ctx).Error("failed to unmarshal completion ratio for channel",
			append(channel.Ref().Zap(), zap.Error(err))...)
		return nil
	}
	return completionRatio
}

// SetModelRatio sets the channel-specific model ratio map
// DEPRECATED: Use SetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) SetModelRatio(modelRatio map[string]float64) error {
	if len(modelRatio) == 0 {
		channel.ModelRatio = nil
		return nil
	}
	jsonBytes, err := json.Marshal(modelRatio)
	if err != nil {
		return errors.Wrap(err, "marshal channel model ratio")
	}
	jsonStr := string(jsonBytes)
	channel.ModelRatio = &jsonStr
	return nil
}

// SetCompletionRatio sets the channel-specific completion ratio map
// DEPRECATED: Use SetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) SetCompletionRatio(completionRatio map[string]float64) error {
	if len(completionRatio) == 0 {
		channel.CompletionRatio = nil
		return nil
	}
	jsonBytes, err := json.Marshal(completionRatio)
	if err != nil {
		return errors.Wrap(err, "marshal channel completion ratio")
	}
	jsonStr := string(jsonBytes)
	channel.CompletionRatio = &jsonStr
	return nil
}
