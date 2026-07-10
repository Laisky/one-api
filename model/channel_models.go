package model

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

// splitCSVNames trims comma-separated names, removes empty entries, and deduplicates them.
// The caseSensitive parameter selects exact model-name identity when true and legacy
// case-insensitive group identity when false. It returns nil when no names remain.
func splitCSVNames(raw string, caseSensitive bool) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key := part
		if !caseSensitive {
			key = strings.ToLower(part)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeHiddenModelArray validates the hidden-model JSON array, trims its
// entries, and deduplicates them case-insensitively while retaining the first
// entry's casing. It returns nil for an absent or empty normalized list.
func normalizeHiddenModelArray(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" || strings.EqualFold(trimmed, "null") {
		return nil, nil
	}

	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, errors.Wrap(err, "hidden_models must be a JSON array of strings")
	}

	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}

		key := strings.ToLower(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, errors.Wrap(err, "marshal normalized hidden_models")
	}
	result := string(data)
	return &result, nil
}

// NormalizeHiddenModels validates and normalizes the channel hidden-model payload.
// It preserves the first configured casing while deduplicating case-insensitively
// for backward compatibility, and returns a wrapped validation error on failure.
func (channel *Channel) NormalizeHiddenModels() error {
	normalized, err := normalizeHiddenModelArray(channel.HiddenModels)
	if err != nil {
		return errors.Wrap(err, "normalize hidden models")
	}
	channel.HiddenModels = normalized
	return nil
}

// GetSupportedModelNames returns the trimmed, case-sensitive model identifiers
// from the channel's comma-separated Models field. Exact duplicates are removed.
func (channel *Channel) GetSupportedModelNames() []string {
	return splitCSVNames(channel.Models, true)
}

// GetGroupNames returns the trimmed group names configured on the channel.
// Case-insensitive duplicates are removed to preserve existing group behavior.
func (channel *Channel) GetGroupNames() []string {
	return splitCSVNames(channel.Group, false)
}

// GetHiddenModels returns lowercase hidden-model membership keys for the channel.
// Hidden names match supported models case-insensitively for backward compatibility;
// malformed persisted JSON is logged and treated as having no hidden models.
func (channel *Channel) GetHiddenModels() map[string]struct{} {
	if channel.HiddenModels == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*channel.HiddenModels)
	if trimmed == "" || trimmed == "[]" || strings.EqualFold(trimmed, "null") {
		return nil
	}

	var rawHidden []string
	if err := json.Unmarshal([]byte(trimmed), &rawHidden); err != nil {
		logger.Logger.Warn("failed to unmarshal hidden models for channel",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}

	supportedModels := channel.GetSupportedModelNames()
	if len(supportedModels) == 0 {
		return nil
	}
	supported := make(map[string]struct{}, len(supportedModels))
	for _, modelName := range supportedModels {
		supported[strings.ToLower(modelName)] = struct{}{}
	}

	hidden := make(map[string]struct{}, len(rawHidden))
	for _, value := range rawHidden {
		item := strings.ToLower(strings.TrimSpace(value))
		if item == "" {
			continue
		}
		if _, ok := supported[item]; !ok {
			continue
		}
		hidden[item] = struct{}{}
	}
	if len(hidden) == 0 {
		return nil
	}
	return hidden
}

// IsModelHidden reports whether name is hidden from public selection on this channel.
// The comparison is case-insensitive to preserve existing hidden-model configurations.
func (channel *Channel) IsModelHidden(name string) bool {
	item := strings.ToLower(strings.TrimSpace(name))
	if item == "" {
		return false
	}
	hidden := channel.GetHiddenModels()
	if len(hidden) == 0 {
		return false
	}
	_, exists := hidden[item]
	return exists
}

// SupportsModel reports whether the channel allows modelName for routing.
// Exact-case supported names and mapping targets are accepted; an empty configured
// model list allows every model, and an empty requested name remains unrestricted.
func (channel *Channel) SupportsModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return true
	}
	supported := channel.GetSupportedModelNames()
	if len(supported) == 0 {
		return true
	}
	for _, name := range supported {
		if name == modelName {
			return true
		}
	}
	if mapping := channel.GetModelMapping(); mapping != nil {
		if mapped := strings.TrimSpace(mapping[modelName]); mapped != "" {
			for _, name := range supported {
				if name == mapped {
					return true
				}
			}
		}
	}
	return false
}
