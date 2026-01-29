package cloudflare

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Cloudflare Workers AI pricing - updated 2026-01-28
// Source: https://developers.cloudflare.com/workers-ai/platform/pricing/
var ModelRatios = map[string]adaptor.ModelConfig{
	// Meta Llama Models
	"@cf/meta/llama-3.2-1b-instruct":           {Ratio: 0.027 * ratio.MilliTokensUsd, CompletionRatio: 0.201 / 0.027},
	"@cf/meta/llama-3.2-3b-instruct":           {Ratio: 0.051 * ratio.MilliTokensUsd, CompletionRatio: 0.335 / 0.051},
	"@cf/meta/llama-3.1-8b-instruct-fp8-fast":  {Ratio: 0.045 * ratio.MilliTokensUsd, CompletionRatio: 0.384 / 0.045},
	"@cf/meta/llama-3.2-11b-vision-instruct":   {Ratio: 0.049 * ratio.MilliTokensUsd, CompletionRatio: 0.676 / 0.049},
	"@cf/meta/llama-3.1-70b-instruct-fp8-fast": {Ratio: 0.293 * ratio.MilliTokensUsd, CompletionRatio: 2.253 / 0.293},
	"@cf/meta/llama-3.3-70b-instruct-fp8-fast": {Ratio: 0.293 * ratio.MilliTokensUsd, CompletionRatio: 2.253 / 0.293},
	"@cf/meta/llama-3.1-8b-instruct":           {Ratio: 0.282 * ratio.MilliTokensUsd, CompletionRatio: 0.827 / 0.282},
	"@cf/meta/llama-3.1-8b-instruct-fp8":       {Ratio: 0.152 * ratio.MilliTokensUsd, CompletionRatio: 0.287 / 0.152},
	"@cf/meta/llama-3.1-8b-instruct-awq":       {Ratio: 0.123 * ratio.MilliTokensUsd, CompletionRatio: 0.266 / 0.123},
	"@cf/meta/llama-3-8b-instruct":             {Ratio: 0.282 * ratio.MilliTokensUsd, CompletionRatio: 0.827 / 0.282},
	"@cf/meta/llama-3-8b-instruct-awq":         {Ratio: 0.123 * ratio.MilliTokensUsd, CompletionRatio: 0.266 / 0.123},
	"@cf/meta/llama-2-7b-chat-fp16":            {Ratio: 0.556 * ratio.MilliTokensUsd, CompletionRatio: 6.667 / 0.556},
	"@cf/meta/llama-2-7b-chat-int8":            {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1}, // Legacy
	"@cf/meta/llama-guard-3-8b":                {Ratio: 0.484 * ratio.MilliTokensUsd, CompletionRatio: 0.030 / 0.484},
	"@cf/meta/llama-4-scout-17b-16e-instruct":  {Ratio: 0.270 * ratio.MilliTokensUsd, CompletionRatio: 0.850 / 0.270},

	// Mistral Models
	"@cf/mistral/mistral-7b-instruct-v0.1":         {Ratio: 0.110 * ratio.MilliTokensUsd, CompletionRatio: 0.190 / 0.110},
	"@cf/mistral/mistral-7b-instruct-v0.2-lora":    {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/mistralai/mistral-small-3.1-24b-instruct": {Ratio: 0.351 * ratio.MilliTokensUsd, CompletionRatio: 0.555 / 0.351},

	// DeepSeek Models
	"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b":  {Ratio: 0.497 * ratio.MilliTokensUsd, CompletionRatio: 4.881 / 0.497},
	"@hf/thebloke/deepseek-coder-6.7b-base-awq":     {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/thebloke/deepseek-coder-6.7b-instruct-awq": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/deepseek-ai/deepseek-math-7b-base":         {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/deepseek-ai/deepseek-math-7b-instruct":     {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},

	// Google Models
	"@cf/google/gemma-3-12b-it":   {Ratio: 0.345 * ratio.MilliTokensUsd, CompletionRatio: 0.556 / 0.345},
	"@cf/google/gemma-2b-it-lora": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/google/gemma-7b-it":      {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/google/gemma-7b-it-lora": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},

	// Qwen Models
	"@cf/qwen/qwq-32b":                    {Ratio: 0.660 * ratio.MilliTokensUsd, CompletionRatio: 1.000 / 0.660},
	"@cf/qwen/qwen2.5-coder-32b-instruct": {Ratio: 0.660 * ratio.MilliTokensUsd, CompletionRatio: 1.000 / 0.660},
	"@cf/qwen/qwen3-30b-a3b-fp8":          {Ratio: 0.051 * ratio.MilliTokensUsd, CompletionRatio: 0.335 / 0.051},
	"@cf/qwen/qwen1.5-0.5b-chat":          {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/qwen/qwen1.5-1.8b-chat":          {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/qwen/qwen1.5-14b-chat-awq":       {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/qwen/qwen1.5-7b-chat-awq":        {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},

	// IBM Granite
	"@cf/ibm-granite/granite-4.0-h-micro": {Ratio: 0.017 * ratio.MilliTokensUsd, CompletionRatio: 0.112 / 0.017},

	// OpenAI OSS
	"@cf/openai/gpt-oss-120b": {Ratio: 0.350 * ratio.MilliTokensUsd, CompletionRatio: 0.750 / 0.350},
	"@cf/openai/gpt-oss-20b":  {Ratio: 0.200 * ratio.MilliTokensUsd, CompletionRatio: 1.5},

	// Other Models
	"@cf/aisingapore/gemma-sea-lion-v4-27b-it":   {Ratio: 0.351 * ratio.MilliTokensUsd, CompletionRatio: 0.555 / 0.351},
	"@cf/thebloke/discolm-german-7b-v1-awq":      {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/tiiuae/falcon-7b-instruct":              {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/nousresearch/hermes-2-pro-mistral-7b":   {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/thebloke/llama-2-13b-chat-awq":          {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/thebloke/llamaguard-7b-awq":             {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/thebloke/mistral-7b-instruct-v0.1-awq":  {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/mistralai/mistral-7b-instruct-v0.2":     {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/thebloke/neural-chat-7b-v3-1-awq":       {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/openchat/openchat-3.5-0106":             {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/thebloke/openhermes-2.5-mistral-7b-awq": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/microsoft/phi-2":                        {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},

	// Embedding Models
	"@cf/baai/bge-small-en-v1.5":    {Ratio: 0.020 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/baai/bge-base-en-v1.5":     {Ratio: 0.067 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/baai/bge-large-en-v1.5":    {Ratio: 0.204 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/baai/bge-m3":               {Ratio: 0.012 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/pfnet/plamo-embedding-1b":  {Ratio: 0.019 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/qwen/qwen3-embedding-0.6b": {Ratio: 0.012 * ratio.MilliTokensUsd, CompletionRatio: 1},

	// Audio Models
	"@cf/openai/whisper":                {Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.0005 / 60}},
	"@cf/openai/whisper-large-v3-turbo": {Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.0005 / 60}},
	"@cf/deepgram/nova-3":               {Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.0052 / 60}},
	"@cf/pipecat-ai/smart-turn-v2":      {Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.00033795 / 60}},

	// Specialized Models
	"@cf/defog/sqlcoder-7b-2":                {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/nexusflow/starling-lm-7b-beta":      {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/tinyllama/tinyllama-1.1b-chat-v1.0": {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@hf/thebloke/zephyr-7b-beta-awq":        {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},

	// Other (Translation, Reranker, etc.)
	"@cf/huggingface/distilbert-sst-2-int8": {Ratio: 0.026 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/baai/bge-reranker-base":            {Ratio: 0.003 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/meta/m2m100-1.2b":                  {Ratio: 0.342 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"@cf/ai4bharat/indictrans2-en-indic-1B": {Ratio: 0.342 * ratio.MilliTokensUsd, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// CloudflareToolingDefaults notes Workers AI publishes only neuron-based model pricing (no server-side tool billing as of 2025-11-12).
// Source: https://r.jina.ai/https://developers.cloudflare.com/workers-ai/platform/pricing/
var CloudflareToolingDefaults = adaptor.ChannelToolConfig{}
