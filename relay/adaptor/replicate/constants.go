package replicate

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	ratio "github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Replicate pricing: https://replicate.com/pricing
func replicateImageConfig(pricePerImage float64) *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: pricePerImage,
		MinImages:        1,
	}
}

var ModelRatios = map[string]adaptor.ModelConfig{
	// -------------------------------------
	// Image Generation Models
	//
	// https://replicate.com/collections/text-to-image
	// -------------------------------------
	"black-forest-labs/flux-kontext-pro":            {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"black-forest-labs/flux-1.1-pro":                {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"black-forest-labs/flux-1.1-pro-ultra":          {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.06)},  // $0.06 per image
	"black-forest-labs/flux-canny-dev":              {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025)}, // $0.025 per image
	"black-forest-labs/flux-canny-pro":              {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05)},  // $0.05 per image
	"black-forest-labs/flux-depth-dev":              {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025)}, // $0.025 per image
	"black-forest-labs/flux-depth-pro":              {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05)},  // $0.05 per image
	"black-forest-labs/flux-dev":                    {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025)}, // $0.025 per image
	"black-forest-labs/flux-dev-lora":               {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.032)}, // $0.032 per image
	"black-forest-labs/flux-fill-dev":               {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"black-forest-labs/flux-fill-pro":               {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05)},  // $0.05 per image
	"black-forest-labs/flux-pro":                    {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"black-forest-labs/flux-redux-dev":              {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025)}, // $0.025 per image
	"black-forest-labs/flux-redux-schnell":          {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.003)}, // $0.003 per image
	"black-forest-labs/flux-schnell":                {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.003)}, // $0.003 per image
	"black-forest-labs/flux-schnell-lora":           {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.02)},  // $0.02 per image
	"bytedance/dreamina-3.1":                        {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03)},  // $0.03 per image
	"bytedance/seedream-3":                          {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03)},  // $0.03 per image
	"bytedance/seedream-4":                          {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03)},  // $0.03 per image
	"bytedance/seedream-4.5":                        {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"google/imagen-4":                               {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"google/imagen-4-fast":                          {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.02)},  // $0.02 per image
	"google/imagen-3":                               {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05)},  // $0.05 per image
	"google/imagen-3-fast":                          {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025)}, // $0.025 per image
	"ideogram-ai/ideogram-v2":                       {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.08)},  // $0.08 per image
	"ideogram-ai/ideogram-v2-turbo":                 {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05)},  // $0.05 per image
	"ideogram-ai/ideogram-v3-turbo":                 {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03)},  // $0.03 per image
	"ideogram-ai/ideogram-v3-balanced":              {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.06)},  // $0.06 per image
	"ideogram-ai/ideogram-v3-quality":               {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.09)},  // $0.09 per image
	"recraft-ai/recraft-v3":                         {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"recraft-ai/recraft-v3-svg":                     {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.08)},  // $0.08 per image
	"stability-ai/stable-diffusion-3":               {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.035)}, // $0.035 per image
	"stability-ai/stable-diffusion-3.5-large":       {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.065)}, // $0.065 per image
	"stability-ai/stable-diffusion-3.5-large-turbo": {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04)},  // $0.04 per image
	"stability-ai/stable-diffusion-3.5-medium":      {Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.035)}, // $0.035 per image

	// -------------------------------------
	// Language Models
	//
	// https://replicate.com/collections/language-models
	// -------------------------------------
	"anthropic/claude-3.5-haiku":                {Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0 / 1.0},       // $1.0/$5.0 per 1M tokens
	"anthropic/claude-3.5-sonnet":               {Ratio: 3.75 * ratio.MilliTokensUsd, CompletionRatio: 18.75 / 3.75},   // $3.75/$18.75 per 1M tokens
	"anthropic/claude-3.7-sonnet":               {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 15.0 / 3.0},      // $3.0/$15.0 per 1M tokens
	"anthropic/claude-4-sonnet":                 {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 15.0 / 3.0},      // $3.0/$15.0 per 1M tokens
	"deepseek-ai/deepseek-r1":                   {Ratio: 3.75 * ratio.MilliTokensUsd, CompletionRatio: 10.0 / 3.75},    // $3.75/$10.0 per 1M tokens
	"deepseek-ai/deepseek-v3":                   {Ratio: 1.45 * ratio.MilliTokensUsd, CompletionRatio: 1.0},            // $1.45/$1.45 per 1M tokens
	"deepseek-ai/deepseek-v3.1":                 {Ratio: 0.672 * ratio.MilliTokensUsd, CompletionRatio: 2.016 / 0.672}, // $0.672/$2.016 per 1M tokens
	"ibm-granite/granite-20b-code-instruct-8k":  {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.1},       // $0.1/$0.5 per 1M tokens
	"ibm-granite/granite-3.0-2b-instruct":       {Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.03},    // $0.03/$0.25 per 1M tokens
	"ibm-granite/granite-3.0-8b-instruct":       {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"ibm-granite/granite-3.1-2b-instruct":       {Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.03},    // $0.03/$0.25 per 1M tokens
	"ibm-granite/granite-3.1-8b-instruct":       {Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.03},    // $0.03/$0.25 per 1M tokens
	"ibm-granite/granite-3.2-8b-instruct":       {Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.03},    // $0.03/$0.25 per 1M tokens
	"ibm-granite/granite-3.3-8b-instruct":       {Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.03},    // $0.03/$0.25 per 1M tokens
	"ibm-granite/granite-4.0-h-small":           {Ratio: 0.06 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.06},    // $0.06/$0.25 per 1M tokens
	"ibm-granite/granite-8b-code-instruct-128k": {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"meta/llama-2-13b":                          {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.1},       // $0.1/$0.5 per 1M tokens
	"meta/llama-2-13b-chat":                     {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.1},       // $0.1/$0.5 per 1M tokens
	"meta/llama-2-70b":                          {Ratio: 0.65 * ratio.MilliTokensUsd, CompletionRatio: 2.75 / 0.65},    // $0.65/$2.75 per 1M tokens
	"meta/llama-2-70b-chat":                     {Ratio: 0.65 * ratio.MilliTokensUsd, CompletionRatio: 2.75 / 0.65},    // $0.65/$2.75 per 1M tokens
	"meta/llama-2-7b":                           {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"meta/llama-2-7b-chat":                      {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"meta/llama-4-maverick-instruct":            {Ratio: 1.2 * ratio.MilliTokensUsd, CompletionRatio: 3.8 / 1.2},       // $1.2/$3.8 per 1M tokens
	"meta/llama-4-scout-instruct":               {Ratio: 0.17 * ratio.MilliTokensUsd, CompletionRatio: 0.65 / 0.17},    // $0.17/$0.65 per 1M tokens
	"meta/meta-llama-3.1-405b-instruct":         {Ratio: 9.5 * ratio.MilliTokensUsd, CompletionRatio: 1.0},             // $9.5 per 1M tokens
	"meta/meta-llama-3-70b":                     {Ratio: 0.65 * ratio.MilliTokensUsd, CompletionRatio: 2.75 / 0.65},    // $0.65/$2.75 per 1M tokens
	"meta/meta-llama-3-70b-instruct":            {Ratio: 0.65 * ratio.MilliTokensUsd, CompletionRatio: 2.75 / 0.65},    // $0.65/$2.75 per 1M tokens
	"meta/meta-llama-3-8b":                      {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"meta/meta-llama-3-8b-instruct":             {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"mistralai/mistral-7b-instruct-v0.2":        {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"mistralai/mistral-7b-v0.1":                 {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.05},    // $0.05/$0.25 per 1M tokens
	"openai/gpt-5":                              {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10.0 / 1.25},    // $1.25/$10.0 per 1M tokens
	"openai/gpt-5-mini":                         {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 0.25},     // $0.25/$2.0 per 1M tokens
	"openai/gpt-5-nano":                         {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.40 / 0.05},    // $0.05/$0.40 per 1M tokens
	"openai/gpt-5-structured":                   {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10.0 / 1.25},    // $1.25/$10.0 per 1M tokens
	"xai/grok-4":                                {Ratio: 7.2 * ratio.MilliTokensUsd, CompletionRatio: 36.0 / 7.2},      // $7.2/$36.0 per 1M tokens

	// -------------------------------------
	// Video Models (TODO: implement the adaptor)
	// -------------------------------------
	// "minimax/video-01": {Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// ReplicateToolingDefaults notes that Replicate bills per model runtime without separate tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://replicate.com/pricing
var ReplicateToolingDefaults = adaptor.ChannelToolConfig{}
