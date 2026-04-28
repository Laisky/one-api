package moonshot

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Moonshot pricing: https://platform.moonshot.cn/docs/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Moonshot legacy models (keep for compatibility)
	// "moonshot-v1-8k":   {Ratio: 12 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// "moonshot-v1-32k":  {Ratio: 24 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// "moonshot-v1-128k": {Ratio: 60 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Kimi-K2 models (2025-11)
	// All prices per 1M tokens, in RMB
	// input: cache-hit, input: cache-miss, output, context
	"kimi-k2-0905-preview": {
		Ratio:            4 * ratio.MilliTokensRmb,  // input (cache-miss)
		CompletionRatio:  16 * ratio.MilliTokensRmb, // output
		CachedInputRatio: 1 * ratio.MilliTokensRmb,  // input (cache-hit)
		// MaxTokens:        262144,
	},
	"kimi-k2-0711-preview": {
		Ratio:            4 * ratio.MilliTokensRmb,
		CompletionRatio:  16 * ratio.MilliTokensRmb,
		CachedInputRatio: 1 * ratio.MilliTokensRmb,
		// MaxTokens:        131072,
	},
	"kimi-k2-turbo-preview": {
		Ratio:            8 * ratio.MilliTokensRmb,
		CompletionRatio:  58 * ratio.MilliTokensRmb,
		CachedInputRatio: 1 * ratio.MilliTokensRmb,
		// MaxTokens:        262144,
	},
	"kimi-k2-thinking": {
		Ratio:            4 * ratio.MilliTokensRmb,
		CompletionRatio:  16 * ratio.MilliTokensRmb,
		CachedInputRatio: 1 * ratio.MilliTokensRmb,
		// MaxTokens:        262144,
	},
	"kimi-k2-thinking-turbo": {
		Ratio:            8 * ratio.MilliTokensRmb,
		CompletionRatio:  58 * ratio.MilliTokensRmb,
		CachedInputRatio: 1 * ratio.MilliTokensRmb,
		// MaxTokens:        262144,
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MoonshotToolingDefaults notes that Moonshot's pricing page lists model fees only; no tool metering is published (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://platform.moonshot.cn/docs/pricing
var MoonshotToolingDefaults = adaptor.ChannelToolConfig{}
