package doubao

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Doubao pricing: https://www.volcengine.com/docs/82379/1099320
var ModelRatios = map[string]adaptor.ModelConfig{
	// Doubao Pro Models
	"Doubao-pro-128k": {Ratio: 0.005 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Doubao-pro-32k":  {Ratio: 0.002 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Doubao-pro-4k":   {Ratio: 0.0008 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Doubao Lite Models
	"Doubao-lite-128k": {Ratio: 0.0008 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Doubao-lite-32k":  {Ratio: 0.0006 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Doubao-lite-4k":   {Ratio: 0.0003 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Embedding Models
	"Doubao-embedding": {Ratio: 0.0002 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// DoubaoToolingDefaults documents that Bytedance's Doubao cloud pricing does not list per-tool fees publicly (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.volcengine.com/docs/82379/1099320
var DoubaoToolingDefaults = adaptor.ChannelToolConfig{}
