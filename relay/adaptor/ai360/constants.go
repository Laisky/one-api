package ai360

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// AI360 retains historical chat and embedding metadata. Rechecked 2026-11-16:
// https://ai.360.com/open/zh/models now publishes native-CNY model cards, and
// https://ai.360.com/docs/quick-start documents the newer public platform.
// The historical entries below remain unverified against current tariffs.
// Router cards may expose different provider-specific prices for the same
// virtual model. Their current API host is api.360.cn, whereas the legacy
// channel defaults to ai.360.cn. Those routing, tariff-selection and cache-write
// distinctions need separate qualification before importing router defaults.
//
// All 360GPT* and embedding SKUs are closed-weight (HuggingFaceID stays empty).

// ai360TextInputs is the input modality set for AI360 chat / embedding models.
var ai360TextInputs = []string{"text"}

// ai360TextOutputs is the text-only output modality used by AI360.
var ai360TextOutputs = []string{"text"}

// ai360ChatFeatures captures tool / JSON capabilities advertised by the AI360
// chat completion API.
var ai360ChatFeatures = []string{"tools", "json_mode"}

// ai360SamplingParams enumerates OpenAI-compatible sampling parameters
// accepted by AI360 chat completions.
var ai360SamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
var ModelRatios = map[string]adaptor.ModelConfig{
	// AI360 Models - Based on historical pricing
	"360GPT_S2_V9": {
		Ratio:                       0.8572 * ratio.MilliTokensUsd, // CNY 0.012 / 1M tokens
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             ai360TextInputs,
		OutputModalities:            ai360TextOutputs,
		SupportedFeatures:           ai360ChatFeatures,
		SupportedSamplingParameters: ai360SamplingParams,
		Description:                 "AI360 360GPT S2 V9 closed-weight chat model.",
	},
	"embedding-bert-512-v1": {
		Ratio:            0.0715 * ratio.MilliTokensUsd, // CNY 0.001 / 1M tokens
		CompletionRatio:  1,
		ContextLength:    512,
		InputModalities:  ai360TextInputs,
		OutputModalities: ai360TextOutputs,
		Description:      "AI360 BERT-based 512-token text embedding model.",
	},
	"embedding_s1_v1": {
		Ratio:            0.0715 * ratio.MilliTokensUsd, // CNY 0.001 / 1M tokens
		CompletionRatio:  1,
		ContextLength:    512,
		InputModalities:  ai360TextInputs,
		OutputModalities: ai360TextOutputs,
		Description:      "AI360 S1 v1 general-purpose text embedding model.",
	},
	"semantic_similarity_s1_v1": {
		Ratio:            0.0715 * ratio.MilliTokensUsd, // CNY 0.001 / 1M tokens
		CompletionRatio:  1,
		ContextLength:    512,
		InputModalities:  ai360TextInputs,
		OutputModalities: ai360TextOutputs,
		Description:      "AI360 S1 v1 semantic similarity model.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// AI360ToolingDefaults documents that AI360 does not publish built-in tool pricing (re-audited 2026-05-18).
// Source: https://r.jina.ai/https://ai.360.com/platform (login wall, no public tool catalog)
var AI360ToolingDefaults = adaptor.ChannelToolConfig{}
