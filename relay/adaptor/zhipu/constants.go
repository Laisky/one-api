package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Zhipu pricing: https://open.bigmodel.cn/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// =====================================================================
	// Flagship Models - Text (tiered pricing)
	// =====================================================================

	// GLM-5-Turbo: input [0,32K) ¥5/¥22, input [32K+) ¥7/¥26
	"glm-5-turbo": {
		Ratio:            5 * ratio.MilliTokensRmb,
		CompletionRatio:  22.0 / 5.0,
		CachedInputRatio: 1.2 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 7 * ratio.MilliTokensRmb, CompletionRatio: 26.0 / 7.0, CachedInputRatio: 1.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-5: input [0,32K) ¥4/¥18, input [32K+) ¥6/¥22
	"glm-5": {
		Ratio:            4 * ratio.MilliTokensRmb,
		CompletionRatio:  18.0 / 4.0,
		CachedInputRatio: 1 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 6 * ratio.MilliTokensRmb, CompletionRatio: 22.0 / 6.0, CachedInputRatio: 1.5 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.7: input [0,32K) output [0,0.2M) ¥2/¥8; output [0.2M+) ¥3/¥14; input [32K,200K) ¥4/¥16
	"glm-4.7": {
		Ratio:            2 * ratio.MilliTokensRmb,
		CompletionRatio:  8.0 / 2.0,
		CachedInputRatio: 0.4 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 3 * ratio.MilliTokensRmb, CompletionRatio: 14.0 / 3.0, CachedInputRatio: 0.6 * ratio.MilliTokensRmb, InputTokenThreshold: 0},
			{Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 4.0, CachedInputRatio: 0.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.7-FlashX: ¥0.5/¥3
	"glm-4.7-flashx": {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 3.0 / 0.5, CachedInputRatio: 0.1 * ratio.MilliTokensRmb},
	// GLM-4.7-Flash (free)
	"glm-4.7-flash": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0},
	// GLM-4.6: same tiered pricing as GLM-4.7
	"glm-4.6": {
		Ratio:            2 * ratio.MilliTokensRmb,
		CompletionRatio:  8.0 / 2.0,
		CachedInputRatio: 0.4 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 3 * ratio.MilliTokensRmb, CompletionRatio: 14.0 / 3.0, CachedInputRatio: 0.6 * ratio.MilliTokensRmb, InputTokenThreshold: 0},
			{Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 4.0, CachedInputRatio: 0.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.5-Air: input [0,32K) output [0,0.2M) ¥0.8/¥2; output [0.2M+) ¥0.8/¥6; input [32K,128K) ¥1.2/¥8
	"glm-4.5-air": {
		Ratio:            0.8 * ratio.MilliTokensRmb,
		CompletionRatio:  2.0 / 0.8,
		CachedInputRatio: 0.16 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 6.0 / 0.8, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 0},
			{Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 8.0 / 1.2, CachedInputRatio: 0.24 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.5 (tiered, same structure as older pricing)
	"glm-4.5": {
		Ratio:            2 * ratio.MilliTokensRmb,
		CompletionRatio:  8.0 / 2.0,
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
	// GLM-4.5-AirX (tiered)
	"glm-4.5-airx": {
		Ratio:            4 * ratio.MilliTokensRmb,
		CompletionRatio:  16.0 / 4.0,
		CachedInputRatio: 0.8 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 8 * ratio.MilliTokensRmb, CompletionRatio: 32.0 / 8.0, CachedInputRatio: 1.6 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},

	// =====================================================================
	// Flagship Models - Visual (tiered pricing)
	// =====================================================================

	// GLM-5V-Turbo: input [0,32K) ¥5/¥22, input [32K+) ¥7/¥26 (same as GLM-5-Turbo)
	"glm-5v-turbo": {
		Ratio:            5 * ratio.MilliTokensRmb,
		CompletionRatio:  22.0 / 5.0,
		CachedInputRatio: 1.2 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 7 * ratio.MilliTokensRmb, CompletionRatio: 26.0 / 7.0, CachedInputRatio: 1.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.6V: input [0,32K) ¥1/¥3, input [32K,128K) ¥2/¥6
	"glm-4.6v": {
		Ratio:            1 * ratio.MilliTokensRmb,
		CompletionRatio:  3.0 / 1.0,
		CachedInputRatio: 0.2 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 2 * ratio.MilliTokensRmb, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.4 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.6V-FlashX: input [0,32K) ¥0.15/¥1.5, input [32K,128K) ¥0.3/¥3
	"glm-4.6v-flashx": {
		Ratio:            0.15 * ratio.MilliTokensRmb,
		CompletionRatio:  1.5 / 0.15,
		CachedInputRatio: 0.03 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 3.0 / 0.3, CachedInputRatio: 0.03 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.5V: input [0,32K) ¥2/¥6, input [32K,64K) ¥4/¥12
	"glm-4.5v": {
		Ratio:            2 * ratio.MilliTokensRmb,
		CompletionRatio:  6.0 / 2.0,
		CachedInputRatio: 0.4 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 12.0 / 4.0, CachedInputRatio: 0.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
		},
	},
	// GLM-4.6V-Flash (free)
	"glm-4.6v-flash": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0},
	// GLM-4V-Flash (free)
	"glm-4v-flash": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0},

	// =====================================================================
	// Language Models (flat pricing per 1M tokens)
	// =====================================================================

	// GLM-4-Plus: ¥5/M tokens
	"glm-4-plus": {Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4-Air: ¥0.5/M tokens
	"glm-4-air": {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4-AirX: ¥10/M tokens
	"glm-4-airx": {Ratio: 10 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4-FlashX-250414: ¥0.1/M tokens
	"glm-4-flashx-250414": {Ratio: 0.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4-Long: ¥1/M tokens
	"glm-4-long": {Ratio: 1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4-Assistant: ¥5/M tokens
	"glm-4-assistant": {Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4-Flash-250414 (free)
	"glm-4-flash-250414": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0},
	// GLM-4.5-Flash (free, 即将下线)
	"glm-4.5-flash": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0},
	// GLM-4-Flash (free, older)
	"glm-4-flash": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0},

	// =====================================================================
	// Reasoning Models
	// =====================================================================

	// GLM-Z1-Air: ¥0.5/M tokens
	"glm-z1-air": {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-Z1-AirX: ¥5/M tokens
	"glm-z1-airx": {Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-Z1-FlashX: ¥0.1/M tokens
	"glm-z1-flashx": {Ratio: 0.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4.1V-Thinking-FlashX: ¥2/M tokens
	"glm-4.1v-thinking-flashx": {Ratio: 2 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4.1V-Thinking-Flash (free)
	"glm-4.1v-thinking-flash": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0},

	// =====================================================================
	// Multimodal Models
	// =====================================================================

	// GLM-4V-Plus-0111: ¥4/M tokens
	"glm-4v-plus-0111": {Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4V-Plus: ¥4/M tokens (older)
	"glm-4v-plus": {Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4V: ¥50/M tokens (older)
	"glm-4v": {Ratio: 50 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// GLM-4-Voice: ¥80/M tokens
	"glm-4-voice": {Ratio: 80 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// =====================================================================
	// Image Generation Models (per-request pricing approximations)
	// =====================================================================

	// CogView-4: ¥0.06/request
	"cogview-4":       {Ratio: 0.06 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogview-3-plus":  {Ratio: 0.08 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogview-3":       {Ratio: 0.04 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogview-3-flash": {Ratio: 0, CompletionRatio: 1, CachedInputRatio: 0}, // free
	"cogviewx":        {Ratio: 0.04 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"cogviewx-flash":  {Ratio: 0.008 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// =====================================================================
	// Character, Code, and Utility Models
	// =====================================================================

	// CharGLM-4: ¥1/M tokens
	"charglm-4": {Ratio: 1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// Emohaa: ¥15/M tokens
	"emohaa": {Ratio: 15 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// CodeGeeX-4: ¥0.1/M tokens
	"codegeex-4": {Ratio: 0.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// Rerank: ¥0.8/M tokens
	"rerank": {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// =====================================================================
	// Embedding Models
	// =====================================================================

	// Embedding-3: ¥0.5/M tokens
	"embedding-3": {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// Embedding-2: ¥0.5/M tokens
	"embedding-2": {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// =====================================================================
	// OCR Models
	// =====================================================================

	// GLM-OCR: ¥0.2/M tokens (input and output identical pricing)
	"glm-ocr": {Ratio: 0.2 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// =====================================================================
	// Legacy Models
	// =====================================================================

	"glm-3-turbo":      {Ratio: 0.005 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"glm-zero-preview": {Ratio: 0.7 * ratio.MilliTokensRmb, CompletionRatio: 1},
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
