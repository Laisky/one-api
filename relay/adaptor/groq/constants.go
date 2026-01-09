package groq

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Groq pricing: https://groq.com/pricing/
var ModelRatios = map[string]adaptor.ModelConfig{
	// Regular Models
	"llama-3.3-70b-versatile":      {Ratio: 0.59 * ratio.MilliTokensUsd, CompletionRatio: 0.79 / 0.59},
	"llama-3.1-8b-instant":         {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.08 / 0.05},
	"meta-llama/llama-guard-4-12b": {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"whisper-large-v3":             {Ratio: 0.111 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"whisper-large-v3-turbo":       {Ratio: 0.04 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"openai/gpt-oss-120b":          {Ratio: 0.15 * ratio.MilliTokensUsd, CachedInputRatio: 0.075 * ratio.MilliTokensUsd, CompletionRatio: 0.60 / 0.15},
	"openai/gpt-oss-20b":           {Ratio: 0.075 * ratio.MilliTokensUsd, CachedInputRatio: 0.0375 * ratio.MilliTokensUsd, CompletionRatio: 0.30 / 0.075},

	// Preview Models
	"meta-llama/llama-4-maverick-17b-128e-instruct": {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 0.6 / 0.2},
	"meta-llama/llama-4-scout-17b-16e-instruct":     {Ratio: 0.11 * ratio.MilliTokensUsd, CompletionRatio: 0.34 / 0.11},
	"meta-llama/llama-prompt-guard-2-22m":           {Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/llama-prompt-guard-2-86m":           {Ratio: 0.04 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"moonshotai/kimi-k2-instruct-0905":              {Ratio: 1 * ratio.MilliTokensUsd, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3},
	"openai/gpt-oss-safeguard-20b":                  {Ratio: 0.075 * ratio.MilliTokensUsd, CompletionRatio: 0.30 / 0.075},
	"qwen/qwen3-32b":                                {Ratio: 0.29 * ratio.MilliTokensUsd, CompletionRatio: 0.59 / 0.29},

	// New Models (Jan 2026)
	"canopylabs/orpheus-arabic-saudi": {Ratio: 40.0 * ratio.MilliTokensUsd, CompletionRatio: 1}, // per 1M characters
	"canopylabs/orpheus-v1-english":   {Ratio: 22.0 * ratio.MilliTokensUsd, CompletionRatio: 1}, // per 1M characters
	"groq/compound":                   {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 0.60 / 0.15},
	"groq/compound-mini":              {Ratio: 0.11 * ratio.MilliTokensUsd, CompletionRatio: 0.34 / 0.11},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// GroqToolingDefaults enumerates Groq's Compound and GPT-OSS built-in tools and pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://groq.com/pricing
var GroqToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"basic_search":          {UsdPerCall: 0.005},
		"advanced_search":       {UsdPerCall: 0.008},
		"visit_website":         {UsdPerCall: 0.001},
		"code_execution":        {UsdPerCall: 0.18},
		"browser_automation":    {UsdPerCall: 0.08},
		"browser_search_basic":  {UsdPerCall: 0.005},
		"browser_search_visit":  {UsdPerCall: 0.001},
		"code_execution_python": {UsdPerCall: 0.18},
	},
}
