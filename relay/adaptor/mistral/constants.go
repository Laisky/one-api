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
// by the Magistral family. Reference:
// https://docs.mistral.ai/capabilities/reasoning/
var reasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Official sources:
// - https://docs.mistral.ai/getting-started/models/models_overview/
// - https://docs.mistral.ai/resources/changelogs
// - https://mistral.ai/pricing
// - model cards under https://docs.mistral.ai/models/model-cards/
var ModelRatios = map[string]adaptor.ModelConfig{
	"mistral-medium-latest": {
		Ratio:                       0.4 * ratio.MilliTokensUsd, // $0.4 input
		CompletionRatio:             5.0,                        // $2 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Medium frontier-class multimodal chat model (alias for the latest medium release).",
	},
	"mistral-medium-2508": {
		Ratio:                       0.4 * ratio.MilliTokensUsd,
		CompletionRatio:             5.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Medium 3.1 (2025-08) multimodal chat model with 128k context.",
	},
	"magistral-medium-latest": {
		Ratio:                       2.0 * ratio.MilliTokensUsd, // $2 input
		CompletionRatio:             2.5,                        // $5 output
		ContextLength:               40960,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Magistral Medium reasoning model (alias for the latest release).",
	},
	"magistral-medium-2509": {
		Ratio:                       2.0 * ratio.MilliTokensUsd,
		CompletionRatio:             2.5,
		ContextLength:               40960,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Magistral Medium reasoning model snapshot 2509.",
	},
	"devstral-2512": {
		Ratio:                       0.4 * ratio.MilliTokensUsd,
		CompletionRatio:             5.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Devstral 2025-12 software engineering / agentic coding model.",
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
		Description:                 "Devstral Medium 2025-07 agentic coding model.",
	},
	"codestral-latest": {
		Ratio:                       0.3 * ratio.MilliTokensUsd, // $0.3 input
		CompletionRatio:             3.0,                        // $0.9 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Codestral code-completion model (alias for the latest release).",
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
		Description:                 "Codestral 2025-08 code-completion model with 256k context.",
	},
	"mistral-large-latest": {
		Ratio:                       0.5 * ratio.MilliTokensUsd, // $0.5 input
		CompletionRatio:             3.0,                        // $1.5 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Large flagship dense chat model (alias for the latest release).",
	},
	"mistral-large-2512": {
		Ratio:                       0.5 * ratio.MilliTokensUsd,
		CompletionRatio:             3.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Large 2025-12 dense chat model with 128k context.",
	},
	"pixtral-large-latest": {
		Ratio:                       2.0 * ratio.MilliTokensUsd, // $2 input
		CompletionRatio:             3.0,                        // $6 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Pixtral Large 124B multimodal vision-language model.",
	},
	"mistral-saba-latest": {
		Ratio:                       0.2 * ratio.MilliTokensUsd, // $0.2 input
		CompletionRatio:             3.0,                        // $0.6 output
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Saba regional model tuned for Middle East and South Asian languages.",
	},
	"mistral-small-latest": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input
		CompletionRatio:             4.0,                         // $0.6 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Small-3.2-24B-Instruct-2506",
		Description:                 "Mistral Small 3.2 24B open-weight multimodal chat model (alias for the latest small release).",
	},
	"magistral-small-latest": {
		Ratio:                       0.5 * ratio.MilliTokensUsd, // $0.5 input
		CompletionRatio:             3.0,                        // $1.5 output
		ContextLength:               40960,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Magistral-Small-2509",
		Description:                 "Magistral Small open-weight reasoning model (alias for the latest release).",
	},
	"magistral-small-2509": {
		Ratio:                       0.5 * ratio.MilliTokensUsd,
		CompletionRatio:             3.0,
		ContextLength:               40960,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Magistral-Small-2509",
		Description:                 "Magistral Small 2025-09 open-weight reasoning model.",
	},
	"devstral-small-2507": {
		Ratio:                       0.1 * ratio.MilliTokensUsd, // $0.1 input
		CompletionRatio:             3.0,                        // $0.3 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Devstral-Small-2507",
		Description:                 "Devstral Small 2025-07 open-weight agentic coding model.",
	},
	"pixtral-12b": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input
		CompletionRatio:             1.0,                         // $0.15 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Pixtral-12B-2409",
		Description:                 "Pixtral 12B open-weight multimodal vision-language model.",
	},
	"open-mistral-7b": {
		Ratio:                       0.25 * ratio.MilliTokensUsd, // $0.25 input
		CompletionRatio:             1.0,                         // $0.25 output
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-7B-Instruct-v0.3",
		Description:                 "Mistral 7B Instruct v0.3 open-weight dense chat model.",
	},
	"open-mixtral-8x7b": {
		Ratio:                       0.7 * ratio.MilliTokensUsd, // $0.7 input
		CompletionRatio:             1.0,                        // $0.7 output
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mixtral-8x7B-Instruct-v0.1",
		Description:                 "Mixtral 8x7B Instruct open-weight sparse MoE chat model.",
	},
	"open-mixtral-8x22b": {
		Ratio:                       2.0 * ratio.MilliTokensUsd, // $2 input
		CompletionRatio:             3.0,                        // $6 output
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mixtral-8x22B-Instruct-v0.1",
		Description:                 "Mixtral 8x22B Instruct open-weight sparse MoE chat model.",
	},
	"ministral-14b-2512": {
		Ratio:                       0.2 * ratio.MilliTokensUsd, // $0.2 input
		CompletionRatio:             1.0,                        // $0.2 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Ministral 14B 2025-12 edge chat model with 128k context.",
	},
	"ministral-8b-latest": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input
		CompletionRatio:             1.0,                         // $0.15 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Ministral 8B edge chat model (alias for the latest release).",
	},
	"ministral-8b-2512": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Ministral 8B 2025-12 edge chat model with 128k context.",
	},
	"ministral-3b-latest": {
		Ratio:                       0.1 * ratio.MilliTokensUsd, // $0.1 input
		CompletionRatio:             1.0,                        // $0.1 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Ministral 3B edge chat model (alias for the latest release).",
	},
	"ministral-3b-2512": {
		Ratio:                       0.1 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Ministral 3B 2025-12 edge chat model with 128k context.",
	},

	// Embedding Models
	"mistral-embed": {
		Ratio:           0.1 * ratio.MilliTokensUsd, // $0.1 input only
		CompletionRatio: 1.0,
		ContextLength:   8192,
		InputModalities: textOnlyModalities,
		Description:     "Mistral Embed text embedding model with 8k context.",
	},
	"codestral-embed-2505": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 input only
		CompletionRatio: 1.0,
		ContextLength:   8192,
		InputModalities: textOnlyModalities,
		Description:     "Codestral Embed 2025-05 code embedding model.",
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
