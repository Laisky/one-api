package baiduv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9do2
var ModelRatios = map[string]adaptor.ModelConfig{
	// ERNIE 4.0 Models
	"ernie-4.0-8k-latest":        {Ratio: 0.12 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.12 / 1k tokens
	"ernie-4.0-8k-preview":       {Ratio: 0.12 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.12 / 1k tokens
	"ernie-4.0-8k":               {Ratio: 0.12 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.12 / 1k tokens
	"ernie-4.0-turbo-8k-latest":  {Ratio: 0.02 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-8k-preview": {Ratio: 0.02 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-8k":         {Ratio: 0.02 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-128k":       {Ratio: 0.02 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.02 / 1k tokens

	// ERNIE 3.5 Models
	"ernie-3.5-8k-preview": {Ratio: 0.012 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.012 / 1k tokens
	"ernie-3.5-8k":         {Ratio: 0.012 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.012 / 1k tokens
	"ernie-3.5-128k":       {Ratio: 0.012 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.012 / 1k tokens

	// ERNIE Speed Models
	"ernie-speed-8k":       {Ratio: 0.004 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.004 / 1k tokens
	"ernie-speed-128k":     {Ratio: 0.004 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.004 / 1k tokens
	"ernie-speed-pro-128k": {Ratio: 0.004 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.004 / 1k tokens

	// ERNIE Lite Models
	"ernie-lite-8k":       {Ratio: 0.008 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.008 / 1k tokens
	"ernie-lite-pro-128k": {Ratio: 0.008 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.008 / 1k tokens

	// ERNIE Tiny Models
	"ernie-tiny-8k": {Ratio: 0.004 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.004 / 1k tokens

	// ERNIE Character Models
	"ernie-char-8k":         {Ratio: 0.04 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.04 / 1k tokens
	"ernie-char-fiction-8k": {Ratio: 0.04 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.04 / 1k tokens
	"ernie-novel-8k":        {Ratio: 0.04 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.04 / 1k tokens

	// DeepSeek Models (hosted on Baidu)
	"deepseek-v3":                  {Ratio: 0.01 * ratio.MilliTokensRmb, CompletionRatio: 2},  // CNY 0.01 / 1k tokens
	"deepseek-r1":                  {Ratio: 0.01 * ratio.MilliTokensRmb, CompletionRatio: 8},  // CNY 0.01 / 1k tokens
	"deepseek-r1-distill-qwen-32b": {Ratio: 0.004 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.004 / 1k tokens
	"deepseek-r1-distill-qwen-14b": {Ratio: 0.003 * ratio.MilliTokensRmb, CompletionRatio: 1}, // CNY 0.003 / 1k tokens
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// BaiduV2ToolingDefaults records that the updated Wenxin billing docs require authentication for tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9do2 (restricted access)
var BaiduV2ToolingDefaults = adaptor.ChannelToolConfig{}
