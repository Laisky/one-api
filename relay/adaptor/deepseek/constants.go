package deepseek

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on official DeepSeek pricing: https://api-docs.deepseek.com/quick_start/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Legacy models (to be deprecated; kept for backward compatibility)
	"deepseek-chat": {
		Ratio:            0.28 * ratio.MilliTokensUsd,
		CachedInputRatio: 0.028 * ratio.MilliTokensUsd,
		CompletionRatio:  0.42 / 0.28,
	},
	"deepseek-reasoner": {
		Ratio:            0.28 * ratio.MilliTokensUsd,
		CachedInputRatio: 0.028 * ratio.MilliTokensUsd,
		CompletionRatio:  0.42 / 0.28,
	},
	// Current models (retrieved 2026-04-24)
	// deepseek-v4-flash: $0.14/1M input (cache miss), $0.028/1M input (cache hit), $0.28/1M output, 1M context
	"deepseek-v4-flash": {
		Ratio:            0.14 * ratio.MilliTokensUsd,
		CachedInputRatio: 0.028 * ratio.MilliTokensUsd,
		CompletionRatio:  0.28 / 0.14,
	},
	// deepseek-v4-pro: $1.74/1M input (cache miss), $0.145/1M input (cache hit), $3.48/1M output, 1M context
	"deepseek-v4-pro": {
		Ratio:            1.74 * ratio.MilliTokensUsd,
		CachedInputRatio: 0.145 * ratio.MilliTokensUsd,
		CompletionRatio:  3.48 / 1.74,
	},
}

// DeepseekToolingDefaults documents that DeepSeek does not publish built-in tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://api-docs.deepseek.com/quick_start/pricing
var DeepseekToolingDefaults = adaptor.ChannelToolConfig{}
