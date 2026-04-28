package baichuan

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
var ModelRatios = map[string]adaptor.ModelConfig{
	// Baichuan Models - Based on https://platform.baichuan-ai.com/price
	"Baichuan2-Turbo":         {Ratio: 0.008 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Baichuan2-Turbo-192k":    {Ratio: 0.016 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Baichuan2-53B":           {Ratio: 0.02 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Baichuan-Text-Embedding": {Ratio: 0.002 * ratio.MilliTokensRmb, CompletionRatio: 1}, // Estimated pricing
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// baichuanToolingDefaults documents that Baichuan's public console does not expose tool billing without authentication (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://platform.baichuan-ai.com/price (returns 404)
var BaichuanToolingDefaults = adaptor.ChannelToolConfig{}
