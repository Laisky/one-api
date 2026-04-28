package lingyiwanwu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on LingYi WanWu pricing: https://platform.lingyiwanwu.com/docs#%E6%A8%A1%E5%9E%8B%E4%B8%8E%E8%AE%A1%E8%B4%B9
var ModelRatios = map[string]adaptor.ModelConfig{
	// LingYi WanWu Models - Based on https://platform.lingyiwanwu.com/docs
	"yi-lightning": {Ratio: 0.99 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"yi-vision-v2": {Ratio: 6 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// LingyiWanwuToolingDefaults notes that LingYi WanWu's pricing docs list model rates only (no tool metering) as of 2025-11-12.
// Source: https://r.jina.ai/https://platform.lingyiwanwu.com/docs#%E6%A8%A1%E5%9E%8B%E4%B8%8E%E8%AE%A1%E8%B4%B9
var LingyiWanwuToolingDefaults = adaptor.ChannelToolConfig{}
