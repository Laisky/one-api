package controller

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// completeGPTImage25Defaults preserves request defaults with a price-only override.
// Parameters: name is the mapped model and cfg is resolved channel pricing.
// Returns: a private copy with unspecified request defaults filled from the catalog;
// other models retain their existing configuration and pricing is never invented.
func completeGPTImage25Defaults(name string, cfg *adaptor.ImagePricingConfig) *adaptor.ImagePricingConfig {
	if !isGPTImage25Model(name) || cfg == nil {
		return cfg
	}
	defaults := openai.ModelRatios[name].Image
	if defaults == nil {
		return cfg
	}
	out := cfg.Clone()
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
