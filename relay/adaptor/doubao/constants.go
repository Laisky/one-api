package doubao

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Doubao (ByteDance Volcengine) is a closed-weight Chinese cloud LLM family.
// Public docs:
//   - https://www.volcengine.com/docs/82379/1099455 (model overview)
//   - https://www.volcengine.com/docs/82379/1330310 (model list)
//   - https://www.volcengine.com/docs/82379/1099320 (pricing)
//
// Pricing is published in CNY per million tokens. We convert via
// ratio.MilliTokensRmb, which divides by the codebase USD↔RMB exchange rate
// (1 USD = 8 RMB ≈ 0.125 USD/RMB; close to the requested 0.14 conversion).
//
// All Doubao production models are accessed through OpenAI-compatible chat
// completion APIs that support tool calling and JSON mode. Vision-capable
// variants accept image inputs in addition to text. doubao-seed-1.6 also
// accepts video input. Weights are not published on HuggingFace, so
// HuggingFaceID and Quantization stay empty.
//
// Doubao "deep thinking" toggles reasoning on/off (binary). The provider does
// not publish a configurable reasoning budget, so MaxReasoningTokens is left
// empty for reasoning-capable models per project convention.

// doubaoTextInputs lists modalities accepted by text-only Doubao chat models.
var doubaoTextInputs = []string{"text"}

// doubaoVisionInputs lists modalities for vision-capable Doubao models.
var doubaoVisionInputs = []string{"text", "image"}

// doubaoMultimodalInputs lists the full multimodal input set accepted by the
// doubao-seed-1.6 deep-thinking flagship (text + image + video).
var doubaoMultimodalInputs = []string{"text", "image", "video"}

// doubaoEmbeddingVisionInputs lists modalities for the multimodal embedding
// model, which embeds text, images, and short video frames.
var doubaoEmbeddingVisionInputs = []string{"text", "image", "video"}

// doubaoTextOutputs declares the text-only output modality used by all chat
// models in the Doubao lineup.
var doubaoTextOutputs = []string{"text"}

// doubaoChatFeatures captures the tool / JSON capabilities advertised by the
// Volcengine Ark API for standard Doubao chat models.
var doubaoChatFeatures = []string{"tools", "json_mode", "structured_outputs"}

// doubaoReasoningFeatures appends the "reasoning" flag for deep-thinking
// capable models such as doubao-seed-1.6.
var doubaoReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

