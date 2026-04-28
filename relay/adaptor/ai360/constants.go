package ai360

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
var ModelRatios = map[string]adaptor.ModelConfig{
	// AI360 Models - Based on historical pricing
	"360GPT_S2_V9":              {Ratio: 0.8572 * ratio.MilliTokensUsd, CompletionRatio: 1}, // CNY 0.012 / 1M tokens
	"embedding-bert-512-v1":     {Ratio: 0.0715 * ratio.MilliTokensUsd, CompletionRatio: 1}, // CNY 0.001 / 1M tokens
	"embedding_s1_v1":           {Ratio: 0.0715 * ratio.MilliTokensUsd, CompletionRatio: 1}, // CNY 0.001 / 1M tokens
	"semantic_similarity_s1_v1": {Ratio: 0.0715 * ratio.MilliTokensUsd, CompletionRatio: 1}, // CNY 0.001 / 1M tokens
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// AI360ToolingDefaults documents that AI360 does not publish built-in tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://ai.360.com/platform (login wall, no public tool catalog)
var AI360ToolingDefaults = adaptor.ChannelToolConfig{}
