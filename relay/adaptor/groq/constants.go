package groq

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Reusable metadata fragments. Groq advertises "TruePoint Numerics" which is a
// Groq-proprietary mixed-precision scheme rather than a standard OpenRouter
// quantization label, so the Quantization field is intentionally left empty
// (omitempty) for those models.
//
// Sources (last audited 2026-09-24):
//   - https://console.groq.com/docs/models
//   - https://console.groq.com/docs/deprecations
//   - https://console.groq.com/docs/reasoning
//   - https://console.groq.com/docs/prompt-caching
//   - https://console.groq.com/docs/api-reference
//   - https://console.groq.com/docs/structured-outputs
//   - Per-model pages under https://console.groq.com/docs/model/<id>
var (
	// groqTextOnlyModalities advertises a chat model that consumes and emits text only.
	groqTextOnlyModalities = []string{"text"}
	// groqTextImageInModalities advertises text+image input with text output.
	groqTextImageInModalities = []string{"text", "image"}
	// groqAudioInModalities advertises audio-only input (Whisper STT).
	groqAudioInModalities = []string{"audio"}
	// groqAudioOutModalities advertises audio-only output (TTS).
	groqAudioOutModalities = []string{"audio"}

	// groqChatSamplingParams enumerates the OpenAI-compatible sampling parameters
	// accepted by Groq chat completions. n is restricted to 1; max_tokens is a
	// deprecated alias of max_completion_tokens. Unsupported penalties and
	// log-probability fields must not be advertised.
	// Source: https://console.groq.com/docs/api-reference#chat
	groqChatSamplingParams = []string{
		"temperature",
		"top_p",
		"max_tokens",
		"max_completion_tokens",
		"stop",
		"seed",
		"n",
		"response_format",
		"tools",
		"tool_choice",
	}

	// groqReasoningSamplingParams includes sampling and reasoning controls
	// accepted by Groq; each model defines its own reasoning_effort vocabulary.
	groqReasoningSamplingParams = []string{
		"temperature",
		"top_p",
		"max_tokens",
		"max_completion_tokens",
		"stop",
		"seed",
		"response_format",
		"tools",
		"tool_choice",
		"reasoning_effort",
	}

	// groqReasoningNoEffortSamplingParams is used by reasoning models for
	// which Groq does not publish a discrete reasoning_effort vocabulary.
	groqReasoningNoEffortSamplingParams = []string{
		"temperature",
		"top_p",
		"max_tokens",
		"max_completion_tokens",
		"stop",
		"seed",
		"response_format",
		"tools",
		"tool_choice",
	}

	// groqClassifierSamplingParams covers the minimal parameter set for
	// classifier-style guard models that emit a fixed label set.
	groqClassifierSamplingParams = []string{"max_tokens", "seed"}
)

