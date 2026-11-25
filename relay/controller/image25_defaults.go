package controller

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// completeStrictGPTImageDefaults preserves request defaults with a price-only override.
// Parameters: name is the mapped model and cfg is resolved channel pricing, nil
// when no image tariff resolved (an operator-priced host whose channel prices
// only tokens). Returns: a private copy, or a new price-free configuration for
// nil, with unspecified request defaults filled from the catalog, so such a
// request is never sent the legacy "1024x1024"/"standard" defaults; other
// models retain their existing configuration and pricing is never invented.
func completeStrictGPTImageDefaults(name string, cfg *adaptor.ImagePricingConfig) *adaptor.ImagePricingConfig {
	if !usesStrictGPTImageTokenBilling(name) {
		return cfg
	}
	defaults := openai.ModelRatios[name].Image
	if defaults == nil {
		return cfg
	}
	out := &adaptor.ImagePricingConfig{}
	if cfg != nil {
		out = cfg.Clone()
	}
	if out.DefaultSize == "" {
		out.DefaultSize = defaults.DefaultSize
	}
	if out.DefaultQuality == "" {
		out.DefaultQuality = defaults.DefaultQuality
	}
	if out.PromptTokenLimit == 0 {
		out.PromptTokenLimit = defaults.PromptTokenLimit
	}
	if out.MinImages == 0 {
		out.MinImages = defaults.MinImages
	}
	if out.MaxImages == 0 {
		out.MaxImages = defaults.MaxImages
	}
	return out
}
