package baidu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Pricing from https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9dlf
var ModelRatios = map[string]adaptor.ModelConfig{
	// ERNIE 4.0 Models
	"ERNIE-4.0-8K": {Ratio: 12 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// ERNIE 3.5 Models
	"ERNIE-3.5-8K":      {Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"ERNIE-3.5-8K-0205": {Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"ERNIE-3.5-8K-1222": {Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"ERNIE-Bot-8K":      {Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"ERNIE-3.5-4K-0205": {Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// ERNIE Speed Models
	"ERNIE-Speed-8K":   {Ratio: 0.4 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"ERNIE-Speed-128K": {Ratio: 0.4 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// ERNIE Lite Models
	"ERNIE-Lite-8K-0922": {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"ERNIE-Lite-8K-0308": {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// ERNIE Tiny Models
	"ERNIE-Tiny-8K": {Ratio: 0.4 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Other Models
	"BLOOMZ-7B": {Ratio: 0.4 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Embedding Models
	"Embedding-V1": {Ratio: 0.2 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"bge-large-zh": {Ratio: 0.2 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"bge-large-en": {Ratio: 0.2 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// TAO Models
	"tao-8k": {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// BaiduToolingDefaults notes that Wenxin ModelBuilder documentation does not disclose per-tool billing publicly (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9dlf
var BaiduToolingDefaults = adaptor.ChannelToolConfig{}
