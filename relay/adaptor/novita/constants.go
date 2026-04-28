package novita

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Novita pricing: https://novita.ai/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Novita Models - Based on https://novita.ai/pricing
	"meta-llama/llama-3.1-8b-instruct":            {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/llama-3.1-70b-instruct":           {Ratio: 0.9 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/llama-3.1-405b-instruct":          {Ratio: 5.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/llama-3-8b-instruct":              {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/llama-3-70b-instruct":             {Ratio: 0.9 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"mistralai/mistral-7b-instruct":               {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"mistralai/mixtral-8x7b-instruct":             {Ratio: 0.6 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"mistralai/mixtral-8x22b-instruct":            {Ratio: 1.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"qwen/qwen-2-72b-instruct":                    {Ratio: 0.9 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"qwen/qwen-2-7b-instruct":                     {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"deepseek-ai/deepseek-coder-33b-instruct":     {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"01-ai/yi-34b-chat":                           {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"01-ai/yi-6b-chat":                            {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"google/gemma-2-9b-it":                        {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"google/gemma-2-27b-it":                       {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"nousresearch/hermes-2-pro-llama-3-8b":        {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"nousresearch/nous-hermes-llama2-13b":         {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"cognitivecomputations/dolphin-mixtral-8x22b": {Ratio: 1.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"sao10k/l3-70b-euryale-v2.1":                  {Ratio: 0.9 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"sophosympatheia/midnight-rose-70b":           {Ratio: 0.9 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"gryphe/mythomax-l2-13b":                      {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"Nous-Hermes-2-Mixtral-8x7B-DPO":              {Ratio: 0.6 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"lzlv_70b":                                    {Ratio: 0.9 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"teknium/openhermes-2.5-mistral-7b":           {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"microsoft/wizardlm-2-8x22b":                  {Ratio: 1.2 * ratio.MilliTokensUsd, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// NovitaToolingDefaults notes that Novita's public pricing focuses on model tokens; no tool metering is published (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://novita.ai/pricing
var NovitaToolingDefaults = adaptor.ChannelToolConfig{}
