package baiduv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for the Qianfan v2 (OpenAI-compatible) ERNIE chat models
// and DeepSeek hosted variants. Reused across ModelRatios entries so the table
// stays compact and consistent.
var (
	// ernieV2TextInputs lists the input modalities for text-only ERNIE v2 chat models.
	ernieV2TextInputs = []string{"text"}
	// ernieV2TextOutputs lists the output modalities for ERNIE v2 chat completions.
	ernieV2TextOutputs = []string{"text"}

	// ernieV2ChatFeatures lists the capability set for ERNIE 4.0/3.5 chat tiers
	// that support tool-calling and JSON mode on the Qianfan v2 API.
	ernieV2ChatFeatures = []string{"tools", "json_mode"}
	// ernieV2TurboFeatures lists the capability set for the ERNIE 4.5 Turbo family,
	// which adds structured outputs on top of tool-calling and JSON mode per the
	// Qianfan v2 reference (verified 2026-05-18 via LLMReference catalog).
	ernieV2TurboFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// ernieV2VisionInputs lists the input modalities for multimodal ERNIE Turbo VL.
	ernieV2VisionInputs = []string{"text", "image"}
	// ernieV2OmniInputs lists the input modalities for native omni-modal ERNIE 5.0,
	// which jointly models text, image, audio and video per the Qianfan reference.
	ernieV2OmniInputs = []string{"text", "image", "audio", "video"}
	// ernieV2ReasoningFeatures lists the capability set for the ERNIE X1 reasoning
	// family. Baidu exposes reasoning as a binary thinking mode without a tunable
	// budget so MaxReasoningTokens stays unset.
	ernieV2ReasoningFeatures = []string{"tools", "json_mode", "reasoning"}
	// ernieV2BasicFeatures lists the reduced capability set for Speed/Lite/Tiny/
	// Character/Novel tiers — tool-calling per Qianfan docs but no advertised JSON mode.
	ernieV2BasicFeatures = []string{"tools"}

	// deepseekV2ChatFeatures advertises the capability set for non-thinking DeepSeek
	// models hosted on Baidu Qianfan v2.
	deepseekV2ChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// deepseekV2ReasoningFeatures advertises the capability set for thinking-mode
	// DeepSeek and DeepSeek-R1-distilled models hosted on Baidu Qianfan v2.
	deepseekV2ReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

	// qianfanV2SamplingParameters lists OpenAI-compatible sampling parameters Qianfan v2
	// accepts. Baidu also exposes top_k and a repetition penalty alongside the
	// standard OpenAI knobs for Chinese-cloud chat APIs.
	qianfanV2SamplingParameters = []string{
		"temperature",
		"top_p",
		"top_k",
		"frequency_penalty",
		"presence_penalty",
		"repetition_penalty",
		"stop",
		"seed",
		"max_tokens",
	}
	// deepseekV2SamplingParameters lists the standard OpenAI-compatible sampling
	// parameters supported by hosted DeepSeek chat models on Qianfan v2.
	deepseekV2SamplingParameters = []string{
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
		"stop",
		"seed",
		"max_tokens",
	}
)

