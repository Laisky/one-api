package mistral

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// commonSamplingParams enumerates OpenAI-compatible sampling parameters accepted
// by Mistral chat-completion models. Reference:
// https://docs.mistral.ai/api/#tag/chat
var commonSamplingParams = []string{
	"temperature",
	"top_p",
	"stop",
	"seed",
	"max_tokens",
	"frequency_penalty",
	"presence_penalty",
}

// textOnlyModalities is the default modality set for text-only chat models.
var textOnlyModalities = []string{"text"}

// multimodalInputModalities is the input modality set for vision-capable models.
var multimodalInputModalities = []string{"text", "image"}

// chatFeatures is the feature subset advertised by every Mistral chat model.
// All currently published Mistral chat models support OpenAI-style tool calling,
// JSON-mode responses, and structured outputs. Reference:
// https://docs.mistral.ai/capabilities/function_calling/ and
// https://docs.mistral.ai/capabilities/structured-output/json_mode/
var chatFeatures = []string{"tools", "json_mode", "structured_outputs"}

// reasoningFeatures extends chatFeatures with the reasoning capability advertised
// by the Magistral family and hybrid reasoning models (Mistral Small 4,
// Mistral Medium 3.5). Reference:
// https://docs.mistral.ai/capabilities/reasoning/
var reasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

