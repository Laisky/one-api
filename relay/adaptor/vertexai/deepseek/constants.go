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

// ModelRatios contains DeepSeek models and their pricing ratios
var ModelRatios = map[string]adaptor.ModelConfig{
	// DeepSeek OCR - Input: $0.30 / million tokens, Output: $1.20 / million tokens
	"deepseek-ai/deepseek-ocr-maas": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             1.20 / 0.30,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             deepSeekVertexOCRInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexStandardFeatures,
		SupportedSamplingParameters: deepSeekVertexStandardSampling,
		Description:                 "DeepSeek OCR on Vertex AI MaaS for document and image understanding with text output.",
	},
	// DeepSeek V3.2 - Input: $0.56 / million tokens, Output: $1.68 / million tokens
	"deepseek-ai/deepseek-v3.2-maas": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1.68 / 0.56,
		ContextLength:               163840,
		MaxOutputTokens:             65536,
		InputModalities:             deepSeekVertexTextFileInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexReasoningFeatures,
		SupportedSamplingParameters: deepSeekVertexStandardSampling,
		Description:                 "DeepSeek V3.2 on Vertex AI MaaS with long context, tool use, and reasoning support.",
	},
	// DeepSeek V3.1 - Input: $0.60 / million tokens, Output: $1.70 / million tokens
	"deepseek-ai/deepseek-v3.1-maas": {
		Ratio:                       0.60 * ratio.MilliTokensUsd, // Input price: $0.60 per million tokens
		CompletionRatio:             1.70 / 0.60,                 // Output/Input ratio: $1.70 / $0.60 = 2.833
		ContextLength:               163840,
		MaxOutputTokens:             32768,
		InputModalities:             deepSeekVertexTextFileInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexReasoningFeatures,
		SupportedSamplingParameters: deepSeekVertexStandardSampling,
		Description:                 "DeepSeek V3.1 on Vertex AI MaaS with long context and reasoning-oriented chat capabilities.",
	},
	// DeepSeek R1 - Input: $1.35 / million tokens, Output: $5.40 / million tokens
	"deepseek-ai/deepseek-r1-0528-maas": {
		Ratio:                       1.35 * ratio.MilliTokensUsd, // Input price: $1.35 per million tokens
		CompletionRatio:             5.40 / 1.35,                 // Output/Input ratio: $5.40 / $1.35 = 4.0
		ContextLength:               163840,
		MaxOutputTokens:             32768,
		InputModalities:             deepSeekVertexTextFileInputs,
		OutputModalities:            deepSeekVertexTextOutputs,
		SupportedFeatures:           deepSeekVertexReasoningFeatures,
		SupportedSamplingParameters: deepSeekVertexReasoningSampling,
		Description:                 "DeepSeek R1-0528 on Vertex AI MaaS optimized for reasoning with restricted sampling controls.",
	},
}

// ModelList derived from ModelRatios keys
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
