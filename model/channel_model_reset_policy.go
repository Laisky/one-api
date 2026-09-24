package model

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
)

// ChannelModelResetConflict describes a configuration that cannot safely survive
// a reset. Only field names and model identifiers are exposed, never config values.
type ChannelModelResetConflict struct {
	Code   string   `json:"code"`
	Field  string   `json:"field,omitempty"`
	Models []string `json:"models,omitempty"`
}

// Error returns a safe, actionable English description of the reset conflict.
func (conflict *ChannelModelResetConflict) Error() string {
	switch conflict.Code {
	case "unsupported_channel":
		return "Custom compatible channels cannot be reset to provider defaults. Configure their models manually."
	case "no_default_models":
		return "No default models are registered for this channel type."
	case "mapping_conflict":
		return "Model mappings reference models outside the provider default model list. Update the mappings before resetting."
	case "pricing_conflict":
		return "Custom pricing references models outside the provider default model list. Update the pricing before resetting."
	case "inference_profile_conflict":
		return "Inference profiles reference models outside the provider default model list. Update the profiles before resetting."
	default:
		return "Channel configuration is invalid. Correct it before resetting models."
	}
}

// resetConflict wraps a stable conflict code, field name, and sorted model names
// for callers to classify with errors.As without exposing configuration contents.
func resetConflict(code, field string, models []string) error {
	names := slices.Clone(models)
	slices.Sort(names)
	names = slices.Compact(names)
	return errors.WithStack(&ChannelModelResetConflict{Code: code, Field: field, Models: names})
}

// decodeResetMap strictly parses an optional model-keyed JSON object. Null roots
// mean no override; null entries, malformed schemas, and untrimmed keys are refused.
func decodeResetMap[T any](raw *string, field string) (map[string]T, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" || strings.TrimSpace(*raw) == "null" {
		return nil, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*raw), &entries); err != nil {
		return nil, resetConflict("invalid_configuration", field, nil)
	}
	parsed := make(map[string]T, len(entries))
	for name, value := range entries {
		if name == "" || strings.TrimSpace(name) != name || string(value) == "null" {
			return nil, resetConflict("invalid_configuration", field, nil)
		}
		var decoded T
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, resetConflict("invalid_configuration", field, nil)
		}
		parsed[name] = decoded
	}
	return parsed, nil
}

// resetOutsideDefaults returns the model keys in entries that are absent from
// the exact-case provider catalog. It does not modify either input map.
func resetOutsideDefaults[T any](entries map[string]T, supported map[string]bool) []string {
	var outside []string
	for name := range entries {
		if !supported[name] {
			outside = append(outside, name)
		}
	}
	return outside
}

// validateResetOverrides checks mappings, current and legacy pricing, and AWS
// profile references against supported models without rewriting any override.
func validateResetOverrides(channel *Channel, supported map[string]bool) error {
	mapping, err := decodeResetMap[string](channel.ModelMapping, "model_mapping")
	if err != nil {
		return errors.Wrap(err, "validate reset mappings")
	}
	outside := resetOutsideDefaults(mapping, supported)
	for _, target := range mapping {
		if strings.TrimSpace(target) == "" || strings.TrimSpace(target) != target {
			return resetConflict("invalid_configuration", "model_mapping", nil)
		}
		if !supported[target] {
			outside = append(outside, target)
		}
	}
	if len(outside) > 0 {
		return resetConflict("mapping_conflict", "model_mapping", outside)
	}

	prices, err := decodeResetMap[ModelConfigLocal](channel.ModelConfigs, "model_configs")
	if err != nil {
		return errors.Wrap(err, "validate reset pricing")
	}
	if outside := resetOutsideDefaults(prices, supported); len(outside) > 0 {
		return resetConflict("pricing_conflict", "model_configs", outside)
	}
	for _, price := range prices {
		if _, err := normalizeModelConfigLocal(price); err != nil {
			return resetConflict("invalid_configuration", "model_configs", nil)
		}
	}
	if err := channel.validateModelPriceConfigs(prices); err != nil {
		return resetConflict("invalid_configuration", "model_configs", nil)
	}
	for _, legacy := range []struct {
		field string
		raw   *string
	}{{"model_ratio", channel.ModelRatio}, {"completion_ratio", channel.CompletionRatio}} {
		ratios, err := decodeResetMap[float64](legacy.raw, legacy.field)
		if err != nil {
			return errors.Wrap(err, "validate legacy reset pricing")
		}
		if outside := resetOutsideDefaults(ratios, supported); len(outside) > 0 {
			return resetConflict("pricing_conflict", legacy.field, outside)
		}
		for _, ratio := range ratios {
			if ratio < 0 {
				return resetConflict("invalid_configuration", legacy.field, nil)
			}
		}
	}

	profiles, err := decodeResetMap[string](channel.InferenceProfileArnMap, "inference_profile_arn_map")
	if err != nil {
		return errors.Wrap(err, "validate reset inference profiles")
	}
	if outside := resetOutsideDefaults(profiles, supported); len(outside) > 0 {
		return resetConflict("inference_profile_conflict", "inference_profile_arn_map", outside)
	}
	for _, profile := range profiles {
		if strings.TrimSpace(profile) == "" {
			return resetConflict("invalid_configuration", "inference_profile_arn_map", nil)
		}
	}
	if _, err := channel.LoadConfig(); err != nil {
		return resetConflict("invalid_configuration", "config", nil)
	}
	return nil
}

// planChannelModelReset validates a channel against its provider's catalog and
// returns a copy with every default model visible. The original row and catalog
// remain untouched, including on failure; unrelated channel settings are retained.
func planChannelModelReset(channel *Channel, defaults []string) (*Channel, error) {
	switch channel.Type {
	case channeltype.Custom, channeltype.OpenAICompatible, channeltype.ClaudeCompatible:
		return nil, resetConflict("unsupported_channel", "type", nil)
	}
	if channel.Type <= channeltype.Unknown || channel.Type >= channeltype.Dummy {
		return nil, resetConflict("no_default_models", "type", nil)
	}

	supported := make(map[string]bool, len(defaults))
	names := make([]string, 0, len(defaults))
	for _, name := range defaults {
		name = strings.TrimSpace(name)
		if name == "" || supported[name] {
			continue
		}
		if strings.Contains(name, ",") {
			return nil, resetConflict("no_default_models", "models", nil)
		}
		supported[name] = true
		names = append(names, name)
	}
	// An empty Models field means unrestricted routing in legacy code. Never
	// mistake a missing catalog for a valid empty reset.
	if len(names) == 0 {
		return nil, resetConflict("no_default_models", "models", nil)
	}
	if err := validateResetOverrides(channel, supported); err != nil {
		return nil, errors.Wrap(err, "validate channel before model reset")
	}

	planned := *channel
	planned.Models = strings.Join(names, ",")
	planned.HiddenModels = nil
	if planned.TestingModel != nil && *planned.TestingModel != ChannelTestingModelSkip && !supported[*planned.TestingModel] {
		planned.TestingModel = nil
	}
	return &planned, nil
}
