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

// Shared metadata helpers for Together AI chat/LLM models.
// All Together AI hosted models are closed-weight (quantization is in-house) or open-weight
// where quantization comes from Together AI's serving layer.
// Sources: https://docs.together.ai/docs/serverless-models, HuggingFace model cards.
var (
	togetherTextIn         = []string{"text"}
	togetherVisionIn       = []string{"text", "image"}
	togetherTextOut        = []string{"text"}
	togetherChatFeatures   = []string{"tools", "json_mode"}
	togetherReasonFeatures = []string{"tools", "json_mode", "reasoning"}
	togetherChatSampling   = []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"}
	togetherR1Sampling     = []string{"temperature", "top_p", "stop", "seed", "max_tokens"}
)

// ModelRatios contains Together AI models with published pricing metadata from the public
// serverless models catalog.
// Source: https://docs.together.ai/docs/serverless-models (retrieved 2026-06-13)
var ModelRatios = map[string]adaptor.ModelConfig{
	// Chat and vision-capable LLMs.
	"MiniMaxAI/MiniMax-M2.7": {
		Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30, CachedInputRatio: 0.06 * ratio.MilliTokensUsd,
		ContextLength: 202752, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "MiniMaxAI/MiniMax-M2",
		Description: "MiniMax M2 interleaved-thinking MoE (230B/10B active) for coding and agentic workflows.",
	},
	"MiniMaxAI/MiniMax-M2.5": {
		Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30, CachedInputRatio: 0.06 * ratio.MilliTokensUsd,
		ContextLength: 202752, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "MiniMaxAI/MiniMax-M2",
		Description: "Earlier checkpoint of MiniMax M2 interleaved-thinking MoE.",
	},
	"MiniMaxAI/MiniMax-M3": {
		Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30, CachedInputRatio: 0.06 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 524288,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "MiniMaxAI/MiniMax-M3",
		Description: "MiniMax M3 native multimodal (text+image+video) frontier model, 1M context, 512K max output, $0.30/$1.20 per 1M.",
	},
	// Qwen3.6-Plus released 2026-04-01; serverless price $0.50/$3.00 with hybrid linear-attention + MoE
	// routing and 1M context. Multimodal (text+image+video input) with thinking mode.
	// Source: https://www.together.ai/models/qwen36-plus
	"Qwen/Qwen3.6-Plus": {
		Ratio: 0.50 * ratio.MilliTokensUsd, CompletionRatio: 3.00 / 0.50,
		ContextLength: 1000000, MaxOutputTokens: 32768,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "Qwen/Qwen3.6-Plus",
		Description: "Qwen 3.6 Plus multimodal agentic MoE with hybrid linear-attention routing, 1M context, and thinking mode.",
	},
	"Qwen/Qwen3.7-Max": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 3.75 / 1.25, CachedInputRatio: 0.13 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 65536,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "Qwen/Qwen3.7-Max",
		Description: "Alibaba Qwen3.7 Max reasoning flagship, 1M context, $1.25/$3.75 per 1M.",
	},
	"Qwen/Qwen3.5-397B-A17B": {
		Ratio: 0.60 * ratio.MilliTokensUsd, CompletionRatio: 3.60 / 0.60, CachedInputRatio: 0.35 * ratio.MilliTokensUsd,
		ContextLength: 262144, MaxOutputTokens: 8192,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "Qwen/Qwen3.5-397B-A17B",
		Description: "Qwen 3.5 397B vision-language MoE with 262K context and strong multilingual coding.",
	},
	"Qwen/Qwen3.5-9B": {
		Ratio: 0.17 * ratio.MilliTokensUsd, CompletionRatio: 0.25 / 0.17,
		ContextLength: 262144, MaxOutputTokens: 8192,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "Qwen/Qwen3.5-9B",
		Description: "Compact Qwen 3.5 9B vision-language model with 262K context.",
	},
	"moonshotai/Kimi-K2.6": {
		Ratio: 1.20 * ratio.MilliTokensUsd, CompletionRatio: 4.50 / 1.20, CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
		ContextLength: 262144, MaxOutputTokens: 8192,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "moonshotai/Kimi-K2-Instruct",
		Description: "Kimi K2.6 agentic MoE (1T/32B active) with cached input pricing, optimised for tool use and SWE.",
	},
	"moonshotai/Kimi-K2.5": {
		Ratio: 0.50 * ratio.MilliTokensUsd, CompletionRatio: 2.80 / 0.50,
		ContextLength: 262144, MaxOutputTokens: 8192,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "moonshotai/Kimi-K2-Instruct",
		Description: "Kimi K2.5 agentic MoE (1T/32B active) with vision support and 262K context. (RETIRED 2026-05-21; use moonshotai/Kimi-K2.6)",
	},
	"zai-org/GLM-5.1": {
		Ratio: 1.40 * ratio.MilliTokensUsd, CompletionRatio: 4.40 / 1.40, CachedInputRatio: 0.26 * ratio.MilliTokensUsd,
		ContextLength: 202752, MaxOutputTokens: 131072,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "zai-org/GLM-5.1",
		Description: "Z.ai GLM-5.1 hybrid-reasoning MoE (754B/40B active) with strong agentic performance.",
	},
	"zai-org/GLM-5": {
		Ratio: 1.00 * ratio.MilliTokensUsd, CompletionRatio: 3.20 / 1.00,
		ContextLength: 202752, MaxOutputTokens: 131072,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "zai-org/GLM-5",
		Description: "Z.ai GLM-5 hybrid-reasoning MoE (754B/40B active), state-of-the-art open-source agentic model.",
	},
	"openai/gpt-oss-120b": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 0.60 / 0.15,
		ContextLength: 128000, MaxOutputTokens: 16384,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		// Together's OpenAI-compatibility doc states reasoning_effort works on GPT-OSS
		// models with values "low", "medium", "high".
		// Source: https://docs.together.ai/docs/openai-api-compatibility
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		Quantization:              "fp4", HuggingFaceID: "openai/gpt-oss-120b",
		Description: "OpenAI gpt-oss-120b dense open-weight model with 128K context.",
	},
	"openai/gpt-oss-20b": {
		Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.20 / 0.05,
		ContextLength: 128000, MaxOutputTokens: 16384,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		// Together's OpenAI-compatibility doc states reasoning_effort works on GPT-OSS
		// models with values "low", "medium", "high".
		// Source: https://docs.together.ai/docs/openai-api-compatibility
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		Quantization:              "fp4", HuggingFaceID: "openai/gpt-oss-20b",
		Description: "OpenAI gpt-oss-20b compact dense open-weight model with 128K context.",
	},
	"deepseek-ai/DeepSeek-V4-Pro": {
		Ratio: 1.74 * ratio.MilliTokensUsd, CompletionRatio: 3.48 / 1.74, CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
		ContextLength: 512000, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp4", HuggingFaceID: "deepseek-ai/DeepSeek-V4-Pro",
		Description: "DeepSeek V4-Pro flagship MoE with 512K context and prompt-cache pricing.",
	},
	"deepseek-ai/DeepSeek-V3.1": {
		Ratio: 0.60 * ratio.MilliTokensUsd, CompletionRatio: 1.70 / 0.60,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "deepseek-ai/DeepSeek-V3",
		Description: "DeepSeek V3.1 MoE (671B/37B active) with 128K context. (RETIRED 2026-05-14)",
	},
	"Qwen/Qwen3-Coder-Next-FP8": {
		Ratio: 0.50 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.50,
		ContextLength: 262144, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8",
		Description:  "Next-generation Qwen3 coding model in FP8 (architecture not yet published). (RETIRED 2026-05-14)",
	},
	"Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8": {
		Ratio: 2.00 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 256000, MaxOutputTokens: 65536,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "Qwen/Qwen3-Coder-480B-A35B-Instruct",
		Description: "Qwen3 480B coding MoE (480B/35B active) in FP8, non-thinking mode, 256K context. (RETIRED 2026-06-04)",
	},
	"Qwen/Qwen3-235B-A22B-Instruct-2507-tput": {
		Ratio: 0.20 * ratio.MilliTokensUsd, CompletionRatio: 0.60 / 0.20,
		ContextLength: 262144, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "Qwen/Qwen3-235B-A22B-Instruct",
		Description: "Throughput-optimised Qwen3 235B MoE instruct (July 2025 checkpoint), non-thinking mode.",
	},
	"deepseek-ai/DeepSeek-R1": {
		Ratio: 3.00 * ratio.MilliTokensUsd, CompletionRatio: 7.00 / 3.00,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherR1Sampling,
		Quantization: "fp8", HuggingFaceID: "deepseek-ai/DeepSeek-R1",
		Description: "DeepSeek-R1 671B full reasoning model with extended CoT; restrict temperature to 0.5-0.7. (RETIRED 2026-05-14; use deepseek-ai/DeepSeek-V4-Flash)",
	},
	"meta-llama/Llama-3.3-70B-Instruct-Turbo": {
		Ratio: 1.04 * ratio.MilliTokensUsd, CompletionRatio: 1.04 / 1.04,
		ContextLength: 131072, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "meta-llama/Llama-3.3-70B-Instruct",
		Description: "Meta Llama 3.3 70B instruction model (FP8 turbo) with 128K context.",
	},
	"deepcogito/cogito-v2-1-671b": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 163840, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "bf16", HuggingFaceID: "deepcogito/cogito-v2-1-671b",
		Description: "Cogito v2.1 671B hybrid-reasoning model using IDA for optional extended thinking.",
	},
	"essentialai/rnj-1-instruct": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "bf16", HuggingFaceID: "essentialai/rnj-1-instruct",
		Description: "Essential AI RNJ-1 instruction model in BF16 with 32K context and tool-calling.",
	},
	"Qwen/Qwen2.5-7B-Instruct-Turbo": {
		Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "Qwen/Qwen2.5-7B-Instruct",
		Description: "FP8-quantised Qwen 2.5 7B instruction model with 32K context.",
	},
	"google/gemma-4-31B-it": {
		// Together AI repriced Gemma 4 31B IT to $0.39/$0.97 effective May 2026.
		// Source: https://www.together.ai/pricing (retrieved 2026-05-19)
		Ratio: 0.39 * ratio.MilliTokensUsd, CompletionRatio: 0.97 / 0.39,
		ContextLength: 262144, MaxOutputTokens: 8192,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherChatFeatures, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "google/gemma-4-31b-it",
		Description: "Google Gemma 4 31B multimodal instruction model with 262K context.",
	},
	// Llama Guard 4 12B available on Together serverless moderation tier.
	// Source: https://www.together.ai/pricing (retrieved 2026-05-19)
	"meta-llama/Llama-Guard-4-12B": {
		Ratio: 0.20 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 1048576, MaxOutputTokens: 1024,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedSamplingParameters: togetherChatSampling,
		Quantization:                "fp8", HuggingFaceID: "meta-llama/Llama-Guard-4-12B",
		Description: "Meta Llama Guard 4 12B safety/classification model accepting text+image input with 1M context.",
	},
	"google/gemma-3n-E4B-it": {
		Ratio: 0.06 * ratio.MilliTokensUsd, CompletionRatio: 0.12 / 0.06,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: []string{"json_mode"}, SupportedSamplingParameters: togetherChatSampling,
		Quantization: "fp8", HuggingFaceID: "google/gemma-3n-E4B-it",
		Description: "Google Gemma 3n E4B multimodal instruction model with vision support and 32K context.",
	},
	"LiquidAI/LFM2-24B-A2B": {
		Ratio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.12 / 0.03,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedSamplingParameters: togetherChatSampling,
		Quantization:                "bf16", HuggingFaceID: "LiquidAI/LFM2-24B-A2B",
		Description: "Liquid Foundation Model 2, 24B MoE with hybrid-conv/attn architecture for edge deployment.",
	},
	"meta-llama/Meta-Llama-3-8B-Instruct-Lite": {
		Ratio: 0.14 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedSamplingParameters: togetherChatSampling,
		Quantization:                "bf16", HuggingFaceID: "meta-llama/Meta-Llama-3-8B-Instruct",
		Description: "Lightweight quantised variant of Meta Llama 3 8B for fast low-cost inference.",
	},
	"nvidia/nemotron-3-ultra-550b-a55b": {
		Ratio: 0.60 * ratio.MilliTokensUsd, CompletionRatio: 3.60 / 0.60,
		ContextLength: 1000000, MaxOutputTokens: 32768,
		InputModalities: togetherTextIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		HuggingFaceID: "nvidia/Nemotron-3-Ultra-550B-A55B",
		Description:   "NVIDIA Nemotron-3 Ultra 550B A55B reasoning MoE, 1M context, $0.60/$3.60 per 1M.",
	},
	"moonshotai/Kimi-K2.7-Code": {
		Ratio: 0.95 * ratio.MilliTokensUsd, CompletionRatio: 4.00 / 0.95,
		ContextLength: 262144, MaxOutputTokens: 32768,
		InputModalities: togetherVisionIn, OutputModalities: togetherTextOut,
		SupportedFeatures: togetherReasonFeatures, SupportedSamplingParameters: togetherChatSampling,
		HuggingFaceID: "moonshotai/Kimi-K2.7-Code",
		Description:   "Moonshot Kimi K2.7 Code multimodal coding model, 256K context, $0.95/$4.00 per 1M.",
	},

	// Embeddings.
	"intfloat/multilingual-e5-large-instruct": {
		Ratio:           0.02 * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.02 * ratio.MilliTokensUsd,
		},
	},

	// Image generation models with published pricing.
	"google/imagen-4.0-preview":            {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.04)},
	"google/imagen-4.0-fast":               {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.02)},
	"google/imagen-4.0-ultra":              {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.06)},
	"google/flash-image-2.5":               {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.039)},
	"google/gemini-3-pro-image":            {Ratio: 0, CompletionRatio: 1, Image: togetherAIGeminiProImageConfig()},
	"black-forest-labs/FLUX.1-schnell":     {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0027)},
	"black-forest-labs/FLUX.1.1-pro":       {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.04)},
	"black-forest-labs/FLUX.1-kontext-pro": {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.04)},
	"black-forest-labs/FLUX.1-kontext-max": {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.08)},
	"black-forest-labs/FLUX.1-krea-dev":    {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.025), Description: "FLUX.1 Krea Dev image model. (RETIRED 2026-05-27)"},
	// FLUX.2 family (Together AI pricing page, retrieved 2026-05-18).
	// Source: https://www.together.ai/pricing
	"black-forest-labs/FLUX.2-dev":  {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0154)},
	"black-forest-labs/FLUX.2-flex": {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.03)},
	"black-forest-labs/FLUX.2-pro": {Ratio: 0, CompletionRatio: 1, Image: &adaptor.ImagePricingConfig{
		PricePerImageUsd: 0.03,
		DefaultSize:      togetherAIImageBaseSize,
		MinImages:        1,
	}},
	"black-forest-labs/FLUX.2-max": {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.070)},
	// gpt-image-1.5 and flash-image-3.1 (Gemini 3.1 Flash / Nano Banana 2) bill per image, not per MP.
	// Sources: https://www.together.ai/models/gpt-image-1-5, https://www.together.ai/models/gemini-31-flash-image
	"openai/gpt-image-1.5": {Ratio: 0, CompletionRatio: 1, Image: &adaptor.ImagePricingConfig{
		PricePerImageUsd: 0.034,
		DefaultSize:      togetherAIImageBaseSize,
		MinImages:        1,
	}},
	"google/flash-image-3.1": {Ratio: 0, CompletionRatio: 1, Image: &adaptor.ImagePricingConfig{
		PricePerImageUsd: 0.05,
		DefaultSize:      "1920x1080",
		MinImages:        1,
	}},
	// Additional 2026 image models served by Together AI.
	// Source: https://docs.together.ai/docs/serverless-models
	"Wan-AI/Wan2.6-image":                      {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.03)},
	"ByteDance-Seed/Seedream-3.0":              {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.018)},
	"ByteDance-Seed/Seedream-4.0":              {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.03)},
	"Qwen/Qwen-Image":                          {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0058)},
	"Qwen/Qwen-Image-2.0":                      {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.035)},
	"Qwen/Qwen-Image-2.0-Pro":                  {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.075)},
	"RunDiffusion/Juggernaut-pro-flux":         {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0049)},
	"HiDream-ai/HiDream-I1-Full":               {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.009)},
	"HiDream-ai/HiDream-I1-Dev":                {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0045)},
	"HiDream-ai/HiDream-I1-Fast":               {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0032)},
	"ideogram/ideogram-3.0":                    {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.06)},
	"Lykon/DreamShaper":                        {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0006)},
	"stabilityai/stable-diffusion-3-medium":    {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0019)},
	"stabilityai/stable-diffusion-xl-base-1.0": {Ratio: 0, CompletionRatio: 1, Image: togetherAIImagePerMegapixelConfig(0.0019)},

	// Audio models with published pricing.
	"canopylabs/orpheus-3b-0.1-ft": {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	// Kokoro 82M TTS repriced to $10/1M characters (Together pricing 2026-05-19).
	"hexgrad/Kokoro-82M":          {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"cartesia/sonic-3":            {Ratio: 65.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"cartesia/sonic-2":            {Ratio: 65.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"cartesia/sonic":              {Ratio: 65.0 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"openai/whisper-large-v3":     {Ratio: 0, CompletionRatio: 1, Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.0015 / 60}},
	"nvidia/parakeet-tdt-0.6b-v3": {Ratio: 0, CompletionRatio: 1, Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.0015 / 60}},
}

// ModelList captures the current public Together AI serverless catalog used by the OpenAI-compatible adapter.
// Pricing is currently published for only a subset of these models, so ModelRatios intentionally stays smaller.
// Source: https://docs.together.ai/docs/serverless-models (retrieved 2026-06-13)
var ModelList = []string{
	"MiniMaxAI/MiniMax-M2.7",
	"MiniMaxAI/MiniMax-M2.5",
	"MiniMaxAI/MiniMax-M3",
	"Qwen/Qwen3.6-Plus",
	"Qwen/Qwen3.7-Max",
	"Qwen/Qwen3.5-397B-A17B",
	"Qwen/Qwen3.5-9B",
	"moonshotai/Kimi-K2.6",
	"moonshotai/Kimi-K2.5",
	"moonshotai/Kimi-K2.7-Code",
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
	"meta-llama/Llama-Guard-4-12B",
	"deepcogito/cogito-v2-1-671b",
	"essentialai/rnj-1-instruct",
	"Qwen/Qwen2.5-7B-Instruct-Turbo",
	"google/gemma-4-31B-it",
	"google/gemma-3n-E4B-it",
	"LiquidAI/LFM2-24B-A2B",
	"meta-llama/Meta-Llama-3-8B-Instruct-Lite",
	"nvidia/nemotron-3-ultra-550b-a55b",
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
