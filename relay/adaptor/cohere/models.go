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
	// cohereMultimodalInputs lists modalities for models accepting text and images
	// (e.g. Embed v4, Aya Vision).
	cohereMultimodalInputs = []string{"text", "image"}
	// cohereTextOutputs lists the output modalities for Cohere chat completions.
	cohereTextOutputs = []string{"text"}

	// cohereChatFeatures advertises the capability set for current Command Cohere chat models.
	// Tools, JSON mode and structured outputs are all documented as supported.
	cohereChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// cohereChatFeaturesWithSearch is the capability set for current flagship Cohere chat
	// models that additionally expose connector-based web search (Command A, Command R+).
	cohereChatFeaturesWithSearch = []string{"tools", "json_mode", "structured_outputs", "web_search"}
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
// Official sources (verified 2026-06-13):
// - https://docs.cohere.com/docs/models
// - https://docs.cohere.com/docs/structured-outputs
// - https://docs.cohere.com/docs/how-does-cohere-pricing-work
// - https://cohere.com/pricing
// - https://docs.cohere.com/docs/aya-vision
// - https://docs.cohere.com/changelog/embed-multimodal-v4
// HuggingFace research weight references (where available):
// - https://huggingface.co/CohereLabs/c4ai-command-a-03-2025
// - https://huggingface.co/CohereLabs/c4ai-command-r-plus
// - https://huggingface.co/CohereLabs/c4ai-command-r-v01
// - https://huggingface.co/CohereLabs/c4ai-command-r7b-12-2024
// - https://huggingface.co/CohereLabs/aya-expanse-32b
// - https://huggingface.co/CohereLabs/aya-vision-32b
// Additional model-specific sources checked 2026-10-12 and 2026-11-16:
// https://docs.cohere.com/docs/command-a-plus
// https://docs.cohere.com/docs/command-a-reasoning
// https://docs.cohere.com/docs/command-a-translate
// https://docs.cohere.com/docs/command-a-vision
// https://docs.cohere.com/docs/cohere-embed
// https://cohere.com/blog/embed-5
// https://docs.cohere.com/docs/north-mini-code-1.0
// https://docs.cohere.com/docs/north-small-translate-1.0
var ModelRatios = adaptor.JoinModelCatalogs(map[string]adaptor.ModelConfig{

	// c4ai models.
	"c4ai-aya-expanse-32b": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/aya-expanse-32b",
		Description:   "Cohere Aya Expanse 32B multilingual research model with 128k context.",
	},
	"c4ai-aya-vision-32b": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 16000, MaxOutputTokens: 4096,
		InputModalities: cohereMultimodalInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/aya-vision-32b",
		Description:   "Cohere Aya Vision 32B multilingual multimodal research model (text+image inputs).",
	},

	// cohere models.
	"cohere-transcribe-03-2026": {
		Ratio:           0.0,
		CompletionRatio: 1.0,
		ContextLength:   32768,
		InputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			// Cohere Transcribe is currently free during API trial with rate limits;
			// dedicated production deployment via Model Vault starts from $3.75/hour/instance
			// (per https://cohere.com/pricing). No public per-second/per-token rate for the
			// free trial tier, so UsdPerSecond is left at 0 as a non-billing placeholder.
			UsdPerSecond: 0.0,
		},
		HuggingFaceID: "CohereLabs/cohere-transcribe-03-2026",
		Description:   "Cohere Transcribe (March 2026) state-of-the-art multilingual ASR model covering 14 languages with 25MB max file size.",
	},

	// command models.
	"command": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command instruction-following model (deprecated 2025-09-15).",
	},
	"command-a-03-2025": {
		// $2.50 input / $10.00 output per 1M tokens.
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 256000, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesWithSearch, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/c4ai-command-a-03-2025",
		Description:   "Cohere Command A (March 2025) flagship 111B model excelling at tool use, agents and RAG.",
	},
	"command-a-plus-05-2026": {
		// Public API access is free until rate limits; see the model card.
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 128000, MaxOutputTokens: 64000,
		InputModalities: cohereMultimodalInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: cohereSamplingParams,
		MaxReasoningTokens:          64000,
		Quantization:                "fp8",
		HuggingFaceID:               "CohereLabs/command-a-plus-05-2026-fp8",
		Description:                 "Command A+ (May 2026), Cohere's multimodal mixture-of-experts model with 128K context and 64K maximum output. Public API access is free until rate limits for both trial and production keys. Dedicated Model Vault or negotiated production deployment prices must be configured separately; this is not unlimited free production hosting.",
	},
	"command-a-reasoning-08-2025": {
		// No public per-token pricing published by Cohere (free-until-rate-limits / Model Vault).
		// $2.50 input / $10.00 output per 1M tokens is an estimate inherited from Command A.
		// 256K context with budget-token thinking; multilingual reasoning across 23 languages.
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 256000, MaxOutputTokens: 32768,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: cohereSamplingParams,
		MaxReasoningTokens:          32000,
		Quantization:                "bf16",
		HuggingFaceID:               "CohereLabs/command-a-reasoning-08-2025",
		Description:                 "Command A Reasoning (August 2025), Cohere's hybrid reasoning model with 256K context. Public API access is free until rate limits for both trial and production keys. Dedicated Model Vault or negotiated production deployment prices must be configured separately; this is not unlimited free production hosting.",
	},
	"command-a-translate-08-2025": {
		// No public per-token pricing; free-until-rate-limit, contact sales for production
		// (per https://docs.cohere.com/docs/command-a-translate: "free until rate limits are reached").
		// Public API access is free until rate limits; dedicated deployment pricing is separate.
		// 16K total context split 8K input / 8K output; translation-focused, no tool use.
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 16000, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: []string{}, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/command-a-translate-08-2025",
		Description:   "Command A Translate (August 2025), Cohere's multilingual translation specialist. Public API access is free until rate limits for both trial and production keys. Dedicated Model Vault or negotiated production deployment prices must be configured separately; this is not unlimited free production hosting.",
	},
	"command-a-vision-07-2025": {
		// No public per-token pricing; free-until-rate-limit, contact sales for production
		// (per https://docs.cohere.com/docs/command-a-vision: "free until rate limits are reached").
		// Public API access is free until rate limits; dedicated deployment pricing is separate.
		// 128K context, supports up to 20 images per request; no tool use per docs.
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cohereMultimodalInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: []string{"json_mode"}, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/command-a-vision-07-2025",
		Description:   "Command A Vision (July 2025), Cohere's image-understanding model with 128K context; tool use is not supported. Public API access is free until rate limits for both trial and production keys. Dedicated Model Vault or negotiated production deployment prices must be configured separately; this is not unlimited free production hosting.",
	},
	"command-internet": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command with internet grounding enabled.",
	},
	"command-light": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command Light faster variant (deprecated 2025-09-15).",
	},
	"command-light-internet": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy Cohere Command Light with internet grounding enabled.",
	},
	"command-light-nightly": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Nightly build of the legacy Cohere Command Light model (deprecated 2025-09-15).",
	},
	"command-light-nightly-internet": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2, // $0.3/$0.6 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy nightly Cohere Command Light with internet grounding enabled.",
	},
	"command-nightly": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereLegacyFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Nightly build of the legacy Cohere Command model (deprecated 2025-09-15).",
	},
	"command-nightly-internet": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2, // $1/$2 per 1M tokens
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereLegacyFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		Description: "Legacy nightly Cohere Command with internet grounding enabled.",
	},
	"command-r": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3, // $0.5/$1.5 per 1M tokens
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/c4ai-command-r-v01",
		Description:   "Cohere Command R alias for command-r-03-2024 (deprecated 2025-09-15).",
	},
	"command-r-08-2024": {
		// $0.15 input / $0.60 output per 1M tokens.
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Cohere Command R (August 2024 refresh) for RAG, code and agent workflows.",
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
	"command-r-plus": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5, // $3/$15 per 1M tokens
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		HuggingFaceID: "CohereLabs/c4ai-command-r-plus",
		Description:   "Cohere Command R+ alias for command-r-plus-04-2024 (deprecated 2025-09-15).",
	},
	"command-r-plus-08-2024": {
		// $2.50 input / $10.00 output per 1M tokens.
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesWithSearch, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Cohere Command R+ (August 2024 refresh) flagship RAG and tool-use model.",
	},
	"command-r-plus-internet": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5, // $3/$15 per 1M tokens
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: append([]string{"web_search"}, cohereChatFeatures...), SupportedSamplingParameters: cohereSamplingParams,
		HuggingFaceID: "CohereLabs/c4ai-command-r-plus",
		Description:   "Cohere Command R+ with internet grounding enabled.",
	},
	"command-r7b-12-2024": {
		// $0.0375 input / $0.15 output per 1M tokens.
		Ratio: 0.0375 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/c4ai-command-r7b-12-2024",
		Description:   "Cohere Command R7B (December 2024) compact 7B chat model tuned for RAG and tool use.",
	},

	// embed models.
	"embed-english-light-v3.0": {
		// $0.10 per 1M input tokens (lightweight 384-dim variant).
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereMultimodalInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed English Light v3 384-dim faster text embedding model.",
	},
	"embed-english-v3.0": {
		// $0.10 per 1M input tokens.
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereMultimodalInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed English v3 1024-dim text embedding model.",
	},
	"embed-multilingual-light-v3.0": {
		// $0.10 per 1M input tokens (lightweight 384-dim variant).
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereMultimodalInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed Multilingual Light v3 384-dim faster multilingual embedding model.",
	},
	"embed-multilingual-v3.0": {
		// $0.10 per 1M input tokens.
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereMultimodalInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed Multilingual v3 1024-dim 100+ language embedding model.",
	},
	"embed-v4.0": {
		// $0.12 per 1M text tokens, $0.47 per 1M image tokens.
		Ratio:           0.12 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   128000,
		InputModalities: cohereMultimodalInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio:  0.12 * ratio.MilliTokensUsd,
			ImageTokenRatio: 0.47 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed v4 multimodal embeddings (text+image) with 128k context and Matryoshka dimensions.",
	},
	"embed-v5.0-fast": {
		Ratio: 0.08 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   128_000,
		InputModalities: []string{"text"},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio:  0.08 * ratio.MilliTokensUsd,
			ImageTokenRatio: 0.40 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed 5 via /v2/embed, 128k context, configurable dimensions 256/512/768/1024/1536/2048 (default 2048). This adaptor admits text input only; the documented $0.40/M image-token tariff does not imply support for the provider's native media payload. Text billing uses meta.billed_units.input_tokens.",
	},
	"embed-v5.0-pro": {
		Ratio: 0.12 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   128_000,
		InputModalities: []string{"text"},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio:  0.12 * ratio.MilliTokensUsd,
			ImageTokenRatio: 0.40 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed 5 via /v2/embed, 128k context, configurable dimensions 256/512/768/1024/1536/2048 (default 2048). This adaptor admits text input only; the documented $0.40/M image-token tariff does not imply support for the provider's native media payload. Text billing uses meta.billed_units.input_tokens.",
	},

	// north models.
	"north-mini-code-1-0": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 256000, MaxOutputTokens: 64000,
		InputModalities: []string{"text"}, OutputModalities: []string{"text"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
		Description:                 "North Mini Code 1.0, Cohere's 30B/3B-active coding model. Public API access is free until rate limits for trial and production keys; dedicated Model Vault production pricing must be configured separately. This adaptor currently supports basic text chat, not the model's native tool/reasoning controls. Historical gateway spelling retained for administrator mappings; the published API ID is north-mini-code-1.0.",
	},
	"north-mini-code-1.0": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 256000, MaxOutputTokens: 64000,
		InputModalities: []string{"text"}, OutputModalities: []string{"text"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
		Description:                 "North Mini Code 1.0, Cohere's 30B/3B-active coding model. Public API access is free until rate limits for trial and production keys; dedicated Model Vault production pricing must be configured separately. This adaptor currently supports basic text chat, not the model's native tool/reasoning controls.",
	},
	"north-small-translate-1-0": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 16000, MaxOutputTokens: 16000,
		InputModalities: []string{"text"}, OutputModalities: []string{"text"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
		Description:                 "North Small Translate 1.0, Cohere's multilingual translation model. Public API access is free until rate limits for trial and production keys; production deployment contracts require separate operator pricing. Text input and output only; no inherited Command tools or vision advertisement. Historical gateway spelling retained for administrator mappings; the published API ID is north-small-translate-1.0.",
	},
	"north-small-translate-1.0": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength: 16000, MaxOutputTokens: 16000,
		InputModalities: []string{"text"}, OutputModalities: []string{"text"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
		Description:                 "North Small Translate 1.0, Cohere's multilingual translation model. Public API access is free until rate limits for trial and production keys; production deployment contracts require separate operator pricing. Text input and output only; no inherited Command tools or vision advertisement.",
	},

	// rerank models.
	"rerank-english-v3.0": {
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.0},
		ContextLength:    4096,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank English v3.0 reranker for English documents.",
	},
	"rerank-multilingual-v3.0": {
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.0},
		ContextLength:    4096,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank Multilingual v3.0 reranker for non-English documents and JSON.",
	},
	"rerank-v3.5": {
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.0},
		ContextLength:    4096,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank v3.5 multilingual reranker for English documents and JSON.",
	},
	"rerank-v4.0-fast": {
		// Rerank v4.0 Fast: $2.00 per 1,000 searches (lighter low-latency variant).
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.0},
		ContextLength:    32768,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank v4.0 Fast low-latency multilingual reranker with 32k context window.",
	},
	"rerank-v4.0-pro": {
		// Rerank v4.0 Pro: $2.50 per 1,000 searches.
		// References: https://openrouter.ai/cohere/rerank-4-pro and Cohere pricing page.
		Ratio:            (2.5 / 1000.0) * ratio.QuotaPerUsd,
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5},
		ContextLength:    32768,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank v4.0 Pro multilingual reranker with 32k context window optimized for best accuracy.",
	},

	// tiny models.
	"tiny-aya-earth": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/tiny-aya-earth",
		Description:   "Tiny Aya Earth (2026-02) 3.35B regional multilingual model tuned for West Asian and African languages.",
	},
	"tiny-aya-fire": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/tiny-aya-fire",
		Description:   "Tiny Aya Fire (2026-02) 3.35B regional multilingual model tuned for South Asian languages.",
	},
	"tiny-aya-global": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/tiny-aya-global",
		Description:   "Tiny Aya Global (2026-02) 3.35B open-weight multilingual instruction-tuned model covering 70 languages.",
	},
	"tiny-aya-water": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/tiny-aya-water",
		Description:   "Tiny Aya Water (2026-02) 3.35B regional multilingual model tuned for Asia-Pacific and European languages.",
	},
})

// CohereToolingDefaults remains empty because Cohere's official docs and pricing pages publish
// model pricing, but not separate server-side tool invocation fees.
// Sources:
// - https://docs.cohere.com/docs/how-does-cohere-pricing-work
// - https://cohere.com/pricing
var CohereToolingDefaults = adaptor.ChannelToolConfig{}
