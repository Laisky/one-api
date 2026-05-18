package minimax

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// MiniMax operates the abab, MiniMax-Text/VL, and MiniMax-M chat families.
// Reference docs:
//   - https://platform.minimaxi.com/docs/guides/models-intro (model catalog)
//   - https://platform.minimaxi.com/docs/guides/pricing-paygo (pay-as-you-go pricing)
//
// abab* and the MiniMax-Text-01 / MiniMax-VL-01 production endpoints are
// closed-weight. MiniMax-M1 / M2 / M2.5 / M2.7 are the current MiniMax-M
// reasoning models; M1 weights are open on HuggingFace (MiniMaxAI/MiniMax-M1-80k)
// and Text-01 weights are open (MiniMaxAI/MiniMax-Text-01). MiniMax-VL-01
// accepts text + image inputs. Prices retrieved 2026-05-18 in CNY per 1M tokens.

// minimaxTextInputs is the input modality set for text-only MiniMax models.
var minimaxTextInputs = []string{"text"}

// minimaxVisionInputs is the modality set for MiniMax-VL multimodal models.
var minimaxVisionInputs = []string{"text", "image"}

// minimaxTextOutputs is the text-only output modality used by chat APIs.
var minimaxTextOutputs = []string{"text"}

// minimaxChatFeatures advertises tool / JSON capabilities exposed by the
// MiniMax chat API for production abab and MiniMax-Text/VL endpoints.
var minimaxChatFeatures = []string{"tools", "json_mode"}

// minimaxReasoningFeatures extends the chat feature set with reasoning for
// the MiniMax-M1 / M2 series (binary thinking switch, no tunable budget).
var minimaxReasoningFeatures = []string{"tools", "json_mode", "reasoning"}

