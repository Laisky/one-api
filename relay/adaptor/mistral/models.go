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
// Hosted tariffs and lifecycle sources checked 2026-09-24 through 2026-11-16:
// https://docs.mistral.ai/inference/pricing
// https://docs.mistral.ai/models/zai-glm-5-2
// https://docs.mistral.ai/models/zai-glm-5-3
// https://docs.mistral.ai/models/zai-glm-5-3-flash
// https://docs.mistral.ai/models/deepseek-v4-flash-1017
// https://docs.mistral.ai/models/leanstral-1-5
// https://docs.mistral.ai/resources/changelogs
// https://docs.mistral.ai/inference/conversations/reasoning
var ModelRatios = adaptor.JoinModelCatalogs(map[string]adaptor.ModelConfig{

	// codestral models.
	"codestral-2508": {
		Ratio:                       0.3 * ratio.MilliTokensUsd,
		CompletionRatio:             3.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Codestral 2025-08 code-completion model with 128k context and FIM support.",
		CachedInputRatio:            .03 * ratio.MilliTokensUsd,
	},
	"codestral-embed-2505": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 input only
		CompletionRatio: 1.0,
		ContextLength:   8192,
		InputModalities: textOnlyModalities,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.15 * ratio.MilliTokensUsd,
		},
		Description:      "Codestral Embed 2025-05 code embedding model with 8k context.",
		CachedInputRatio: .015 * ratio.MilliTokensUsd,
	},
	"codestral-latest": {
		Ratio:                       0.3 * ratio.MilliTokensUsd, // $0.30 input
		CompletionRatio:             3.0,                        // $0.90 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Codestral code-completion model (alias for the latest release; currently codestral-2508).",
		CachedInputRatio:            .03 * ratio.MilliTokensUsd,
	},

	// deepseek models.
	"deepseek-v4-1-flash-1017": {
		Ratio: 0.30 * ratio.MilliTokensUsd, CachedInputRatio: 0.007 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30,
		ContextLength: 1_000_000, MaxOutputTokens: 128_000,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice"},
		Description:                 "Mistral-hosted DeepSeek V4.1 Flash 1017 preview, released 2026-10-28, with text/image input, 1M context and 128K maximum output. Exact API ID includes v4-1 even though the model-card URL uses v4. Mistral USD/M input/cache/output: 0.30/0.007/1.20. No undocumented reasoning-effort vocabulary or checkpoint precision is inferred.",
	},

	// devstral models.
	"devstral-2512": {
		Ratio:                       0.4 * ratio.MilliTokensUsd, // $0.40 input
		CompletionRatio:             5.0,                        // $2.00 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Devstral-2-123B-Instruct-2512",
		Description:                 "Devstral 2 (2025-12) 123B agentic coding (text-to-text) model with 256k context. Deprecated 2026-05-22; retires 2026-07-31 (replaced by Mistral Medium 3.5).",
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
		Description:                 "Devstral Medium 2025-07 agentic coding model. Deprecated 2026-02-27; RETIRED 2026-05-31.",
	},
	"devstral-medium-latest": {
		Ratio:                       0.4 * ratio.MilliTokensUsd, // $0.40 input
		CompletionRatio:             5.0,                        // $2.00 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             textOnlyModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Devstral-2-123B-Instruct-2512",
		Description:                 "Devstral Medium agentic coding model (alias for the latest release; currently devstral-2512).",
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
		Description:                 "Devstral Small 2025-07 open-weight agentic coding model. Deprecated 2026-02-27; RETIRED 2026-05-31.",
	},
	"devstral-small-latest": {
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
		Description:                 "Devstral Small agentic coding model (alias for the latest release; currently devstral-small-2507).",
	},

	// labs models.
	"labs-leanstral-1-5": {
		Ratio:             0,
		CompletionRatio:   1,
		ContextLength:     262_144,
		MaxOutputTokens:   128_000,
		InputModalities:   []string{"text"},
		OutputModalities:  []string{"text"},
		SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: []string{
			"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice",
		},
		Description: "Retired on 2026-09-30. Leanstral 1.5 was a Labs code agent for Lean 4 formal proof engineering with 256K context and 128K maximum output. The historical free tariff is retained for existing configurations, not a promise of current upstream availability.",
	},

	// magistral models.
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
		Description:                 "Magistral Medium 1.2 reasoning model snapshot 2509 with native vision and tool use. Deprecated 2026-05-22; retires 2026-07-31 (replaced by Mistral Medium 3.5).",
	},
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
		Description:                 "Magistral Medium reasoning model (alias for the latest release; currently magistral-medium-2509, which is deprecated 2026-05-22 and retires 2026-07-31, replaced by Mistral Medium 3.5).",
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
		Description:                 "Magistral Small 2025-09 open-weight reasoning model with vision support. Deprecated 2026-04-30; retires 2026-07-31 (replaced by Mistral Small 4).",
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
		Description:                 "Magistral Small open-weight reasoning model (alias for the latest release; currently magistral-small-2509, which is deprecated 2026-04-30 and retires 2026-07-31, replaced by Mistral Small 4).",
	},

	// ministral models.
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
		CachedInputRatio:            .02 * ratio.MilliTokensUsd,
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
		CachedInputRatio:            .01 * ratio.MilliTokensUsd,
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
		CachedInputRatio:            .01 * ratio.MilliTokensUsd,
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
		CachedInputRatio:            .015 * ratio.MilliTokensUsd,
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
		CachedInputRatio:            .015 * ratio.MilliTokensUsd,
	},

	// mistral models.
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
		CachedInputRatio:            .05 * ratio.MilliTokensUsd,
	},
	"mistral-large-4":      large4ModelConfig(),
	"mistral-large-4-0":    large4ModelConfig(),
	"mistral-large-latest": large4LatestModelConfig(),
	"mistral-medium-2508": {
		Ratio:                       0.4 * ratio.MilliTokensUsd, // $0.40 input
		CompletionRatio:             5.0,                        // $2.00 output
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: commonSamplingParams,
		Description:                 "Mistral Medium 3.1 (2025-08) multimodal chat model with 128k context. Deprecated 2026-05-22; retires 2026-08-31 (replaced by Mistral Medium 3.5).",
	},
	"mistral-medium-2604": {
		Ratio:                       1.5 * ratio.MilliTokensUsd, // $1.50 input
		CompletionRatio:             5.0,                        // $7.50 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "seed", "max_tokens", "frequency_penalty", "presence_penalty", "reasoning_effort"},
		SupportedReasoningEfforts:   []string{"none", "high"},
		Description:                 "Mistral Medium 3.5 (2026-04) multimodal model with tunable reasoning_effort and 256k context. Equivalent API alias: mistral-medium-3-5.",
		CachedInputRatio:            .15 * ratio.MilliTokensUsd,
	},
	"mistral-medium-3-5": {
		Ratio:                       1.5 * ratio.MilliTokensUsd, // $1.50 input
		CompletionRatio:             5.0,                        // $7.50 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "seed", "max_tokens", "frequency_penalty", "presence_penalty", "reasoning_effort"},
		SupportedReasoningEfforts:   []string{"none", "high"},
		Description:                 "Mistral Medium 3.5 official dated model ID (equivalent to mistral-medium-2604) with tunable reasoning_effort and 256k context.",
		CachedInputRatio:            .15 * ratio.MilliTokensUsd,
	},
	"mistral-medium-latest": {
		Ratio:                       1.5 * ratio.MilliTokensUsd, // $1.50 input (Medium 3.5)
		CompletionRatio:             5.0,                        // $7.50 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "seed", "max_tokens", "frequency_penalty", "presence_penalty", "reasoning_effort"},
		SupportedReasoningEfforts:   []string{"none", "high"},
		Description:                 "Mistral Medium frontier-class multimodal chat model (alias for the latest medium release; currently Medium 3.5).",
		CachedInputRatio:            .15 * ratio.MilliTokensUsd,
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
	"mistral-small-2603": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input
		CompletionRatio:             4.0,                         // $0.60 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "seed", "max_tokens", "frequency_penalty", "presence_penalty", "reasoning_effort"},
		SupportedReasoningEfforts:   []string{"none", "high"},
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Small-4-119B-2603",
		Description:                 "Mistral Small 4 (2026-03) 119B MoE hybrid model with reasoning, vision, and 256k context.",
		CachedInputRatio:            .015 * ratio.MilliTokensUsd,
	},
	"mistral-small-latest": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 input (Small 4)
		CompletionRatio:             4.0,                         // $0.60 output
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             multimodalInputModalities,
		OutputModalities:            textOnlyModalities,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "seed", "max_tokens", "frequency_penalty", "presence_penalty", "reasoning_effort"},
		SupportedReasoningEfforts:   []string{"none", "high"},
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-Small-4-119B-2603",
		Description:                 "Mistral Small hybrid reasoning/coding multimodal model (alias for the latest release; currently Small 4).",
		CachedInputRatio:            .015 * ratio.MilliTokensUsd,
	},

	// open models.
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

	// pixtral models.
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
		Description:                 "Pixtral Large 2024-11 124B open-weight multimodal vision-language model. Deprecated 2026-02-27; RETIRED 2026-05-31.",
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
		Description:                 "Pixtral Large 124B multimodal vision-language model (alias for pixtral-large-2411). Deprecated 2026-02-27; RETIRED 2026-05-31.",
	},

	// voxtral models.
	"voxtral-mini-2602": {
		Ratio:           0.1 * ratio.MilliTokensUsd, // fallback token billing
		CompletionRatio: 1.0,
		ContextLength:   32768,
		InputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			InputUnit: "seconds", InputPriceUsd: 0.003, InputPriceQuantity: 60,
			UsdPerSecond: 0.003 / 60.0, // $0.003 per audio minute
		},
		Description: "Voxtral Mini Transcribe 2 (2026-02) low-latency speech-to-text model at $0.003 per audio minute.",
	},
	"voxtral-mini-transcribe-2602": {
		Ratio:           0.1 * ratio.MilliTokensUsd, // fallback token billing
		CompletionRatio: 1.0,
		ContextLength:   32768,
		InputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			InputUnit: "seconds", InputPriceUsd: 0.003, InputPriceQuantity: 60,
			UsdPerSecond: 0.003 / 60.0, // $0.003 per audio minute
		},
		Description: "Voxtral Mini Transcribe 2 (2026-02) low-latency speech-to-text model at $0.003 per audio minute.",
	},
	"voxtral-mini-transcribe-3":      voxtralTranscribeConfig(0.0035),
	"voxtral-mini-transcribe-3-0":    voxtralTranscribeConfig(0.0035),
	"voxtral-mini-transcribe-latest": voxtralTranscribeConfig(0.0035),
	"voxtral-mini-tts-2603": {
		Audio: &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceUsd: 16, InputPriceQuantity: 1e6},
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
	"voxtral-mini-tts-3":      voxtralTTSConfig(),
	"voxtral-mini-tts-3-0":    voxtralTTSConfig(),
	"voxtral-mini-tts-latest": voxtralTTSConfig(),
	"voxtral-small-2507": {
		Ratio:                       0.1 * ratio.MilliTokensUsd, // $0.10 input
		CompletionRatio:             4.0,                        // $0.40 output
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
	"voxtral-small-transcribe-3":      voxtralTranscribeConfig(0.004),
	"voxtral-small-transcribe-3-0":    voxtralTranscribeConfig(0.004),
	"voxtral-small-transcribe-latest": voxtralTranscribeConfig(0.004),
	"voxtral-tts-2603": {
		Audio: &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceUsd: 16, InputPriceQuantity: 1e6},
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

	// zai models.
	"zai-glm-5-2": {
		Ratio:             1.4 * ratio.MilliTokensUsd,
		CachedInputRatio:  0.14 * ratio.MilliTokensUsd,
		CompletionRatio:   4.4 / 1.4,
		ContextLength:     1_000_000,
		MaxOutputTokens:   128_000,
		InputModalities:   []string{"text"},
		OutputModalities:  []string{"text"},
		SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: []string{
			"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice",
		},
		Description: "Deprecated; retired on 2026-10-31. Use zai-glm-5-3 at the same tariff." + " Mistral-hosted GLM model with 1M context and 128K maximum output. Uses Mistral's model ID and USD tariff: $1.40 input, $0.14 cached input, and $4.40 output per million tokens; not Z.AI's cached-input price. Reasoning controls are advertised only when documented for this hosted model.",
	},
	"zai-glm-5-3": {
		Ratio:                       1.4 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             4.4 / 1.4,
		ContextLength:               1_000_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice", "reasoning_effort"},
		Description:                 "Generally Available since 2026-09-28." + " Mistral-hosted GLM model with 1M context and 128K maximum output. Uses Mistral's model ID and USD tariff: $1.40 input, $0.14 cached input, and $4.40 output per million tokens; not Z.AI's cached-input price. Reasoning controls are advertised only when documented for this hosted model." + " Mistral supports low/high/max reasoning_effort; none is not supported.",
		SupportedReasoningEfforts:   []string{"low", "high", "max"},
	},
	"zai-glm-5-3-flash": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.50 / 0.15,
		ContextLength: 1_000_000, MaxOutputTokens: 128_000,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedReasoningEfforts:   []string{"low", "high", "max"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice", "reasoning_effort"},
		Description:                 "Mistral-hosted GLM 5.3 Flash, generally available since 2026-10-22, with text/image input, 1M context and 128K maximum output. Mistral USD/M input/cache/output: 0.15/0.03/0.50. Native low/high/max reasoning; no unsupported none value or undocumented default is advertised.",
	},
})

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

