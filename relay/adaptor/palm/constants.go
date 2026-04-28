package palm

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains a legacy PaLM compatibility alias.
// Model list is derived from the keys of this map, eliminating redundancy.
// Google has decommissioned PaLM in favor of Gemini; the old PaLM docs no longer publish a current pricing surface,
// so this file intentionally preserves a minimal placeholder rather than inventing a broader legacy catalog.
var ModelRatios = map[string]adaptor.ModelConfig{
	// Google PaLM Models - retained only for legacy compatibility.
	"PaLM-2": {Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// PalmToolingDefaults notes that legacy PaLM APIs no longer publish built-in tool pricing (retrieved 2026-04-28).
// Source: https://ai.google.dev/palm_docs
var PalmToolingDefaults = adaptor.ChannelToolConfig{}