// minimaxSamplingParams enumerates sampling parameters accepted by MiniMax
// chat completions.
var minimaxSamplingParams = []string{
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
// Based on MiniMax pay-as-you-go pricing: https://platform.minimaxi.com/docs/guides/pricing-paygo
var ModelRatios = map[string]adaptor.ModelConfig{
	// MiniMax-M reasoning family (current). MiniMax-M2 and successors share
	// the ¥2.1 input / ¥8.4 output base rate with cache-read ¥0.42 and
	// cache-write ¥2.625 per 1M tokens. The "-highspeed" tiers double the
	// base price. Reasoning is a binary toggle (no tunable budget) so
	// MaxReasoningTokens is left at 0 per memory note.
	"MiniMax-M2.7": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             8.4 / 2.1,
		CachedInputRatio:            0.42 * ratio.MilliTokensRmb,
		CacheWrite5mRatio:           2.625 * ratio.MilliTokensRmb,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-M2.7 flagship reasoning chat model (2026-05) with iterative self-improvement.",
	},
	"MiniMax-M2.7-highspeed": {
		Ratio:                       4.2 * ratio.MilliTokensRmb,
		CompletionRatio:             16.8 / 4.2,
		CachedInputRatio:            0.42 * ratio.MilliTokensRmb,
		CacheWrite5mRatio:           2.625 * ratio.MilliTokensRmb,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-M2.7 high-speed tier with lower latency at 2x base pricing.",
	},
	"MiniMax-M2.5": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             8.4 / 2.1,
		CachedInputRatio:            0.21 * ratio.MilliTokensRmb,
		CacheWrite5mRatio:           2.625 * ratio.MilliTokensRmb,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-M2.5 previous-generation reasoning chat model.",
	},
	"MiniMax-M2.5-highspeed": {
		Ratio:                       4.2 * ratio.MilliTokensRmb,
		CompletionRatio:             16.8 / 4.2,
		CachedInputRatio:            0.21 * ratio.MilliTokensRmb,
		CacheWrite5mRatio:           2.625 * ratio.MilliTokensRmb,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-M2.5 high-speed tier with lower latency at 2x base pricing.",
	},
	"MiniMax-M2.1": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             8.4 / 2.1,
		CachedInputRatio:            0.21 * ratio.MilliTokensRmb,
		CacheWrite5mRatio:           2.625 * ratio.MilliTokensRmb,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-M2.1 legacy reasoning model (kept for backward compatibility).",
	},
	"MiniMax-M2.1-highspeed": {
		Ratio:                       4.2 * ratio.MilliTokensRmb,
		CompletionRatio:             16.8 / 4.2,
		CachedInputRatio:            0.21 * ratio.MilliTokensRmb,
		CacheWrite5mRatio:           2.625 * ratio.MilliTokensRmb,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-M2.1 high-speed tier with lower latency at 2x base pricing.",
	},
	"MiniMax-M2": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             8.4 / 2.1,
		CachedInputRatio:            0.21 * ratio.MilliTokensRmb,
		CacheWrite5mRatio:           2.625 * ratio.MilliTokensRmb,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-M2 reasoning chat model with binary thinking toggle.",
	},
	"M2-her": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             8.4 / 2.1,
		ContextLength:               204800,
		MaxOutputTokens:             32768,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax M2-her dialogue / role-play specialized variant.",
	},

	// abab legacy chat models. Retained for backward compatibility with
	// existing channels; current pricing pages no longer list these but
	// upstream still serves the SKUs to existing keys (historical pricing).
	"abab6.5-chat": {
		Ratio:                       0.03 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               245760,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab6.5 long-context closed-weight chat model (legacy).",
	},
	"abab6.5s-chat": {
		Ratio:                       0.01 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               245760,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab6.5s cost-optimized variant of abab6.5 (legacy).",
	},
	"abab6.5t-chat": {
		Ratio:                       0.005 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab6.5t tiny cost-optimized chat model (legacy).",
	},
	"abab6-chat": {
		Ratio:                       0.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab6 closed-weight chat model with 32k context (legacy).",
	},
	"abab5.5-chat": {
		Ratio:                       0.015 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab5.5 closed-weight chat model (legacy).",
	},
	"abab5.5s-chat": {
		Ratio:                       0.005 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab5.5s cost-optimized chat model (legacy).",
	},

	// MiniMax-VL-01 multimodal (text + image). Pricing estimated based on
	// historical pay-as-you-go disclosures; not on the current paygo page.
	"MiniMax-VL-01": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxVisionInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-VL-01 multimodal chat model accepting text and image inputs (legacy estimated pricing).",
	},
	"MiniMax-Text-01": {
		Ratio:                       0.015 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "MiniMaxAI/MiniMax-Text-01",
		Description:                 "MiniMax-Text-01 long-context chat model with open weights on HuggingFace (legacy estimated pricing).",
	},

	// MiniMax-M1 (legacy MoE reasoning model). Weights open on HuggingFace.
	// Pricing matches the historical announcement at ¥2.8 / ¥16.8 per 1M tokens
	// for the 80k variant; treat as estimated since not on current paygo page.
	"MiniMax-M1": {
		Ratio:                       2.8 * ratio.MilliTokensRmb,
		CompletionRatio:             16.8 / 2.8,
		ContextLength:               1024000,
		MaxOutputTokens:             40960,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxReasoningFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "MiniMaxAI/MiniMax-M1-80k",
		Description:                 "MiniMax-M1 reasoning MoE model with 1M context (open weights, legacy estimated pricing).",
	},

	// embo-01 text embedding model. Pricing held over from historical
	// pay-as-you-go disclosure (not on current paygo page).
	"embo-01": {
		Ratio:            0.0005 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    4096,
		InputModalities:  minimaxTextInputs,
		OutputModalities: minimaxTextOutputs,
		Description:      "MiniMax embo-01 text embedding model (legacy estimated pricing).",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MinimaxToolingDefaults notes that MiniMax's pay-as-you-go pricing reference lists model rates only (no per-tool pricing) as of 2026-05-18.
// Source: https://platform.minimaxi.com/docs/guides/pricing-paygo
var MinimaxToolingDefaults = adaptor.ChannelToolConfig{}