// ModelRatios contains all supported models and their pricing/configuration metadata.
// Model list is derived from the keys of this map, eliminating redundancy.
//
// Pricing sources (verified 2026-06-13):
//   - https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya (Qianfan inference pricing — primary)
//   - https://cloud.baidu.com/doc/WENXINWORKSHOP/s/hlrk4akp7 (legacy ERNIE pricing — verified)
//
// Capability metadata sources:
//   - https://cloud.baidu.com/doc/qianfan-api/s/ (Qianfan v2 API reference)
//   - https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Nlks5zkzu (ERNIE model catalog)
//   - https://ai.baidu.com/ai-doc/AISTUDIO/Mmhslv9lf (per-model context/output limits)
//   - https://huggingface.co/baidu (open-weight ERNIE 4.5 family)
//   - https://huggingface.co/deepseek-ai (hosted DeepSeek chat models)
//
// Notes:
//   - ERNIE 4.5 / 4.5 Turbo and ERNIE X1 / X1 Turbo are served through the Qianfan v2
//     OpenAI-compatible endpoint, so they live in this adaptor rather than the legacy v1.
//   - Baidu exposes reasoning on the X1 family as a binary thinking mode (no tunable
//     budget), so MaxReasoningTokens is intentionally left at zero per the memory rule
//     applied to Chinese-cloud reasoning models.
//   - Prompt cache: Qianfan reports cache hits via the nested
//     usage.prompt_tokens_details.cached_tokens field on the v2 endpoint (captured by
//     the openai handler). Per the platform billing rule, cached_tokens are charged at
//     40% of the standard input unit price, so every ERNIE entry sets
//     CachedInputRatio = 0.4 * Ratio (https://ai.baidu.com/ai-doc/WENXINWORKSHOP/Rm6uq7jy9).
//     Hosted DeepSeek models follow DeepSeek's own (unconfirmed-on-Baidu) cache economics
//     and are intentionally left without a CachedInputRatio.
var ModelRatios = map[string]adaptor.ModelConfig{
	// ERNIE 5.x Models (next-generation flagship and reasoning family released 2026-05-09)
	"ernie-5.1": {
		// Tiered: ¥4 input / ¥18 output per 1M (input<=32k); ¥6 input / ¥22 output per 1M (32k<input<=128k)
		Ratio:           4 * ratio.MilliTokensRmb,
		CompletionRatio: 18.0 / 4.0,
		Tiers: []adaptor.ModelRatioTier{
			{InputTokenThreshold: 32768, Ratio: 0.006 * ratio.MilliTokensRmb, CompletionRatio: 0.022 / 0.006}, // 32k<input<=128k tier
		},
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		MaxReasoningTokens:          61440,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 5.1 next-generation text chat model (released 2026-05-09) on Qianfan v2 API; 128K context.",
	},
	"ernie-5.0": {
		// Tiered: ¥6 input / ¥24 output per 1M (input<=32k); ¥10 input / ¥40 output per 1M (32k<input<=128k)
		Ratio:           6 * ratio.MilliTokensRmb,
		CompletionRatio: 24.0 / 6.0,
		Tiers: []adaptor.ModelRatioTier{
			{InputTokenThreshold: 32768, Ratio: 0.01 * ratio.MilliTokensRmb, CompletionRatio: 0.04 / 0.01}, // 32k<input<=128k tier
		},
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2OmniInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2TurboFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 5.0 native omni-modal flagship (released 2026-05-09) with text+image+audio+video input on Qianfan v2 API; 128K context.",
	},
	"ernie-x1.1": {
		// ¥1 input ($0.139 USD) / ¥4 output ($0.556 USD) per 1M tokens, context 64K, reasoning
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             4.0 / 1.0,
		ContextLength:               65536,
		MaxOutputTokens:             65536,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ReasoningFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE X1.1 improved deep-reasoning model on Qianfan v2 API; 64K context.",
	},

	// ERNIE 4.5 Models (closed-weight flagship family released March 2025)
	"ernie-4.5": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.004 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             4,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2TurboFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.5: closed-weight flagship chat model on Qianfan v2 (text-only).",
	}, // CNY 0.004 / 0.016 per 1k tokens (input/output)
	"ernie-4.5-turbo-32k": {
		Ratio:                       0.0008 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.0002 * ratio.MilliTokensRmb, // Prompt cache: official cache-hit price ¥0.0002/1k (25% of input).
		CompletionRatio:             4,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2TurboFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.5 Turbo 32K: closed-weight balanced-cost chat model priced at 20% of ERNIE 4.5.",
	}, // CNY 0.0008 / 0.0032 per 1k tokens
	"ernie-4.5-turbo-128k": {
		Ratio:                       0.0008 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.0002 * ratio.MilliTokensRmb, // Prompt cache: official cache-hit price ¥0.0002/1k (25% of input).
		CompletionRatio:             4,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2TurboFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.5 Turbo 128K: long-context closed-weight balanced-cost chat model.",
	}, // CNY 0.0008 / 0.0032 per 1k tokens
	"ernie-4.5-turbo-vl": {
		Ratio:                       0.003 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.003 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             3,
		ContextLength:               131072,
		MaxOutputTokens:             16384,
		InputModalities:             ernieV2VisionInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2TurboFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.5 Turbo VL: closed-weight multimodal chat model accepting text and image inputs.",
	}, // CNY 0.003 / 0.009 per 1k tokens

	// ERNIE X1 Models (closed-weight reasoning family)
	"ernie-x1": {
		Ratio:                       0.002 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.002 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             4,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ReasoningFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE X1: closed-weight deep-thinking reasoning chat model on Qianfan v2.",
	}, // CNY 0.002 / 0.008 per 1k tokens
	"ernie-x1-turbo": {
		Ratio:                       0.001 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.001 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             4,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ReasoningFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE X1 Turbo: closed-weight reasoning chat model priced at half of ERNIE X1.",
	}, // CNY 0.001 / 0.004 per 1k tokens

	// ERNIE 4.0 Models
	"ernie-4.0-8k-latest": {
		Ratio:                       0.12 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.12 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 8K (latest alias): closed-weight flagship chat model on Qianfan v2.",
	}, // CNY 0.12 / 1k tokens
	"ernie-4.0-8k-preview": {
		Ratio:                       0.12 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.12 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 8K (preview): closed-weight flagship chat model on Qianfan v2.",
	}, // CNY 0.12 / 1k tokens
	"ernie-4.0-8k": {
		Ratio:                       0.12 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.12 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 8K: closed-weight flagship chat model on Qianfan v2.",
	}, // CNY 0.12 / 1k tokens
	"ernie-4.0-turbo-8k-latest": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.02 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 8K (latest alias): closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-8k-preview": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.02 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 8K (preview): closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-8k": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.02 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 8K: closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-128k": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.02 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 128K: long-context closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens

	// ERNIE 3.5 Models
	"ernie-3.5-8k-preview": {
		Ratio:                       0.012 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.012 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 3.5 8K (preview): closed-weight general-purpose chat model.",
	}, // CNY 0.012 / 1k tokens
	"ernie-3.5-8k": {
		Ratio:                       0.012 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.012 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 3.5 8K: closed-weight general-purpose chat model.",
	}, // CNY 0.012 / 1k tokens
	"ernie-3.5-128k": {
		Ratio:                       0.012 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.012 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 3.5 128K: long-context closed-weight general-purpose chat model.",
	}, // CNY 0.012 / 1k tokens

	// ERNIE Speed Models
	"ernie-speed-8k": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.004 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Speed 8K: throughput-optimized closed-weight chat tier on Qianfan v2.",
	}, // CNY 0.004 / 1k tokens
	"ernie-speed-128k": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.004 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Speed 128K: long-context throughput-optimized closed-weight chat tier.",
	}, // CNY 0.004 / 1k tokens
	"ernie-speed-pro-128k": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.3 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             0.6 / 0.3,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Speed Pro 128K: enhanced throughput-optimized closed-weight chat tier.",
	}, // CNY 0.3 input / 0.6 output per 1M tokens

	// ERNIE Lite Models
	"ernie-lite-8k": {
		Ratio:                       0.008 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.008 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Lite 8K: cost-efficient closed-weight chat tier on Qianfan v2.",
	}, // CNY 0.008 / 1k tokens
	"ernie-lite-pro-128k": {
		Ratio:                       0.2 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.2 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             0.4 / 0.2,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Lite Pro 128K: long-context cost-efficient closed-weight chat tier.",
	}, // CNY 0.2 input / 0.4 output per 1M tokens

	// ERNIE Tiny Models
	"ernie-tiny-8k": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.004 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Tiny 8K: ultra low-cost closed-weight chat tier on Qianfan v2.",
	}, // CNY 0.004 / 1k tokens

	// ERNIE Character Models
	"ernie-char-8k": {
		Ratio:                       0.04 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.04 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Character 8K: closed-weight role-play / persona chat model.",
	}, // CNY 0.04 / 1k tokens
	"ernie-char-fiction-8k": {
		Ratio:                       0.04 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.04 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Character Fiction 8K: closed-weight model tuned for fictional persona dialogue.",
	}, // CNY 0.04 / 1k tokens
	"ernie-novel-8k": {
		Ratio:                       0.04 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.4 * (0.04 * ratio.MilliTokensRmb), // Prompt cache: cached input billed at 40% of input price.
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Novel 8K: closed-weight model specialized for long-form fiction continuation.",
	}, // CNY 0.04 / 1k tokens

	// DeepSeek Models (hosted on Baidu)
	"deepseek-v3": {
		Ratio:                       0.002 * ratio.MilliTokensRmb,
		CompletionRatio:             0.008 / 0.002,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ChatFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
		Description:                 "DeepSeek V3 hosted on Baidu Qianfan v2: open-weight MoE chat model (non-thinking mode).",
	}, // CNY 0.002 input / 0.008 output per 1k tokens
	"deepseek-r1": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CompletionRatio:             0.016 / 0.004,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ReasoningFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		MaxReasoningTokens:          32768,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
		Description:                 "DeepSeek R1 hosted on Baidu Qianfan v2: open-weight reasoning chat model (thinking mode).",
	}, // CNY 0.004 input / 0.016 output per 1k tokens
	"deepseek-r1-distill-qwen-32b": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ReasoningFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		MaxReasoningTokens:          32768,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
		Description:                 "DeepSeek R1 distilled into Qwen-32B, hosted on Baidu Qianfan v2.",
	}, // CNY 0.004 / 1k tokens
	"deepseek-r1-distill-qwen-14b": {
		Ratio:                       0.003 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ReasoningFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		MaxReasoningTokens:          32768,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-14B",
		Description:                 "DeepSeek R1 distilled into Qwen-14B, hosted on Baidu Qianfan v2.",
	}, // CNY 0.003 / 1k tokens
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// BaiduV2ToolingDefaults records that the updated Qianfan billing docs do not enumerate per-tool tariffs (retrieved 2026-05-18).
// Source: https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya
var BaiduV2ToolingDefaults = adaptor.ChannelToolConfig{}