// doubaoSamplingParams enumerates OpenAI-compatible sampling parameters that
// the Doubao Ark endpoint accepts for chat generation.
var doubaoSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Pricing source: https://www.volcengine.com/docs/82379/1099320 (last verified 2026-05).
var ModelRatios = map[string]adaptor.ModelConfig{
	// --- Doubao Seed 1.6 (current flagship deep-thinking multimodal) ---
	// Tiered input pricing: [0,32K] ¥0.8/¥8; (32K,128K] ¥1.2/¥16; (128K,256K] ¥2.4/¥24
	// Cached input ¥0.16; deep-thinking is binary (no published budget).
	// Output uses the unconditional (>200-token) price; the ¥2 promo zone
	// (output ≤200 tokens) cannot be expressed since one-api tiers by input only.
	"doubao-seed-1.6": {
		Ratio:            0.8 * ratio.MilliTokensRmb,
		CompletionRatio:  8.0 / 0.8,
		CachedInputRatio: 0.16 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 1.2, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
			{Ratio: 2.4 * ratio.MilliTokensRmb, CompletionRatio: 24.0 / 2.4, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 128},
		},
		ContextLength:               256000,
		MaxOutputTokens:             16384,
		InputModalities:             doubaoMultimodalInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoReasoningFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Seed 1.6: multimodal deep-thinking flagship with 256K context and text/image/video input.",
	},
	"doubao-seed-1.6-flash": {
		Ratio:            0.15 * ratio.MilliTokensRmb,
		CompletionRatio:  1.5 / 0.15,
		CachedInputRatio: 0.03 * ratio.MilliTokensRmb,
		// Tiered input pricing: [0,32K] ¥0.15/¥1.5; (32K,128K] ¥0.3/¥3; (128K,256K] ¥0.6/¥6.
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 3.0 / 0.3, CachedInputRatio: 0.03 * ratio.MilliTokensRmb, InputTokenThreshold: 32},
			{Ratio: 0.6 * ratio.MilliTokensRmb, CompletionRatio: 6.0 / 0.6, CachedInputRatio: 0.03 * ratio.MilliTokensRmb, InputTokenThreshold: 128},
		},
		ContextLength:               256000,
		MaxOutputTokens:             16384,
		InputModalities:             doubaoMultimodalInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoReasoningFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Seed 1.6 Flash: cost-optimized multimodal deep-thinking variant with 256K context.",
	},

	// --- Doubao Seed 2.1 (FORCE 2026 flagship deep-thinking multimodal) ---
	// Unified (non input-length-tiered) pricing per Volcengine:
	//   Pro:   ¥6 in / ¥30 out / ¥1.2 cached
	//   Turbo: ¥3 in / ¥15 out / ¥0.6 cached
	// Both have 256K context, deep-thinking, multimodal (text/image/video)
	// understanding, and tool calling. ARK ids: doubao-seed-2-1-pro-260628 /
	// doubao-seed-2-1-turbo-260628 (released 2026-06-23).
	"doubao-seed-2.1-pro": {
		Ratio:                       6 * ratio.MilliTokensRmb,
		CompletionRatio:             30.0 / 6.0,
		CachedInputRatio:            1.2 * ratio.MilliTokensRmb,
		ContextLength:               256000,
		MaxOutputTokens:             128000,
		InputModalities:             doubaoMultimodalInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoReasoningFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Seed 2.1 Pro: flagship deep-thinking multimodal model (text/image/video input) with 256K context, built for coding and agent workloads.",
	},
	"doubao-seed-2.1-turbo": {
		Ratio:                       3 * ratio.MilliTokensRmb,
		CompletionRatio:             15.0 / 3.0,
		CachedInputRatio:            0.6 * ratio.MilliTokensRmb,
		ContextLength:               256000,
		MaxOutputTokens:             256000,
		InputModalities:             doubaoMultimodalInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoReasoningFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Seed 2.1 Turbo: low-cost low-latency deep-thinking multimodal model with 256K context, half the price of 2.1 Pro.",
	},

	// --- Doubao 1.5 series ---
	"doubao-1.5-pro-32k": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             2.0 / 0.8,
		CachedInputRatio:            0.16 * ratio.MilliTokensRmb,
		ContextLength:               32000,
		MaxOutputTokens:             12288,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao 1.5 Pro 32K text chat model with tool calling and JSON mode.",
	},
	"doubao-1.5-pro-256k": {
		Ratio:                       5 * ratio.MilliTokensRmb,
		CompletionRatio:             9.0 / 5.0,
		CachedInputRatio:            1 * ratio.MilliTokensRmb,
		ContextLength:               256000,
		MaxOutputTokens:             12288,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao 1.5 Pro 256K long-context text chat model.",
	},
	"doubao-1.5-lite-32k": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             0.6 / 0.3,
		CachedInputRatio:            0.06 * ratio.MilliTokensRmb,
		ContextLength:               32000,
		MaxOutputTokens:             12288,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao 1.5 Lite 32K cost-optimized text chat model.",
	},
	"doubao-1.5-vision-pro-32k": {
		Ratio:                       3 * ratio.MilliTokensRmb,
		CompletionRatio:             9.0 / 3.0,
		ContextLength:               32000,
		MaxOutputTokens:             12288,
		InputModalities:             doubaoVisionInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao 1.5 Vision Pro 32K vision-language model with tool calling.",
	},

	// --- Doubao Pro (legacy, retained for backward compatibility) ---
	"Doubao-pro-256k": {
		Ratio:                       0.005 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               256000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Pro 256K legacy long-context chat model.",
	},
	"Doubao-pro-128k": {
		Ratio:                       0.005 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Pro 128k legacy chat model with tool calling and JSON mode.",
	},
	"Doubao-pro-32k": {
		Ratio:                       0.002 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Pro 32k legacy chat model.",
	},
	"Doubao-pro-4k": {
		Ratio:                       0.0008 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Pro 4k legacy chat model for short prompts.",
	},

	// --- Doubao Lite (legacy) ---
	"Doubao-lite-128k": {
		Ratio:                       0.0008 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Lite 128k legacy cost-optimized chat model.",
	},
	"Doubao-lite-32k": {
		Ratio:                       0.0006 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Lite 32k legacy cost-optimized chat model.",
	},
	"Doubao-lite-4k": {
		Ratio:                       0.0003 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Lite 4k legacy cost-optimized chat model.",
	},

	// --- Embedding Models ---
	"Doubao-embedding": {
		Ratio:            0.0002 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    4096,
		InputModalities:  doubaoTextInputs,
		OutputModalities: doubaoTextOutputs,
		Description:      "ByteDance Doubao text embedding model.",
	},
	"doubao-embedding-vision": {
		Ratio:            0.7 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    128000,
		InputModalities:  doubaoEmbeddingVisionInputs,
		OutputModalities: doubaoTextOutputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio:  0.7 * ratio.MilliTokensRmb,
			ImageTokenRatio: 1.8 * ratio.MilliTokensRmb,
			VideoTokenRatio: 1.8 * ratio.MilliTokensRmb,
		},
		Description: "ByteDance Doubao multimodal embedding model for text, image, and short video inputs.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// DoubaoToolingDefaults documents that Bytedance's Doubao cloud pricing does not list per-tool fees publicly (retrieved 2026-05-18).
// Source: https://www.volcengine.com/docs/82379/1099320
var DoubaoToolingDefaults = adaptor.ChannelToolConfig{}
