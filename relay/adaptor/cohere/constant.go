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
// Official sources (verified May 2026):
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
var ModelRatios = map[string]adaptor.ModelConfig{
	// Current Command Models. Pricing per 1M tokens (May 2026).
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
	"command-r-08-2024": {
		// $0.15 input / $0.60 output per 1M tokens.
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeatures, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Cohere Command R (August 2024 refresh) for RAG, code and agent workflows.",
	},
	"command-r-plus-08-2024": {
		// $2.50 input / $10.00 output per 1M tokens.
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesWithSearch, SupportedSamplingParameters: cohereSamplingParams,
		Description: "Cohere Command R+ (August 2024 refresh) flagship RAG and tool-use model.",
	},

	// Aya research models (open-weights, hosted on Cohere API).
	// Aya Expanse: $0.50 input / $1.50 output per 1M tokens; multilingual research model.
	// Note: c4ai-aya-expanse-8b and c4ai-aya-vision-8b retired 2026-04-04 and have been removed.
	// Reference: https://docs.cohere.com/docs/aya and https://docs.cohere.com/changelog
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

	// Tiny Aya family (released 2026-02-17). 3.35B open-weight multilingual models
	// covering 70 languages with regional specializations. Surfaced on the Cohere API
	// with conservative Aya-family pricing pending official rate publication.
	// References: https://cohere.com/blog/cohere-labs-tiny-aya
	// https://huggingface.co/CohereLabs/tiny-aya-global
	"tiny-aya-global": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/tiny-aya-global",
		Description:   "Tiny Aya Global (2026-02) 3.35B open-weight multilingual instruction-tuned model covering 70 languages.",
	},
	"tiny-aya-earth": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/tiny-aya-earth",
		Description:   "Tiny Aya Earth (2026-02) 3.35B regional multilingual model tuned for African languages.",
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
	"tiny-aya-water": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: cohereChatFeaturesNoStructured, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/tiny-aya-water",
		Description:   "Tiny Aya Water (2026-02) 3.35B regional multilingual model tuned for Asia-Pacific, West Asian, and European languages.",
	},

	// Command A specialized variants released in 2025.
	"command-a-vision-07-2025": {
		// $2.50 input / $10.00 output per 1M tokens (vision variant inherits Command A rates).
		// 128K context, supports up to 20 images per request; no tool use per docs.
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cohereMultimodalInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: []string{"json_mode"}, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/command-a-vision-07-2025",
		Description:   "Cohere Command A Vision (July 2025) 112B multimodal model for document analysis, OCR, and chart interpretation.",
	},
	"command-a-reasoning-08-2025": {
		// $2.50 input / $10.00 output per 1M tokens (inherits Command A rates).
		// 256K context with toggleable thinking; multilingual reasoning across 23 languages.
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 256000, MaxOutputTokens: 32768,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"},
		SupportedSamplingParameters: cohereSamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high"},
		Quantization:                "bf16",
		HuggingFaceID:               "CohereLabs/command-a-reasoning-08-2025",
		Description:                 "Cohere Command A Reasoning (August 2025) 111B hybrid reasoning model with toggleable thinking and 256k context.",
	},
	"command-a-translate-08-2025": {
		// $2.50 input / $10.00 output per 1M tokens (inherits Command A rates).
		// 16K total context split 8K input / 8K output; translation-focused, no tool use.
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 16000, MaxOutputTokens: 8192,
		InputModalities: cohereTextInputs, OutputModalities: cohereTextOutputs,
		SupportedFeatures: []string{}, SupportedSamplingParameters: cohereSamplingParams,
		Quantization:  "bf16",
		HuggingFaceID: "CohereLabs/command-a-translate-08-2025",
		Description:   "Cohere Command A Translate (August 2025) 111B translation specialist covering 23 languages.",
	},

	// Audio transcription model. cohere-transcribe is offered via Model Vault per-hour
	// pricing; we expose token-billing fallback in case the relay needs to bill text fragments.
	// Reference: https://docs.cohere.com/docs/transcribe
	"cohere-transcribe-03-2026": {
		Ratio:           0.0,
		CompletionRatio: 1.0,
		ContextLength:   32768,
		InputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			// Cohere Transcribe is currently free during API trial with rate limits;
			// production deployment is per-hour Model Vault. No per-second public rate.
			UsdPerSecond: 0.0,
		},
		HuggingFaceID: "CohereLabs/cohere-transcribe-03-2026",
		Description:   "Cohere Transcribe (March 2026) state-of-the-art multilingual ASR model covering 14 languages with 25MB max file size.",
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

	// Rerank Models. Cohere prices rerank v3.5 and v3.0 at $2.00 per 1,000 searches
	// (one search = one query with up to 100 documents). Per-call pricing is encoded
	// as USD-per-call divided over the QuotaPerUsd factor.
	// Source: https://cohere.com/pricing
	"rerank-v4.0-pro": {
		// Rerank v4.0 Pro: $2.50 per 1,000 searches.
		// References: https://openrouter.ai/cohere/rerank-4-pro and Cohere pricing page.
		Ratio:            (2.5 / 1000.0) * ratio.QuotaPerUsd,
		ContextLength:    32768,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank v4.0 Pro multilingual reranker with 32k context window optimized for best accuracy.",
	},
	"rerank-v4.0-fast": {
		// Rerank v4.0 Fast: $2.00 per 1,000 searches (lighter low-latency variant).
		Ratio:            (2.0 / 1000.0) * ratio.QuotaPerUsd,
		ContextLength:    32768,
		InputModalities:  cohereTextInputs,
		OutputModalities: cohereTextOutputs,
		Description:      "Cohere Rerank v4.0 Fast low-latency multilingual reranker with 32k context window.",
	},
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

	// Embed Models. Pricing is per 1M input tokens, surfaced through the embedding
	// pricing configuration so the relay can bill per-modality.
	// Sources: https://cohere.com/pricing and https://docs.cohere.com/changelog/embed-multimodal-v4
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
	"embed-english-v3.0": {
		// $0.10 per 1M input tokens.
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereTextInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed English v3 1024-dim text embedding model.",
	},
	"embed-english-light-v3.0": {
		// $0.10 per 1M input tokens (lightweight 384-dim variant).
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereTextInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed English Light v3 384-dim faster text embedding model.",
	},
	"embed-multilingual-v3.0": {
		// $0.10 per 1M input tokens.
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereTextInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed Multilingual v3 1024-dim 100+ language embedding model.",
	},
	"embed-multilingual-light-v3.0": {
		// $0.10 per 1M input tokens (lightweight 384-dim variant).
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		ContextLength:   512,
		InputModalities: cohereTextInputs,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.10 * ratio.MilliTokensUsd,
		},
		Description: "Cohere Embed Multilingual Light v3 384-dim faster multilingual embedding model.",
	},
}

// CohereToolingDefaults remains empty because Cohere's official docs and pricing pages publish
// model pricing, but not separate server-side tool invocation fees.
// Sources:
// - https://docs.cohere.com/docs/how-does-cohere-pricing-work
// - https://cohere.com/pricing
var CohereToolingDefaults = adaptor.ChannelToolConfig{}
