package togetherai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

const togetherAIImageBaseSize = "1024x1024"

const togetherAIImageBaseMultiplier = 1.048576

// togetherAIImagePerMegapixelConfig converts Together AI's published per-megapixel pricing
// into the image billing metadata used by one-api.
func togetherAIImagePerMegapixelConfig(pricePerMegapixel float64) *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: pricePerMegapixel * togetherAIImageBaseMultiplier,
		DefaultSize:      togetherAIImageBaseSize,
		MinImages:        1,
		SizeMultipliers: map[string]float64{
			"512x512":   0.25,
			"1024x1024": 1,
			"1024x1536": 1.5,
			"1536x1024": 1.5,
			"1920x1080": 1.9775390625,
			"2048x2048": 4,
			"3840x2160": 7.91015625,
			"4096x4096": 16,
		},
	}
}

// togetherAIGeminiProImageConfig returns the published fixed-per-image pricing metadata for
// Together AI's Gemini Pro image generation offering.
func togetherAIGeminiProImageConfig() *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: 0.134,
		DefaultSize:      "1920x1080",
		MinImages:        1,
		SizeMultipliers: map[string]float64{
			"1920x1080": 1,
			"2048x2048": 1,
			"3840x2160": 0.24 / 0.134,
		},
	}
}

// ModelRatios contains Together AI models with published pricing metadata from the public
// serverless models catalog.
// Source: https://docs.together.ai/docs/serverless-models (retrieved 2026-04-28)
var ModelRatios = map[string]adaptor.ModelConfig{
	// Chat and vision-capable LLMs.
	"MiniMaxAI/MiniMax-M2.7":                   {Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30, CachedInputRatio: 0.06 * ratio.MilliTokensUsd},
	"MiniMaxAI/MiniMax-M2.5":                   {Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30, CachedInputRatio: 0.06 * ratio.MilliTokensUsd},
	"Qwen/Qwen3.5-397B-A17B":                   {Ratio: 0.60 * ratio.MilliTokensUsd, CompletionRatio: 3.60 / 0.60},
	"Qwen/Qwen3.5-9B":                          {Ratio: 0.10 * ratio.MilliTokensUsd, CompletionRatio: 0.15 / 0.10},
	"moonshotai/Kimi-K2.6":                     {Ratio: 1.20 * ratio.MilliTokensUsd, CompletionRatio: 4.50 / 1.20, CachedInputRatio: 0.20 * ratio.MilliTokensUsd},
	"moonshotai/Kimi-K2.5":                     {Ratio: 0.50 * ratio.MilliTokensUsd, CompletionRatio: 2.80 / 0.50},
	"zai-org/GLM-5.1":                          {Ratio: 1.40 * ratio.MilliTokensUsd, CompletionRatio: 4.40 / 1.40},
	"zai-org/GLM-5":                            {Ratio: 1.00 * ratio.MilliTokensUsd, CompletionRatio: 3.20 / 1.00},
	"openai/gpt-oss-120b":                      {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 0.60 / 0.15},
	"openai/gpt-oss-20b":                       {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.20 / 0.05},
	"deepseek-ai/DeepSeek-V4-Pro":              {Ratio: 2.10 * ratio.MilliTokensUsd, CompletionRatio: 4.40 / 2.10, CachedInputRatio: 0.20 * ratio.MilliTokensUsd},
	"deepseek-ai/DeepSeek-V3.1":                {Ratio: 0.60 * ratio.MilliTokensUsd, CompletionRatio: 1.70 / 0.60},
	"Qwen/Qwen3-Coder-Next-FP8":                {Ratio: 0.50 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.50},
	"Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8":  {Ratio: 2.00 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"Qwen/Qwen3-235B-A22B-Instruct-2507-tput":  {Ratio: 0.20 * ratio.MilliTokensUsd, CompletionRatio: 0.60 / 0.20},
	"deepseek-ai/DeepSeek-R1":                  {Ratio: 3.00 * ratio.MilliTokensUsd, CompletionRatio: 7.00 / 3.00},
	"meta-llama/Llama-3.3-70B-Instruct-Turbo":  {Ratio: 0.88 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"deepcogito/cogito-v2-1-671b":              {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"essentialai/rnj-1-instruct":               {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"Qwen/Qwen2.5-7B-Instruct-Turbo":           {Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"google/gemma-4-31B-it":                    {Ratio: 0.20 * ratio.MilliTokensUsd, CompletionRatio: 0.50 / 0.20},
	"google/gemma-3n-E4B-it":                   {Ratio: 0.06 * ratio.MilliTokensUsd, CompletionRatio: 0.12 / 0.06},
	"LiquidAI/LFM2-24B-A2B":                    {Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.12 / 0.03},
	"meta-llama/Meta-Llama-3-8B-Instruct-Lite": {Ratio: 0.10 * ratio.MilliTokensUsd, CompletionRatio: 1},

	// Embeddings.
	"intfloat/multilingual-e5-large-instruct": {
		Ratio:           0.02 * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.02 * ratio.MilliTokensUsd,
		},
	},

	// Image generation models with published pricing.
	"google/imagen-4.0-preview":                {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.04)},
	"google/imagen-4.0-fast":                   {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.02)},
	"google/imagen-4.0-ultra":                  {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.06)},
	"google/flash-image-2.5":                   {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.039)},
	"google/gemini-3-pro-image":                {Ratio: 0, CompletionRatio: 1, Image: togetherAIGeminiProImageConfig()},
	"black-forest-labs/FLUX.1-schnell":         {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0027)},
	"black-forest-labs/FLUX.1.1-pro":           {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.04)},
	"black-forest-labs/FLUX.1-kontext-pro":     {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.04)},
	"black-forest-labs/FLUX.1-kontext-max":     {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.08)},
	"black-forest-labs/FLUX.1-krea-dev":        {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.025)},
	"ByteDance-Seed/Seedream-3.0":              {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.018)},
	"ByteDance-Seed/Seedream-4.0":              {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.03)},
	"Qwen/Qwen-Image":                          {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.005)},
	"RunDiffusion/Juggernaut-pro-flux":         {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.004)},
	"HiDream-ai/HiDream-I1-Full":               {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.009)},
	"HiDream-ai/HiDream-I1-Dev":                {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0045)},
	"HiDream-ai/HiDream-I1-Fast":               {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0032)},
	"ideogram/ideogram-3.0":                    {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.06)},
	"Lykon/DreamShaper":                        {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0006)},
	"stabilityai/stable-diffusion-3-medium":    {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0019)},
	"stabilityai/stable-diffusion-xl-base-1.0": {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0019)},

	// Audio models with published pricing.
	"canopylabs/orpheus-3b-0.1-ft": {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"hexgrad/Kokoro-82M":           {Ratio: 4.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"cartesia/sonic-3":             {Ratio: 65.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"cartesia/sonic-2":             {Ratio: 65.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"cartesia/sonic":               {Ratio: 65.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"openai/whisper-large-v3":      {Ratio: 0, CompletionRatio: 1, Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.0015 / 60}},
	"nvidia/parakeet-tdt-0.6b-v3":  {Ratio: 0, CompletionRatio: 1, Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.0015 / 60}},
}

// ModelList captures the current public Together AI serverless catalog used by the OpenAI-compatible adapter.
// Pricing is currently published for only a subset of these models, so ModelRatios intentionally stays smaller.
// Source: https://docs.together.ai/docs/serverless-models (retrieved 2026-04-28)
var ModelList = []string{
	"MiniMaxAI/MiniMax-M2.7",
	"MiniMaxAI/MiniMax-M2.5",
	"Qwen/Qwen3.5-397B-A17B",
	"Qwen/Qwen3.5-9B",
	"moonshotai/Kimi-K2.6",
	"moonshotai/Kimi-K2.5",
	"zai-org/GLM-5.1",
	"zai-org/GLM-5",
	"openai/gpt-oss-120b",
	"openai/gpt-oss-20b",
	"deepseek-ai/DeepSeek-V4-Pro",
	"deepseek-ai/DeepSeek-V3.1",
	"Qwen/Qwen3-Coder-Next-FP8",
	"Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8",
	"Qwen/Qwen3-235B-A22B-Instruct-2507-tput",
	"deepseek-ai/DeepSeek-R1",
	"meta-llama/Llama-3.3-70B-Instruct-Turbo",
	"deepcogito/cogito-v2-1-671b",
	"essentialai/rnj-1-instruct",
	"Qwen/Qwen2.5-7B-Instruct-Turbo",
	"google/gemma-4-31B-it",
	"google/gemma-3n-E4B-it",
	"LiquidAI/LFM2-24B-A2B",
	"meta-llama/Meta-Llama-3-8B-Instruct-Lite",
	"google/imagen-4.0-preview",
	"google/imagen-4.0-fast",
	"google/imagen-4.0-ultra",
	"google/flash-image-2.5",
	"google/gemini-3-pro-image",
	"black-forest-labs/FLUX.1-schnell",
	"black-forest-labs/FLUX.1.1-pro",
	"black-forest-labs/FLUX.1-kontext-pro",
	"black-forest-labs/FLUX.1-kontext-max",
	"black-forest-labs/FLUX.1-krea-dev",
	"black-forest-labs/FLUX.2-pro",
	"black-forest-labs/FLUX.2-dev",
	"black-forest-labs/FLUX.2-flex",
	"black-forest-labs/FLUX.2-max",
	"ByteDance-Seed/Seedream-3.0",
	"ByteDance-Seed/Seedream-4.0",
	"Qwen/Qwen-Image",
	"google/flash-image-3.1",
	"openai/gpt-image-1.5",
	"Qwen/Qwen-Image-2.0",
	"Qwen/Qwen-Image-2.0-Pro",
	"Wan-AI/Wan2.6-image",
	"RunDiffusion/Juggernaut-pro-flux",
	"HiDream-ai/HiDream-I1-Full",
	"HiDream-ai/HiDream-I1-Dev",
	"HiDream-ai/HiDream-I1-Fast",
	"ideogram/ideogram-3.0",
	"Lykon/DreamShaper",
	"stabilityai/stable-diffusion-3-medium",
	"stabilityai/stable-diffusion-xl-base-1.0",
	"canopylabs/orpheus-3b-0.1-ft",
	"hexgrad/Kokoro-82M",
	"cartesia/sonic-3",
	"cartesia/sonic-2",
	"cartesia/sonic",
	"openai/whisper-large-v3",
	"nvidia/parakeet-tdt-0.6b-v3",
	"intfloat/multilingual-e5-large-instruct",
}

// TogetherAIToolingDefaults notes that Together AI publishes model pricing, but no server-side tool
// invocation fees were documented in the public serverless catalog or compatibility docs as of 2026-04-22.
// Sources: https://docs.together.ai/docs/serverless-models, https://docs.together.ai/docs/openai-api-compatibility
var TogetherAIToolingDefaults = adaptor.ChannelToolConfig{}
