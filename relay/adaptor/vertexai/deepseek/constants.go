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
		Ratio:                       0.56 * ratio.MilliTokensUsd,
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
	// DeepSeek V3.1 - Input: $1.35 / million tokens, Output: $5.40 / million tokens
	// Vertex GA 2025-08-28 in us-central1; hybrid thinking via chat_template_kwargs.thinking.
	// (Vertex aligns V3.1 with R1 list pricing; cloudprice confirmed 2026-05-19.)
	"deepseek-ai/deepseek-v3.1-maas": {
		Ratio:                       1.35 * ratio.MilliTokensUsd, // Input price: $1.35 per million tokens
		CompletionRatio:             5.40 / 1.35,                 // Output/Input ratio: $5.40 / $1.35 = 4.0
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
