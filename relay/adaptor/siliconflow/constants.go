package siliconflow

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on SiliconFlow pricing: https://docs.siliconflow.cn/docs/getting-started
var ModelRatios = map[string]adaptor.ModelConfig{
	// SiliconFlow Models - Based on https://siliconflow.cn/pricing
	"deepseek-chat":                           {Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"deepseek-coder":                          {Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"Qwen/Qwen2-72B-Instruct":                 {Ratio: 0.56 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"Qwen/Qwen2-7B-Instruct":                  {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"Qwen/Qwen2-1.5B-Instruct":                {Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"Qwen/Qwen2-0.5B-Instruct":                {Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/Meta-Llama-3-8B-Instruct":     {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/Meta-Llama-3-70B-Instruct":    {Ratio: 0.56 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/Meta-Llama-3.1-8B-Instruct":   {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/Meta-Llama-3.1-70B-Instruct":  {Ratio: 0.56 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"meta-llama/Meta-Llama-3.1-405B-Instruct": {Ratio: 2.8 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"mistralai/Mistral-7B-Instruct-v0.2":      {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"mistralai/Mixtral-8x7B-Instruct-v0.1":    {Ratio: 0.56 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"01-ai/Yi-1.5-9B-Chat-16K":                {Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"01-ai/Yi-1.5-6B-Chat":                    {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"THUDM/glm-4-9b-chat":                     {Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"THUDM/chatglm3-6b":                       {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"internlm/internlm2_5-7b-chat":            {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"google/gemma-2-9b-it":                    {Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"google/gemma-2-27b-it":                   {Ratio: 0.28 * ratio.MilliTokensUsd, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// SiliconFlowToolingDefaults notes that SiliconFlow public docs focus on model usage; no separate tool fees are published (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://docs.siliconflow.com/en/api-reference/chat-completions/chat-completions
var SiliconFlowToolingDefaults = adaptor.ChannelToolConfig{}