// ModelRatios contains the current Groq catalog plus retired model IDs kept
// for enterprise committed-spend compatibility and historical billing.
// ModelList follows the published catalog, including enterprise-only entries.
// Legacy metadata does not prohibit administrators from configuring those IDs.
// Pricing and capability source: https://console.groq.com/docs/models
var ModelRatios = map[string]adaptor.ModelConfig{
	// Enterprise production models remain in Groq's supported catalog. Public
	// access ended on 2026-08-16. Preserve historical public rates for billing
	// compatibility, not as current enterprise quotes; configure contract rates.
	"llama-3.3-70b-versatile": {
		Ratio:                       0.59 * ratio.MilliTokensUsd,
		CompletionRatio:             0.79 / 0.59,
		ContextLength:               131072,
		MaxOutputTokens:             32768,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-3.3-70B-Instruct",
		Description:                 "Meta Llama 3.3 70B enterprise production model with 131K context and tool/JSON support. Free/developer access ended on 2026-08-16. Contact sales for pricing; retained historical rates are compatibility defaults, not enterprise quotes. Configure contract rates before use.",
	},
	"llama-3.1-8b-instant": {
		Ratio:                       0.05 * ratio.MilliTokensUsd,
		CompletionRatio:             0.08 / 0.05,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-3.1-8B-Instruct",
		Description:                 "Meta Llama 3.1 8B enterprise production model with 131K context and tool/JSON support. Free/developer access ended on 2026-08-16. Contact sales for pricing; retained historical rates are compatibility defaults, not enterprise quotes. Configure contract rates before use.",
	},
	// Current production models.
	"whisper-large-v3": {
		Ratio:                       0,
		CompletionRatio:             1,
		InputModalities:             groqAudioInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: []string{"language", "prompt", "response_format", "temperature"},
		// Groq publishes Whisper pricing as $0.111 per audio-hour (=$0.111/3600 USD/sec).
		// Source: https://console.groq.com/docs/speech-to-text
		Audio:         &adaptor.AudioPricingConfig{InputUnit: "seconds", InputPriceUsd: 0.111, InputPriceQuantity: 3600, UsdPerSecond: 0.111 / 3600, MinimumBillableSeconds: 10},
		HuggingFaceID: "openai/whisper-large-v3",
		Description:   "OpenAI Whisper large-v3 speech-to-text model (audio input, text output) with 99+ language support.",
	},
	"whisper-large-v3-turbo": {
		Ratio:                       0,
		CompletionRatio:             1,
		InputModalities:             groqAudioInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: []string{"language", "prompt", "response_format", "temperature"},
		// Groq publishes Whisper Turbo pricing as $0.04 per audio-hour.
		// Source: https://console.groq.com/docs/speech-to-text
		Audio:         &adaptor.AudioPricingConfig{InputUnit: "seconds", InputPriceUsd: 0.04, InputPriceQuantity: 3600, UsdPerSecond: 0.04 / 3600, MinimumBillableSeconds: 10},
		HuggingFaceID: "openai/whisper-large-v3-turbo",
		Description:   "Whisper large-v3 turbo multilingual speech-to-text model (audio input, text output). Supports transcription, not translation; audio billing has a 10-second minimum.",
	},
	"openai/gpt-oss-120b": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.075 * ratio.MilliTokensUsd,
		CompletionRatio:             0.60 / 0.15,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		// Groq's reasoning docs explicitly enumerate low/medium/high for gpt-oss models.
		// Source: https://console.groq.com/docs/reasoning
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		HuggingFaceID:             "openai/gpt-oss-120b",
		Description:               "OpenAI's 120B open-weight Mixture-of-Experts reasoning model with built-in web search and code execution.",
	},
	"openai/gpt-oss-20b": {
		Ratio:                       0.075 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.0375 * ratio.MilliTokensUsd,
		CompletionRatio:             0.30 / 0.075,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		// Groq's reasoning docs explicitly enumerate low/medium/high for gpt-oss models.
		// Source: https://console.groq.com/docs/reasoning
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		HuggingFaceID:             "openai/gpt-oss-20b",
		Description:               "OpenAI's 20B open-weight MoE reasoning model with browser search and code execution support.",
	},

	// Retired preview ID. Groq removed it from free and developer plans on
	// 2026-07-17; committed-spend enterprise access may continue.
	"meta-llama/llama-4-scout-17b-16e-instruct": {
		Ratio:                       0.11 * ratio.MilliTokensUsd,
		CompletionRatio:             0.34 / 0.11,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             groqTextImageInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-4-Scout-17B-16E-Instruct",
		Description:                 "Meta Llama 4 Scout (17B activated MoE) multimodal model with early-fusion image understanding. RETIRED from free and developer plans on 2026-07-17; committed-spend enterprise access may continue. Migrate to openai/gpt-oss-120b or qwen/qwen3.8-27b.",
	},
	// Current preview models.
	"meta-llama/llama-prompt-guard-2-22m": {
		Ratio:                       0.03 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               512,
		MaxOutputTokens:             512,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: groqClassifierSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-Prompt-Guard-2-22M",
		Description:                 "22M-parameter classifier that flags prompt injection and jailbreak attempts in real time.",
	},
	"meta-llama/llama-prompt-guard-2-86m": {
		Ratio:                       0.04 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               512,
		MaxOutputTokens:             512,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: groqClassifierSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-Prompt-Guard-2-86M",
		Description:                 "86M-parameter multilingual classifier (mDeBERTa) detecting prompt injections across 8 languages.",
	},
	"openai/gpt-oss-safeguard-20b": {
		Ratio:                       0.075 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.0375 * ratio.MilliTokensUsd,
		CompletionRatio:             0.30 / 0.075,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		// The safeguard model card recommends low/high effort for simple versus
		// nuanced classifications.
		// Source: https://console.groq.com/docs/model/openai/gpt-oss-safeguard-20b
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		HuggingFaceID:             "openai/gpt-oss-safeguard-20b",
		Description:               "20B GPT-OSS variant for policy-following safety classification with custom taxonomies, prompt caching, browser search, and code execution.",
	},
	"qwen/qwen3-32b": {
		Ratio:                       0.29 * ratio.MilliTokensUsd,
		CompletionRatio:             0.59 / 0.29,
		ContextLength:               131072,
		MaxOutputTokens:             40960,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		// Groq's legacy Qwen3 reasoning docs enumerate none/default.
		// Source: https://console.groq.com/docs/reasoning
		SupportedReasoningEfforts: []string{"none", "default"},
		DefaultReasoningEffort:    "default",
		HuggingFaceID:             "Qwen/Qwen3-32B",
		Description:               "Alibaba Qwen3-32B with switchable thinking/non-thinking modes for reasoning and general dialog. RETIRED from free and developer plans on 2026-07-17; committed-spend enterprise access may continue. Migrate to openai/gpt-oss-120b.",
	},
	"qwen/qwen3.6-27b": {
		Ratio:                       0.60 * ratio.MilliTokensUsd,
		CompletionRatio:             3.00 / 0.60,
		ContextLength:               131072,
		MaxOutputTokens:             16384,
		InputModalities:             groqTextImageInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		// Groq publishes only none/default for Qwen 3.6 reasoning_effort.
		// Source: https://console.groq.com/docs/reasoning
		SupportedReasoningEfforts: []string{"none", "default"},
		DefaultReasoningEffort:    "default",
		HuggingFaceID:             "Qwen/Qwen3.6-27B",
		Description:               "Alibaba Qwen3.6-27B legacy multimodal model. RETIRED from free and developer plans on 2026-09-14; committed-spend enterprise contracts are unaffected. Historical pricing and none/default reasoning controls are retained for compatibility. Migrate public usage to qwen/qwen3.8-27b.",
	},
	// Current multimodal preview. Groq's API contract is authoritative for the
	// effort default; do not copy conflicting upstream/model-card defaults.
	// Source: https://console.groq.com/docs/api-reference#chat
	"qwen/qwen3.8-27b": {
		Ratio:                       0.80 * ratio.MilliTokensUsd,
		CompletionRatio:             4.00 / 0.80,
		ContextLength:               131072,
		MaxOutputTokens:             16384,
		InputModalities:             groqTextImageInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		SupportedReasoningEfforts:   []string{"none", "default", "low", "medium", "high"},
		DefaultReasoningEffort:      "none",
		HuggingFaceID:               "Qwen/Qwen3.8-27B",
		Description:                 "Alibaba Qwen3.8-27B multimodal preview with 131K context, 16K maximum output, tool calling, and strict/best-effort structured outputs. Accepts text and up to 3 images. Groq reasoning_effort accepts none/default/low/medium/high and defaults to none; no discounted prompt caching is published.",
	},
	"minimaxai/minimax-m2.7": {
		Ratio:                       0,
		CompletionRatio:             1,
		ContextLength:               196608,
		MaxOutputTokens:             131072,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: groqReasoningNoEffortSamplingParams,
		HuggingFaceID:               "MiniMaxAI/MiniMax-M2.7",
		Description:                 "MiniMax M2.7 enterprise preview reasoning model with 196K context and 131K maximum output. Pricing is contact-sales only: the zero placeholder is not a free tariff. Configure contract rates before use.",
	},

	// Current preview text-to-speech models.
	"canopylabs/orpheus-arabic-saudi": {
		Audio:            &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceUsd: 40, InputPriceQuantity: 1e6},
		Ratio:            40.0 * ratio.MilliTokensUsd, // per 1M characters
		CompletionRatio:  1,
		ContextLength:    4000,
		MaxOutputTokens:  50000,
		InputModalities:  groqTextOnlyModalities,
		OutputModalities: groqAudioOutModalities,
		Description:      "Canopy Labs Orpheus text-to-speech model (Saudi Arabic). Output is rendered audio billed per character.",
	},
	"canopylabs/orpheus-v1-english": {
		Audio:            &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceUsd: 22, InputPriceQuantity: 1e6},
		Ratio:            22.0 * ratio.MilliTokensUsd, // per 1M characters
		CompletionRatio:  1,
		ContextLength:    4000,
		MaxOutputTokens:  50000,
		InputModalities:  groqTextOnlyModalities,
		OutputModalities: groqAudioOutModalities,
		HuggingFaceID:    "canopylabs/orpheus-3b-0.1-ft",
		Description:      "Canopy Labs Orpheus v1 English text-to-speech (Llama-3.2-3B backbone) with bracketed vocal direction tags.",
	},
	// Decommissioned systems. Keep historical metadata, but do not advertise
	// these IDs: Groq returns errors for them starting 2026-09-21.
	"groq/compound": {
		Ratio:                       0,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "web_search"},
		SupportedSamplingParameters: groqChatSamplingParams,
		Description:                 "DECOMMISSIONED on 2026-09-21: Groq Compound requests now return errors. Historical metadata only; underlying-model and tool usage never had a standalone token rate.",
	},
	"groq/compound-mini": {
		Ratio:                       0,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "web_search"},
		SupportedSamplingParameters: groqChatSamplingParams,
		Description:                 "DECOMMISSIONED on 2026-09-21: Groq Compound Mini requests now return errors. Historical metadata only; underlying-model and tool usage never had a standalone token rate.",
	},
}

// ModelList mirrors Groq's supported catalog as of 2026-09-24, including
// enterprise-only models. Account permissions belong to channel administrators.
// Legacy IDs not listed here remain configurable through their retained metadata.
var ModelList = []string{
	"llama-3.1-8b-instant",
	"llama-3.3-70b-versatile",
	"openai/gpt-oss-120b",
	"openai/gpt-oss-20b",
	"whisper-large-v3",
	"whisper-large-v3-turbo",
	"canopylabs/orpheus-arabic-saudi",
	"canopylabs/orpheus-v1-english",
	"meta-llama/llama-prompt-guard-2-22m",
	"meta-llama/llama-prompt-guard-2-86m",
	"minimaxai/minimax-m2.7",
	"openai/gpt-oss-safeguard-20b",
	"qwen/qwen3.8-27b",
}

// GroqToolingDefaults preserves the empty tool-tariff configuration rather
// than inventing per-call prices. Compound systems were decommissioned on
// 2026-09-21; this value does not advertise their availability.
// Source: https://console.groq.com/docs/deprecations
var GroqToolingDefaults = adaptor.ChannelToolConfig{}
