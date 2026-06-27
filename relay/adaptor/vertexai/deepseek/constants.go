// Package deepseek provides model pricing constants for DeepSeek AI models in Vertex AI.
package deepseek

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	deepSeekVertexTextFileInputs    = []string{"text", "file"}
	deepSeekVertexOCRInputs         = []string{"text", "image", "file"}
	deepSeekVertexTextOutputs       = []string{"text"}
	deepSeekVertexStandardFeatures  = []string{"tools", "json_mode", "structured_outputs"}
	deepSeekVertexReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	deepSeekVertexStandardSampling  = []string{"temperature", "top_p", "stop", "max_tokens"}
	deepSeekVertexReasoningSampling = []string{"stop", "max_tokens"}
)

// Prompt cache: Vertex AI MaaS implicit caching grants a 90% discount on cached
// input tokens (cache-hit billed at 0.1x standard input) and is documented as
// supported only for deepseek-v3.1-maas and deepseek-v3.2-maas; r1-0528 and OCR
// are not on the implicit-cache list and intentionally carry no CachedInputRatio.
// Source: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/use-open-models
// Note: this is a latent/defensive ratio — Vertex surfaces cached tokens via the
// Gemini-style cachedContentTokenCount; one-api only bills it when the OpenAI-shaped
// cached_tokens field is populated, so it is harmless when not yet emitted.
//
// ModelRatios contains DeepSeek models and their pricing ratios on Vertex AI MaaS.
//
// Pricing sources (retrieved 2026-05-19):
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/deepseek/deepseek-ocr
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/deepseek/deepseek-v31
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/deepseek/deepseek-v32
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/deepseek/deepseek-r1
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/capabilities/thinking
//   - https://cloudprice.net/models/vertex_ai/deepseek-ai/deepseek-v3.1-maas
//   - https://cloudprice.net/models/vertex_ai/deepseek-ai/deepseek-v3.2-maas
//   - https://cloudprice.net/models/vertex_ai/deepseek-ai/deepseek-ocr-maas
var ModelRatios = map[string]adaptor.ModelConfig{
	// DeepSeek OCR - Input: $0.30 / million tokens, Output: $1.20 / million tokens
	// Vertex GA 2025-10-23 in us-central1 only; accepts text, documents, and images.
	"deepseek-ai/deepseek-ocr-maas": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             1.20 / 0.30,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             deepSeekVertexOCRInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexStandardFeatures,
		SupportedSamplingParameters: deepSeekVertexStandardSampling,
		Description:                 "DeepSeek OCR on Vertex AI MaaS (us-central1) for document and image understanding with text output.",
	},
	// DeepSeek V3.2 - Input: $0.56 / million tokens, Output: $1.68 / million tokens
	// Vertex GA 2025-12-10; global endpoint. Hybrid thinking via chat_template_kwargs.thinking;
	// reasoning text surfaces in reasoning_content, response text in content.
	"deepseek-ai/deepseek-v3.2-maas": {
		Ratio: 0.56 * ratio.MilliTokensUsd,
		// Vertex implicit caching gives a 90% discount on cached input tokens
		// (cache-hit billed at 0.1x standard input). deepseek-v3.2-maas is listed
		// as implicit-cache supported by Vertex MaaS.
		CachedInputRatio:            0.1 * 0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1.68 / 0.56,
		ContextLength:               163840,
		MaxOutputTokens:             65536,
		InputModalities:             deepSeekVertexTextFileInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexReasoningFeatures,
		SupportedSamplingParameters: deepSeekVertexStandardSampling,
		// Vertex AI MaaS does not publish a tunable reasoning_effort for DeepSeek V3.2;
		// thinking is toggled via chat_template_kwargs.thinking (boolean) in the request body.
		Description: "DeepSeek V3.2 on Vertex AI MaaS with long context, tool use, and optional hybrid thinking.",
	},
	// DeepSeek V3.1 - Input: $0.60 / million tokens, Output: $1.70 / million tokens
	// Vertex GA 2025-08-28 in us-central1; hybrid thinking via chat_template_kwargs.thinking.
	// (Vertex dropped V3.1 below R1 list pricing; live pricing page + cloudprice/portkey confirmed 2026-06-27.)
	"deepseek-ai/deepseek-v3.1-maas": {
		Ratio: 0.60 * ratio.MilliTokensUsd, // Input price: $0.60 per million tokens
		// Vertex implicit caching gives a discount on cached input tokens.
		// deepseek-v3.1-maas publishes an explicit Cache Hit rate of $0.06/1M.
		CachedInputRatio:            0.06 * ratio.MilliTokensUsd,
		CompletionRatio:             1.70 / 0.60, // Output/Input ratio: $1.70 / $0.60 = 2.8333
		ContextLength:               163840,
		MaxOutputTokens:             32768,
		InputModalities:             deepSeekVertexTextFileInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexReasoningFeatures,
		SupportedSamplingParameters: deepSeekVertexStandardSampling,
		// Vertex AI MaaS does not publish a tunable reasoning_effort for DeepSeek V3.1;
		// thinking is toggled via chat_template_kwargs.thinking (boolean) in the request body.
		Description: "DeepSeek V3.1 on Vertex AI MaaS with long context and optional hybrid thinking mode.",
	},
	// DeepSeek R1-0528 - Input: $1.35 / million tokens, Output: $5.40 / million tokens
	// Always-on reasoning. Vertex returns thinking inside <think>...</think> tags in `content`
	// (no separate reasoning_content field for this model variant).
	"deepseek-ai/deepseek-r1-0528-maas": {
		Ratio:                       1.35 * ratio.MilliTokensUsd, // Input price: $1.35 per million tokens
		CompletionRatio:             5.40 / 1.35,                 // Output/Input ratio: $5.40 / $1.35 = 4.0
		ContextLength:               163840,
		MaxOutputTokens:             32768,
		InputModalities:             deepSeekVertexTextFileInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexReasoningFeatures,
		SupportedSamplingParameters: deepSeekVertexReasoningSampling,
		MaxReasoningTokens:          32768,
		// R1 reasoning is always-on; no reasoning_effort or budget is exposed.
		Description: "DeepSeek R1-0528 on Vertex AI MaaS optimized for reasoning with restricted sampling controls.",
	},
}

// ModelList derived from ModelRatios keys
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
