package palm

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Google PaLM pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Google PaLM Models - estimated pricing
	"PaLM-2": {Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// PalmToolingDefaults notes that legacy PaLM APIs no longer publish built-in tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://cloud.google.com/palm
var PalmToolingDefaults = adaptor.ChannelToolConfig{}
