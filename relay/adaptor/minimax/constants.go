package minimax

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Minimax pricing: https://api.minimax.chat/document/price
var ModelRatios = map[string]adaptor.ModelConfig{
	// Minimax Models - Based on https://api.minimax.chat/document/price
	"abab6.5-chat":    {Ratio: 0.03 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"abab6.5s-chat":   {Ratio: 0.01 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"abab6-chat":      {Ratio: 0.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"abab5.5-chat":    {Ratio: 0.015 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"abab5.5s-chat":   {Ratio: 0.005 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"MiniMax-VL-01":   {Ratio: 0.02 * ratio.MilliTokensRmb, CompletionRatio: 1},  // Estimated pricing
	"MiniMax-Text-01": {Ratio: 0.015 * ratio.MilliTokensRmb, CompletionRatio: 1}, // Estimated pricing
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MinimaxToolingDefaults notes that MiniMax's pricing reference lists model rates only (no tool pricing) as of 2025-11-12.
// Source: https://r.jina.ai/https://api.minimax.chat/document/price
var MinimaxToolingDefaults = adaptor.ChannelToolConfig{}