// large4ModelConfig returns an independently owned Large 4 chat configuration.
// It takes no parameters. Sources verified on 2026-11-16:
// https://docs.mistral.ai/models/mistral-large-4-0
// https://docs.mistral.ai/inference/pricing
// https://docs.mistral.ai/resources/changelogs
// https://docs.mistral.ai/inference/conversations/reasoning
//
// The October 6 announcement grants a two-week launch discount but publishes
// no exact cutoff time or timezone. The bounded overlay uses the repository's
// explicit UTC calendar-date convention, not a claimed provider cutoff instant.
// Standard list prices are the fallback, so the discount cannot become permanent.
func large4ModelConfig() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:                     0.70 * ratio.MilliTokensUsd,
		CachedInputRatio:          0.07 * ratio.MilliTokensUsd,
		CompletionRatio:           2.10 / 0.70,
		ContextLength:             1_000_000,
		InputModalities:           []string{"text", "image"},
		OutputModalities:          []string{"text"},
		SupportedFeatures:         []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "high",
		SupportedSamplingParameters: []string{
			"temperature", "top_p", "max_tokens", "stop", "seed", "frequency_penalty", "presence_penalty", "reasoning_effort",
		},
		TimeWindows: []adaptor.TimeWindow{{
			Name: "large4-launch-utc-convention", TimeZone: "UTC",
			DateFrom: "2026-10-06", DateTo: "2026-10-20",
			Ranges: []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay: adaptor.ModelConfig{
				Ratio:            0.68 * ratio.MilliTokensUsd,
				CachedInputRatio: 0.07 * ratio.MilliTokensUsd,
				CompletionRatio:  2.09 / 0.68,
			},
		}},
		Description: "Mistral Large 4, generally available since 2026-11-05, with a published 1M context and low/medium/high reasoning (default high). Since October 29, none is no longer supported. The output ceiling and hosted quantization are unspecified. Current USD/M input/cache/output: 0.70/0.07/2.10. Historical two-week launch prices remain 0.68/0.07/2.09; the gateway uses October 6 through October 19 UTC because the provider does not publish an exact cutoff timezone. Current defaults do not reconstruct historical standard-price changes. Operators must reconcile date-boundary conventions with their upstream invoice.",
	}
}

