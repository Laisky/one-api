package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Alibaba Bailian pricing: https://help.aliyun.com/zh/model-studio/getting-started/models
var ModelRatios = map[string]adaptor.ModelConfig{
	// Qwen Models
	"qwen-turbo":              {Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-plus":               {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-long":               {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-max":                {Ratio: 20.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-coder-plus":         {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-coder-plus-latest":  {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-coder-turbo":        {Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-coder-turbo-latest": {Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-mt-plus":            {Ratio: 0.8 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-mt-turbo":           {Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwq-32b-preview":         {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// DeepSeek Models (hosted on Alibaba)
	"deepseek-r1": {Ratio: 1.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"deepseek-v3": {Ratio: 0.07 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// AlibailianToolingDefaults reflects that Bailian's public docs do not disclose per-tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.alibabacloud.com/help/en/model-studio/latest/billing (returns 404 for unauthenticated access)
var AlibailianToolingDefaults = adaptor.ChannelToolConfig{}