// magistralReasoningEfforts enumerates the reasoning_effort values accepted by
// reasoning-capable Mistral models. The platform exposes the OpenAI-compatible
// low/medium/high vocabulary. Reference:
// https://docs.mistral.ai/capabilities/reasoning/
var magistralReasoningEfforts = []string{"low", "medium", "high"}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Official sources:
// - https://docs.mistral.ai/getting-started/models/models_overview/
// - https://docs.mistral.ai/getting-started/changelog
// - https://mistral.ai/pricing
// - model cards under https://docs.mistral.ai/models/model-cards/
var ModelRatios = map[string]adaptor.ModelConfig{
	// --- Mistral Medium family ---
	"mistral-medium-latest": {
		Ratio:                       1.5 * ratio.MilliTokensUsd, // $1.50 input (Medium 3.5)
		CompletionRatio:             5.0,                        // $7.50 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Description:                 "Mistral Medium frontier-class multimodal chat model (alias for the latest medium release; currently Medium 3.5).",
	},
	"mistral-medium-2604": {
		Ratio:                       1.5 * ratio.MilliTokensUsd, // $1.50 input
		CompletionRatio:             5.0,                        // $7.50 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Description:                 "Mistral Medium 3.5 (2026-04) multimodal model with tunable reasoning_effort and 256k context. Equivalent API alias: mistral-medium-3-5.",
	},
	"mistral-medium-2508": {
		Ratio:                       0.4 * ratio.MilliTokensUsd, // $0.40 input
		CompletionRatio:             5.0,                        // $2.00 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Medium 3.1 (2025-08) multimodal chat model with 128k context.",
	},

	// --- Magistral reasoning family ---
	"magistral-medium-latest": {
		Ratio:                       2.0 * ratio.MilliTokensUsd, // $2 input
		CompletionRatio:             2.5,                        // $5 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Description:                 "Magistral Medium reasoning model (alias for the latest release).",
	},
	"magistral-medium-2509": {
		Ratio:                       2.0 * ratio.MilliTokensUsd,
		CompletionRatio:             2.5,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Description:                 "Magistral Medium 1.2 reasoning model snapshot 2509 with native vision and tool use.",
	},

	// --- Devstral / coding agent family ---
	"devstral-2512": {
		Ratio:                       0.4 * ratio.MilliTokensUsd, // $0.40 input
		CompletionRatio:             5.0,                        // $2.00 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Devstral-2-123B-Instruct-2512",
		Description:                 "Devstral 2 (2025-12) 123B agentic coding model with 256k context and image input.",
	},
	"devstral-medium-2507": {
		Ratio:                       0.4 * ratio.MilliTokensUsd,
		CompletionRatio:             5.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Devstral Medium 2025-07 agentic coding model. Deprecated 2026-02-27; retires 2026-05-31.",
	},

	// --- Codestral / code completion family ---
	"codestral-latest": {
		Ratio:                       0.3 * ratio.MilliTokensUsd, // $0.30 input
		CompletionRatio:             3.0,                        // $0.90 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Codestral code-completion model (alias for the latest release; currently codestral-2508).",
	},
	"codestral-2508": {
		Ratio:                       0.3 * ratio.MilliTokensUsd,
		CompletionRatio:             3.0,
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Codestral 2025-08 code-completion model with 256k context and FIM support.",
	},

	// --- Voxtral audio family (speech-to-text, audio-conditioned chat, text-to-speech) ---
	// Reference: https://docs.mistral.ai/models/overview and
	// https://docs.mistral.ai/models/voxtral-mini-transcribe-26-02
	"voxtral-small-2507": {
		Ratio:                       0.1 * ratio.MilliTokensUsd, // $0.10 input
		CompletionRatio:             3.0,                        // $0.30 output
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Voxtral-Small-24B-2507",
		Description:                 "Voxtral Small 24B (2025-07) open-weight audio-conditioned instruct model with 32k context.",
	},
	"voxtral-mini-transcribe-2602": {
		Ratio:           0.1 * ratio.MilliTokensUsd, // fallback token billing
		CompletionRatio: 1.0,
		ContextLength:   32768,
		InputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			UsdPerSecond: 0.003 / 60.0, // $0.003 per audio minute
		},
		Description: "Voxtral Mini Transcribe 2 (2026-02) low-latency speech-to-text model at $0.003 per audio minute.",
	},
	"voxtral-tts-2603": {
		// Voxtral TTS bills at $16 per million characters ($0.016/1k chars).
		// We surface the per-character rate via the token ratio so prompt billing
		// fires consistently when callers pass text input.
		Ratio:            16.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    8192,
		InputModalities:  textOnlyModalities,
		OutputModalities: []string{"audio"},
		Description:      "Voxtral TTS (2026-03) zero-shot voice cloning text-to-speech model at $16 per million input characters.",
	},

	// --- Mistral Large family ---
	"mistral-large-latest": {
		Ratio:                       0.5 * ratio.MilliTokensUsd, // $0.50 input (Large 3)
		CompletionRatio:             3.0,                        // $1.50 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Large flagship multimodal MoE model (alias for the latest release; currently Large 3).",
	},
	"mistral-large-2512": {
		Ratio:                       0.5 * ratio.MilliTokensUsd,
		CompletionRatio:             3.0,
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Large-3-675B-Instruct-2512",
		Description:                 "Mistral Large 3 (2025-12) 675B sparse-MoE multimodal model with 256k context.",
	},

	// --- Pixtral / multimodal vision family ---
	"pixtral-large-latest": {
		Ratio:                       2.0 * ratio.MilliTokensUsd, // $2 input
		CompletionRatio:             3.0,                        // $6 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Pixtral Large 124B multimodal vision-language model (alias for pixtral-large-2411). Deprecated 2026-02-27; retires 2026-05-31.",
	},
	"pixtral-large-2411": {
		Ratio:                       2.0 * ratio.MilliTokensUsd,
		CompletionRatio:             3.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Pixtral-Large-Instruct-2411",
		Description:                 "Pixtral Large 2024-11 124B open-weight multimodal vision-language model. Deprecated 2026-02-27; retires 2026-05-31.",
	},

	// --- Mistral Small family ---
	"mistral-small-latest": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input (Small 4)
		CompletionRatio:             4.0,                         // $0.60 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Small-4-119B-2603",
		Description:                 "Mistral Small hybrid reasoning/coding multimodal model (alias for the latest release; currently Small 4).",
	},
	"mistral-small-2603": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input
		CompletionRatio:             4.0,                         // $0.60 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Small-4-119B-2603",
		Description:                 "Mistral Small 4 (2026-03) 119B MoE hybrid model with reasoning, vision, and 256k context.",
	},
	"mistral-small-2506": {
		Ratio:                       0.1 * ratio.MilliTokensUsd, // $0.10 input
		CompletionRatio:             3.0,                        // $0.30 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Small-3.2-24B-Instruct-2506",
		Description:                 "Mistral Small 3.2 24B open-weight multimodal chat model snapshot 2506. Deprecated 2026-04-30; retires 2026-07-31.",
	},
	"magistral-small-latest": {
		Ratio:                       0.5 * ratio.MilliTokensUsd, // $0.50 input
		CompletionRatio:             3.0,                        // $1.50 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Magistral-Small-2509",
		Description:                 "Magistral Small open-weight reasoning model (alias for the latest release).",
	},
	"magistral-small-2509": {
		Ratio:                       0.5 * ratio.MilliTokensUsd,
		CompletionRatio:             3.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		SupportedReasoningEfforts:   magistralReasoningEfforts,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Magistral-Small-2509",
		Description:                 "Magistral Small 2025-09 open-weight reasoning model with vision support.",
	},
	"devstral-small-2507": {
		Ratio:                       0.1 * ratio.MilliTokensUsd, // $0.10 input
		CompletionRatio:             3.0,                        // $0.30 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Devstral-Small-2507",
		Description:                 "Devstral Small 2025-07 open-weight agentic coding model. Deprecated 2026-02-27; retires 2026-05-31.",
	},
	// Note: devstral-small-2512 / pixtral-12b / open-mistral-7b / open-mixtral-8x7b /
	// open-mixtral-8x22b / mistral-saba-* are all retired per the Mistral legacy table
	// (https://docs.mistral.ai/getting-started/models/models_overview/#legacy-deprecated)
	// and have been removed.

	"open-mistral-nemo": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input
		CompletionRatio:             1.0,                         // $0.15 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Nemo-Instruct-2407",
		Description:                 "Mistral Nemo 12B open-weight dense chat model co-developed with NVIDIA.",
	},

	// --- Ministral 3 edge family (December 2025) ---
	"ministral-14b-2512": {
		Ratio:                       0.2 * ratio.MilliTokensUsd, // $0.20 input
		CompletionRatio:             1.0,                        // $0.20 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Ministral-3-14B-Instruct-2512",
		Description:                 "Ministral 3 14B (2025-12) edge multimodal model with 256k context.",
	},
	"ministral-8b-latest": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input
		CompletionRatio:             1.0,                         // $0.15 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Ministral-3-8B-Instruct-2512",
		Description:                 "Ministral 8B edge multimodal chat model (alias for the latest release).",
	},
	"ministral-8b-2512": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Ministral-3-8B-Instruct-2512",
		Description:                 "Ministral 3 8B (2025-12) edge multimodal model with 256k context.",
	},
	"ministral-3b-latest": {
		Ratio:                       0.1 * ratio.MilliTokensUsd, // $0.10 input
		CompletionRatio:             1.0,                        // $0.10 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Ministral-3-3B-Instruct-2512",
		Description:                 "Ministral 3B edge multimodal chat model (alias for the latest release).",
	},
	"ministral-3b-2512": {
		Ratio:                       0.1 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Ministral-3-3B-Instruct-2512",
		Description:                 "Ministral 3 3B (2025-12) compact edge multimodal model with 256k context.",
	},

	// --- Embedding Models ---
	"mistral-embed": {
		Ratio:           0.1 * ratio.MilliTokensUsd, // $0.10 input only
		CompletionRatio: 1.0,
		ContextLength:   8192,
		InputModalities: textOnlyModalities,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.1 * ratio.MilliTokensUsd,
		},
		Description: "Mistral Embed text embedding model with 8k context.",
	},
	"codestral-embed-2505": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 input only
		CompletionRatio: 1.0,
		ContextLength:   8192,
		InputModalities: textOnlyModalities,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.15 * ratio.MilliTokensUsd,
		},
		Description: "Codestral Embed 2025-05 code embedding model with 8k context.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MistralToolingDefaults preserves legacy tool defaults.
// Mistral's official model overview, changelog, and accessible model-card pages do not
// currently publish equivalent per-tool invocation pricing, so these values remain unchanged.
var MistralToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"code_execution":     {UsdPerCall: 0.01}, // Connectors and code tools: $0.01 per API call
		"document_library":   {UsdPerCall: 0.01}, // Document library billed under connector rate
		"image_generation":   {UsdPerCall: 0.10}, // $100 per 1K images
		"web_search":         {UsdPerCall: 0.03}, // Web search / knowledge plugins: $30 per 1K queries
		"web_search_premium": {UsdPerCall: 0.03},
	},
}
