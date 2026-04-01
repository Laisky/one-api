package zhipu

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Zhipu pricing: https://open.bigmodel.cn/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// GLM-4.6 (tiered)
	"glm-4.6": {
		Ratio:            2 * ratio.MilliTokensRmb,   // CNY 2/1M input tokens (input length [0,32], output [0,0.2])
		CompletionRatio:  4,                          // CNY 8/1M output tokens
		CachedInputRatio: 0.4 * ratio.MilliTokensRmb, // CNY 0.4/1M cached input
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 3 * ratio.MilliTokensRmb, CompletionRatio: 14.0 / 3.0, CachedInputRatio: 0.6 * ratio.MilliTokensRmb, InputTokenThreshold: 0},  // input [0,32], output [0.2+]
			{Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 4.0, CachedInputRatio: 0.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32}, // input [32,200]
		},
	},
	// GLM-4.5 (tiered)
	"glm-4.5": {
		Ratio:            2 * ratio.MilliTokensRmb,
		CompletionRatio:  4,
		CachedInputRatio: 0.4 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 3 * ratio.MilliTokensRmb, CompletionRatio: 14.0 / 3.0, CachedInputRatio: 0.6 * ratio.MilliTokensRmb, InputTokenThreshold: 0},
			{Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 4.0, CachedInputRatio: 0.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.5-X (tiered)
	"glm-4.5-x": {
		Ratio:            8 * ratio.MilliTokensRmb,
		CompletionRatio:  2,
		CachedInputRatio: 1.6 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 12 * ratio.MilliTokensRmb, CompletionRatio: 32.0 / 12.0, CachedInputRatio: 2.4 * ratio.MilliTokensRmb, InputTokenThreshold: 0},
			{Ratio: 16 * ratio.MilliTokensRmb, CompletionRatio: 64.0 / 16.0, CachedInputRatio: 3.2 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.5-Air (tiered)
	"glm-4.5-air": {
		Ratio:            0.8 * ratio.MilliTokensRmb,
		CompletionRatio:  2.5, // CNY 2/0.8 = 2.5
		CachedInputRatio: 0.16 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 6.0 / 0.8, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 0},
			{Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 8.0 / 1.2, CachedInputRatio: 0.24 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.5-AirX (tiered)
	"glm-4.5-airx": {
		Ratio:            4 * ratio.MilliTokensRmb,
		CompletionRatio:  3,
		CachedInputRatio: 0.8 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 4.0, CachedInputRatio: 0.8 * ratio.MilliTokensRmb, InputTokenThreshold: 0},
			{Ratio: 8 * ratio.MilliTokensRmb, CompletionRatio: 32.0 / 8.0, CachedInputRatio: 1.6 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.5-Flash (free)
	"glm-4.5-flash": {
		Ratio:            0,
		CompletionRatio:  1,
		CachedInputRatio: 0,
	},
	// GLM Zero Models
	"glm-zero-preview": {Ratio: 0.7 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// GLM-3 Models
	"glm-3-turbo": {Ratio: 0.005 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// GLM Vision Models
	"glm-4v-plus":  {Ratio: 0.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"glm-4v":       {Ratio: 0.05 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"glm-4v-flash": {Ratio: 0.001 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// CogView Image Models
	"cogview-3-plus":  {Ratio: 0.08 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogview-3":       {Ratio: 0.04 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogview-3-flash": {Ratio: 0.008 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogviewx":        {Ratio: 0.04 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogviewx-flash":  {Ratio: 0.008 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Character and Code Models
	"charglm-4":  {Ratio: 0.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"emohaa":     {Ratio: 0.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"codegeex-4": {Ratio: 0.001 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Embedding Models
	"embedding-3": {Ratio: 0.0005 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"embedding-2": {Ratio: 0.0005 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// OCR Models
	"glm-ocr": {Ratio: 0.2 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// ZhipuToolingDefaults captures Open BigModel's published search-tool pricing tiers (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://open.bigmodel.cn/pricing
var ZhipuToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"search_std":       {UsdPerCall: 0.01},
		"search_pro":       {UsdPerCall: 0.03},
		"search_pro_sogou": {UsdPerCall: 0.05},
		"search_pro_quark": {UsdPerCall: 0.05},
	},
}
