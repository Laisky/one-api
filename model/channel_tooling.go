package model

import (
	"maps"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

// ToolPricingLocal is the channel-scoped representation of adaptor.ToolPricingConfig.
// Administrators can express pricing either in USD per call or direct quota units.
type ToolPricingLocal struct {
	UsdPerCall   float64 `json:"usd_per_call,omitempty"`
	QuotaPerCall int64   `json:"quota_per_call,omitempty"`
}

// normalizeToolWhitelist trims, deduplicates, and validates the provided tooling whitelist.
func normalizeToolWhitelist(list []string) ([]string, error) {
	if len(list) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(list))
	normalized := make([]string, 0, len(list))
	for _, raw := range list {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, errors.New("tool whitelist cannot contain empty entries")
		}
		lower := strings.ToLower(trimmed)
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// normalizeToolPricingMap validates provider pricing definitions and removes empty entries.
func normalizeToolPricingMap(pricing map[string]ToolPricingLocal) (map[string]ToolPricingLocal, error) {
	if len(pricing) == 0 {
		return nil, nil
	}
	normalized := make(map[string]ToolPricingLocal, len(pricing))
	for rawName, value := range pricing {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, errors.New("tool pricing cannot contain empty tool names")
		}
		if value.UsdPerCall < 0 {
			return nil, errors.Errorf("tool %s usd_per_call cannot be negative", name)
		}
		if value.QuotaPerCall < 0 {
			return nil, errors.Errorf("tool %s quota_per_call cannot be negative", name)
		}
		normalized[name] = value
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// normalizeChannelToolingConfig applies whitelist/pricing validation to a channel tooling configuration.
func normalizeChannelToolingConfig(cfg *ChannelToolingConfig) (*ChannelToolingConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	normalized := &ChannelToolingConfig{}
	list, err := normalizeToolWhitelist(cfg.Whitelist)
	if err != nil {
		return nil, errors.Wrap(err, "normalize tool whitelist")
	}
	if len(list) > 0 {
		normalized.Whitelist = list
	}
	pricing, err := normalizeToolPricingMap(cfg.Pricing)
	if err != nil {
		return nil, errors.Wrap(err, "normalize tool pricing")
	}
	if len(pricing) > 0 {
		normalized.Pricing = pricing
	}
	if len(normalized.Whitelist) == 0 && len(normalized.Pricing) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// cloneChannelToolingConfig produces a deep copy of the tooling configuration so callers can mutate safely.
func cloneChannelToolingConfig(cfg *ChannelToolingConfig) *ChannelToolingConfig {
	if cfg == nil {
		return nil
	}
	clone := &ChannelToolingConfig{}
	if len(cfg.Whitelist) > 0 {
		clone.Whitelist = append([]string(nil), cfg.Whitelist...)
	}
	if len(cfg.Pricing) > 0 {
		clone.Pricing = make(map[string]ToolPricingLocal, len(cfg.Pricing))
		maps.Copy(clone.Pricing, cfg.Pricing)
	}
	return clone
}

// ChannelToolingConfig captures channel-scoped built-in tool policy and pricing.
type ChannelToolingConfig struct {
	Whitelist []string                    `json:"whitelist,omitempty"`
	Pricing   map[string]ToolPricingLocal `json:"pricing,omitempty"`
}

// GetToolingConfig returns the channel-level tooling policy configuration, if any.
func (channel *Channel) GetToolingConfig() *ChannelToolingConfig {
	cfg, err := channel.LoadConfig()
	if err != nil {
		logger.Logger.Error("failed to load channel config for tooling",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}
	return cloneChannelToolingConfig(cfg.Tooling)
}

// SetToolingConfig updates the channel-level tooling configuration stored in the config JSON blob.
func (channel *Channel) SetToolingConfig(tooling *ChannelToolingConfig) error {
	normalized, err := normalizeChannelToolingConfig(tooling)
	if err != nil {
		return errors.Wrap(err, "invalid tooling configuration")
	}
	cfg, err := channel.LoadConfig()
	if err != nil {
		return errors.Wrap(err, "load channel config")
	}
	cfg.Tooling = cloneChannelToolingConfig(normalized)
	return channel.storeConfig(cfg)
}