// large4LatestModelConfig returns the documented mutable Large 4 alias without
// retroactively attaching the October launch discount to the former Large 3 alias.
// It takes no parameters and returns an independently owned configuration.
func large4LatestModelConfig() adaptor.ModelConfig {
	cfg := large4ModelConfig()
	cfg.TimeWindows = nil
	cfg.Description = "Mutable mistral-large-latest alias, documented as Mistral Large 4 on 2026-11-16. Current USD/M input/cache/output: 0.70/0.07/2.10; 1M context and low/medium/high reasoning, default high. The former Large 3 alias does not inherit Large 4's expired launch promotion. Pinned mistral-large-2512 remains independently configured."
	return cfg
}

// voxtralTranscribeConfig returns independent duration pricing for an offline
// Voxtral Transcribe 3 alias. Parameter usd is the price per minute; no realtime
// transport is implied. Sources checked 2026-11-16:
// https://docs.mistral.ai/models/voxtral-mini-transcribe-3-0
// https://docs.mistral.ai/models/voxtral-small-transcribe-3-0
func voxtralTranscribeConfig(usd float64) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		CompletionRatio: 1,
		ContextLength:   32_000,
		InputModalities: []string{"audio"}, OutputModalities: []string{"text"},
		Audio: &adaptor.AudioPricingConfig{
			InputUnit: "seconds", InputPriceUsd: usd, InputPriceQuantity: 60,
			UsdPerSecond: usd / 60,
		},
		Description: "Voxtral Transcribe 3 preview, released 2026-10-20, for the native /v1/audio/transcriptions endpoint. Billed by input audio duration, not text tokens. The documented context is 32k; no undocumented maximum output, cached-audio billing or realtime transport is implied.",
	}
}

// voxtralTTSConfig returns independently owned character pricing for an offline
// Voxtral Mini TTS 3 alias. It takes no parameters. Source checked 2026-11-16:
// https://docs.mistral.ai/models/voxtral-mini-tts-3-0
func voxtralTTSConfig() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio: 13 * ratio.MilliTokensUsd, CompletionRatio: 1,
		InputModalities: []string{"text"}, OutputModalities: []string{"audio"},
		Audio:       &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceUsd: 13, InputPriceQuantity: 1e6},
		Description: "Voxtral Mini TTS 3 preview, released 2026-10-20, for the native /v1/audio/speech endpoint at $13 per million input characters. The latest alias now targets generation 3; the March 2026 pinned IDs retain their historical tariff. Context and output ceilings are not published.",
	}
}
