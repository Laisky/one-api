package cohere

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for Cohere models. Reused across ModelRatios entries
// to keep the table compact and consistent.
var (
	// cohereTextInputs lists the input modalities supported by all Cohere chat models.
	cohereTextInputs = []string{"text"}
	// cohereTextOutputs lists the output modalities for Cohere chat completions.
	cohereTextOutputs = []string{"text"}

	// cohereChatFeatures advertises the capability set for current Command Cohere chat models.
	// Tools, JSON mode and structured outputs are all documented as supported.
	cohereChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// cohereChatFeaturesNoStructured advertises the capability set for Command R7B (smaller model
	// without published structured-outputs support) and the legacy generation-only Command models.
	cohereChatFeaturesNoStructured = []string{"tools", "json_mode"}
	// cohereLegacyFeatures advertises the limited capability set for the deprecated 4k-context
	// generation-only Command / Command Light models.
	cohereLegacyFeatures = []string{}

	// cohereSamplingParams lists the sampling parameters supported by Cohere chat completions.
	// Cohere natively exposes top_k in addition to the OpenAI-standard set.
	cohereSamplingParams = []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"}
)

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Official sources:
// - https://docs.cohere.com/v2/docs/models
// - https://docs.cohere.com/v2/docs/structured-outputs
// - https://docs.cohere.com/docs/how-does-cohere-pricing-work
// - https://cohere.com/pricing
// HuggingFace research weight references (where available):
// - https://huggingface.co/CohereLabs/c4ai-command-a-03-2025
// - https://huggingface.co/CohereLabs/c4ai-command-r-plus
// - https://huggingface.co/CohereLabs/c4ai-command-r-v01
// - https://huggingface.co/CohereLabs/c4ai-command-r7b-12-2024
var ModelRatios = map[string]adaptor.ModelConfig{
	// Current Command Models
	"command-a-03-2025": {
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 256000, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/c4ai-command-a-03-2025",
		Description:   "Cohere Command A (March 2025) flagship 111B model excelling at tool use, agents and RAG.",
	},
	"command-r7b-12-2024": {
		Ratio: 0.0375 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/c4ai-command-r7b-12-2024",
		Description:   "Cohere Command R7B (December 2024) compact 7B chat model tuned for RAG and tool use.",
	},
	"command-r-08-2024": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Cohere Command R (August 2024 refresh) for RAG, code and agent workflows.",
	},
	"command-r-plus-08-2024": {
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Cohere Command R+ (August 2024 refresh) flagship RAG and tool-use model.",
	},

	// Command Models (legacy generation-only; deprecated 2025-09-15)
	"command": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command instruction-following model (deprecated 2025-09-15).",
	},
	"command-nightly": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Nightly build of the legacy Cohere Command model (deprecated 2025-09-15).",
	},

	// Command Light Models (legacy; deprecated 2025-09-15)
	"command-light": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command Light faster variant (deprecated 2025-09-15).",
	},
	"command-light-nightly": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Nightly build of the legacy Cohere Command Light model (deprecated 2025-09-15).",
	},

	// Command R Models (aliases for the original 03-2024 / 04-2024 releases; deprecated 2025-09-15)
	"command-r": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3, // $0.5/$1.5 per 1M tokens
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/c4ai-command-r-v01",
		Description:   "Cohere Command R alias for command-r-03-2024 (deprecated 2025-09-15).",
	},
	"command-r-plus": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5, // $3/$15 per 1M tokens
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		HuggingFaceID: "CohereLabs/c4ai-command-r-plus",
		Description:   "Cohere Command R+ alias for command-r-plus-04-2024 (deprecated 2025-09-15).",
	},

	// Internet-enabled variants share the same upstream models with retrieval grounding enabled.
	"command-internet": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command with internet grounding enabled.",
	},
	"command-nightly-internet": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy nightly Cohere Command with internet grounding enabled.",
	},
	"command-light-internet": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command Light with internet grounding enabled.",
	},
	"command-light-nightly-internet": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy nightly Cohere Command Light with internet grounding enabled.",
	},
	"command-r-internet": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3, // $0.5/$1.5 per 1M tokens
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereChatFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/c4ai-command-r-v01",
		Description:   "Cohere Command R with internet grounding enabled.",
	},
	"command-r-plus-internet": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5, // $3/$15 per 1M tokens
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereChatFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		HuggingFaceID: "CohereLabs/c4ai-command-r-plus",
		Description:   "Cohere Command R+ with internet grounding enabled.",
	},

	// Rerank Models (per-call pricing; text-only, no chat-style sampling).
	"rerank-v3.5": {
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		ContextLength:    4096,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank v3.5 multilingual reranker for English documents and JSON.",
	},
	"rerank-english-v3.0": {
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		ContextLength:    4096,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank English v3.0 reranker for English documents.",
	},
	"rerank-multilingual-v3.0": {
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		ContextLength:    4096,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank Multilingual v3.0 reranker for non-English documents and JSON.",
	},
}

// CohereToolingDefaults remains empty because Cohere's official docs and pricing pages publish
// model pricing, but not separate server-side tool invocation fees.
// Sources:
// - https://docs.cohere.com/docs/how-does-cohere-pricing-work
// - https://cohere.com/pricing
var CohereToolingDefaults = adaptor.ChannelToolConfig{}
