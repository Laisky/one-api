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
var ModelRatios = doubaoModels()

// doubaoModels returns the complete native and hosted Ark model catalog.
// It takes no parameters and returns independently constructed model definitions.
// Seed 1.6 boundaries use raw token counts with inclusive lower bounds, preserving
// the historical correction to 32,001 and 128,001 tokens and its cloned metadata.
// Sources checked 2026-10-12 and 2026-11-16:
// https://www.volcengine.com/docs/82379/1330310
// https://www.volcengine.com/docs/82379/1544106
// https://docs.volcengine.com/docs/ark/model-list?lang=zh
// https://docs.volcengine.com/docs/ark/model-pricing?lang=zh
// https://docs.volcengine.com/docs/ark/image-understanding?lang=zh
// https://docs.volcengine.com/docs/ark/video-understanding?lang=zh
// https://docs.volcengine.com/docs/ark/chat-api?lang=zh
func doubaoModels() map[string]adaptor.ModelConfig {
	text := []string{"text"}
	vision := []string{"text", "image"}
	multimodal := []string{"text", "image", "video"}
	reasoning := []string{"tools", "reasoning"}
	structured := []string{"tools", "reasoning", "structured_outputs"}
	return map[string]adaptor.ModelConfig{
		// --- Doubao Seed 1.6 (current flagship deep-thinking multimodal) ---
		// Tiered input pricing: [0,32K] ¥0.8/¥8; (32K,128K] ¥1.2/¥16; (128K,256K] ¥2.4/¥24
		// Cached input ¥0.16; deep-thinking is binary (no published budget).
		// Output uses the unconditional (>200-token) price; the ¥2 promo zone
		// (output ≤200 tokens) cannot be expressed since one-api tiers by input only.
		"doubao-seed-1.6": (adaptor.ModelConfig{
			Ratio:            0.8 * ratio.MilliTokensRmb,
			CompletionRatio:  8.0 / 0.8,
			CachedInputRatio: 0.16 * ratio.MilliTokensRmb,
			Tiers: []adaptor.ModelRatioTier{
				{Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 1.2, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 32001},
				{Ratio: 2.4 * ratio.MilliTokensRmb, CompletionRatio: 24.0 / 2.4, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 128001},
			},
			ContextLength:               256000,
			MaxOutputTokens:             32000,
			InputModalities:             doubaoMultimodalInputs,
			OutputModalities:            doubaoTextOutputs,
			SupportedFeatures:           doubaoReasoningFeatures,
			SupportedSamplingParameters: doubaoSamplingParams,
			Description:                 "ByteDance Doubao Seed 1.6: multimodal deep-thinking flagship with 256K context and text/image/video input.",
		}).Clone(),
		"doubao-seed-1.6-flash": (adaptor.ModelConfig{
			Ratio:            0.15 * ratio.MilliTokensRmb,
			CompletionRatio:  1.5 / 0.15,
			CachedInputRatio: 0.03 * ratio.MilliTokensRmb,
			// Tiered input pricing: [0,32K] ¥0.15/¥1.5; (32K,128K] ¥0.3/¥3; (128K,256K] ¥0.6/¥6.
			Tiers: []adaptor.ModelRatioTier{
				{Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 3.0 / 0.3, CachedInputRatio: 0.03 * ratio.MilliTokensRmb, InputTokenThreshold: 32001},
				{Ratio: 0.6 * ratio.MilliTokensRmb, CompletionRatio: 6.0 / 0.6, CachedInputRatio: 0.03 * ratio.MilliTokensRmb, InputTokenThreshold: 128001},
			},
			ContextLength:               256000,
			MaxOutputTokens:             32000,
			InputModalities:             doubaoMultimodalInputs,
			OutputModalities:            doubaoTextOutputs,
			SupportedFeatures:           doubaoReasoningFeatures,
			SupportedSamplingParameters: doubaoSamplingParams,
			Description:                 "ByteDance Doubao Seed 1.6 Flash: cost-optimized multimodal deep-thinking variant with 256K context.",
		}).Clone(),
		// doubao-seed-1.6-vision: vision-grounding deep-thinking variant (GUI task
		// grounding + reasoning), text/image input only (no video).
		// Tiered input pricing: [0,32K] ¥0.8/¥8; (32K,128K] ¥1.2/¥16; (128K,256K] ¥2.4/¥24.
		// Cached input ¥0.16.
		"doubao-seed-1.6-vision": (adaptor.ModelConfig{
			Ratio:            0.8 * ratio.MilliTokensRmb,
			CompletionRatio:  8.0 / 0.8,
			CachedInputRatio: 0.16 * ratio.MilliTokensRmb,
			Tiers: []adaptor.ModelRatioTier{
				{Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 16.0 / 1.2, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 32001},
				{Ratio: 2.4 * ratio.MilliTokensRmb, CompletionRatio: 24.0 / 2.4, CachedInputRatio: 0.16 * ratio.MilliTokensRmb, InputTokenThreshold: 128001},
			},
			ContextLength:               256000,
			MaxOutputTokens:             32000,
			InputModalities:             doubaoVisionInputs,
			OutputModalities:            doubaoTextOutputs,
			SupportedFeatures:           doubaoReasoningFeatures,
			SupportedSamplingParameters: doubaoSamplingParams,
			Description:                 "ByteDance Doubao Seed 1.6 Vision: vision-grounding deep-thinking model for GUI/visual-agent tasks, with 256K context and text/image input.",
		}).Clone(),
		// doubao-seed-1.6-lite: cost-optimized Seed 1.6 variant.
		// Tiered input pricing: [0,32K] ¥0.3/¥2.4; (32K,128K] ¥0.6/¥4; (128K,256K] ¥1.2/¥12.
		// Cached input ¥0.06. ContextLength/MaxOutputTokens inferred by analogy to
		// doubao-seed-1.6-flash (not independently confirmed on the capability page).
		"doubao-seed-1.6-lite": (adaptor.ModelConfig{
			Ratio:            0.3 * ratio.MilliTokensRmb,
			CompletionRatio:  2.4 / 0.3,
			CachedInputRatio: 0.06 * ratio.MilliTokensRmb,
			Tiers: []adaptor.ModelRatioTier{
				{Ratio: 0.6 * ratio.MilliTokensRmb, CompletionRatio: 4.0 / 0.6, CachedInputRatio: 0.06 * ratio.MilliTokensRmb, InputTokenThreshold: 32001},
				{Ratio: 1.2 * ratio.MilliTokensRmb, CompletionRatio: 12.0 / 1.2, CachedInputRatio: 0.06 * ratio.MilliTokensRmb, InputTokenThreshold: 128001},
			},
			ContextLength:               256000,
			MaxOutputTokens:             32000,
			InputModalities:             doubaoMultimodalInputs,
			OutputModalities:            doubaoTextOutputs,
			SupportedFeatures:           doubaoReasoningFeatures,
			SupportedSamplingParameters: doubaoSamplingParams,
			Description:                 "ByteDance Doubao Seed 1.6 Lite: cost-optimized multimodal deep-thinking variant with 256K context.",
		}).Clone(),
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
			MaxOutputTokens:             256000,
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
			Description:                 "ByteDance Doubao 1.5 Pro 256K long-context text chat model. [Legacy/likely retired: absent from the current per-token pricing table and full model-list page as of 2026-07; pricing retained as last-known value pending vendor confirmation.]",
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
			Ratio:                       5 * ratio.MilliTokensRmb,
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
			Ratio:                       5 * ratio.MilliTokensRmb,
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
			Ratio:                       0.8 * ratio.MilliTokensRmb,
			CompletionRatio:             2.00 / 0.80,
			ContextLength:               32000,
			MaxOutputTokens:             4096,
			InputModalities:             doubaoTextInputs,
			OutputModalities:            doubaoTextOutputs,
			SupportedFeatures:           doubaoChatFeatures,
			SupportedSamplingParameters: doubaoSamplingParams,
			Description:                 "ByteDance Doubao Pro 32k legacy chat model.",
		}, // CNY 0.80 in / 2.00 out per 1M (batch tier, 火山方舟 2026-08-28; withdrawn from online inference)
		"Doubao-pro-4k": {
			Ratio:                       0.8 * ratio.MilliTokensRmb,
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
			Ratio:                       0.8 * ratio.MilliTokensRmb,
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
			Ratio:                       0.6 * ratio.MilliTokensRmb,
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
			Ratio:                       0.3 * ratio.MilliTokensRmb,
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
			Ratio:            0.5 * ratio.MilliTokensRmb,
			CompletionRatio:  1,
			ContextLength:    4096,
			InputModalities:  doubaoTextInputs,
			OutputModalities: doubaoTextOutputs,
			Description:      "ByteDance Doubao text embedding model. [Legacy/likely retired: absent from the direct-API vector-model pricing table and model list as of 2026-07; only doubao-embedding-vision remains listed there. The sole surviving reference is a Knowledge-Base bundled line item at a different price/billing path. Pricing retained as last-known value pending vendor confirmation.]",
		}, // CNY 0.5 per 1M (survives only as a 知识库 line item, 火山方舟 2026-08-28)
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
		"doubao-seed-evolving":           arkChatConfig(6, 1.2, 30, 1024000, 256000, multimodal, structured, "Rapidly evolving Coding/Agent model; version changes upstream without a new ID."),
		"doubao-seed-2-1-pro-260915":     arkChatConfig(6, 1.2, 30, 1024000, 256000, multimodal, structured, "Seed 2.1 Pro September snapshot with 1024K context."),
		"doubao-seed-2-1-lite-260915":    arkChatConfig(.8, .16, 2.7, 1024000, 256000, multimodal, structured, "Seed 2.1 Lite September snapshot. This gateway entry advertises non-audio use only. Native audio costs CNY 12 input and 2.4 cached input per million; modality-specific cached-audio settlement requires separate integration."),
		"doubao-seed-2-1-turbo-260628":   arkChatConfig(3, .6, 15, 256000, 256000, multimodal, structured, "Seed 2.1 Turbo June snapshot with 256K context."),
		"doubao-seed-2-1-pro-260628":     arkChatConfig(6, 1.2, 30, 256000, 256000, multimodal, structured, "Earlier Seed 2.1 Pro June snapshot; retains its own 256K context."),
		"glm-5-3-flash-260828":           arkChatConfig(.8, .23, 2.8, 1024000, 128000, vision, reasoning, "Ark-hosted GLM-5.3-Flash August snapshot. Structured outputs are not advertised by this host's model table."),
		"glm-5-2-260617":                 arkChatConfig(8, 2, 28, 1024000, 128000, text, reasoning, "Ark-hosted GLM-5.2 June snapshot with text-only input."),
		"deepseek-v4-pro-ga-260813":      arkChatConfig(9, .30, 27, 1024000, 384000, text, structured, "Ark-hosted DeepSeek V4 Pro GA snapshot at current Standard prices, not native DeepSeek off-peak rates."),
		"deepseek-v4-flash-ga-260731":    arkChatConfig(3, .10, 9, 1024000, 384000, text, structured, "Ark-hosted DeepSeek V4 Flash GA snapshot. Uses the rate effective since 2026-08-21, not the expired CNY 1/0.2/2 tariff."),
		"doubao-seed-translation-250915": arkChatConfig(1.2, 0, 3.6, 4000, 3000, text, nil, "Translation-specialized model. No cache discount, tools or reasoning is advertised for Standard inference."),
		"kimi-k3-1-pro-261024": arkChatConfig(13, 1.3, 68, 1_024_000, 1_024_000,
			[]string{"text", "image", "video"}, []string{"tools", "reasoning", "structured_outputs"},
			"Ark-hosted Kimi K3.1 Pro October snapshot. Thinking is always enabled; temperature is fixed at 1. Image and video input require Base64 data, not remote URLs. The compatible gateway does not forward this model's upstream reasoning_effort. Native Moonshot cache-write tariffs and Coding Plan multipliers do not apply."),
	}
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// DoubaoToolingDefaults documents that Bytedance's Doubao cloud pricing does not list per-tool fees publicly (retrieved 2026-05-18).
// Source: https://www.volcengine.com/docs/82379/1099320
var DoubaoToolingDefaults = adaptor.ChannelToolConfig{}

// arkChatConfig builds independent non-audio Standard chat metadata. Parameters
// are CNY per-million prices, advisory token limits, modalities and features;
// it returns a new configuration without account quotas or invented effort levels.
func arkChatConfig(input, cached, output float64, context, maxOutput int32, modalities, features []string, description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{Ratio: input * ratio.MilliTokensRmb, CachedInputRatio: cached * ratio.MilliTokensRmb, CompletionRatio: output / input,
		ContextLength: context, MaxOutputTokens: maxOutput, InputModalities: append([]string(nil), modalities...), OutputModalities: []string{"text"},
		SupportedFeatures: append([]string(nil), features...), SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
		Description: description + " Ark Standard non-audio tariff; native CNY per million tokens. Advisory K limits use the existing decimal-thousand convention. Low-latency/low-priority/batch and cache-storage fees are separate. No deployment entitlement is implied."}
}
