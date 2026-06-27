// Package openai provides model pricing constants for OpenAI GPT-OSS models in Vertex AI.
package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	vertexOpenAIMaaSTextInputs       = []string{"text"}
	vertexOpenAIMaaSTextOutputs      = []string{"text"}
	vertexOpenAIGPTOSS20BFeatures    = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	vertexOpenAIGPTOSS120BFeatures   = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	vertexOpenAIGPTOSSSamplingParams = []string{"temperature", "top_p", "stop", "seed", "max_tokens"}
	vertexOpenAIGPTOSSReasoningTiers = []string{"low", "medium", "high"}
)

// ModelRatios contains pricing information for OpenAI GPT-OSS models on Vertex AI MaaS.
// Sources (retrieved 2026-05-19):
//   - https://discuss.google.dev/t/now-ga-openais-gpt-oss-qwen3-models-on-vertex-ai-as-open-model-apis/253945
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/capabilities/thinking
//   - https://cloudprice.net/models/vertex_ai/openai/gpt-oss-20b-maas
//   - https://cloudprice.net/models/vertex_ai/openai/gpt-oss-120b-maas
//
// Both variants surface thinking in `reasoning_content` and accept the standard OpenAI
// `reasoning_effort` parameter with low/medium/high tiers per the Vertex thinking capability doc.
var ModelRatios = map[string]adaptor.ModelConfig{
	"openai/gpt-oss-20b-maas": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,  // $0.07 per million input tokens
		CompletionRatio:             0.25 / 0.07,                  // Output/Input = $0.25 / $0.07 = 3.5714
		CachedInputRatio:            0.007 * ratio.MilliTokensUsd, // $0.007 per million cached input tokens
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             vertexOpenAIMaaSTextInputs,
		OutputModalities:            vertexOpenAIMaaSTextOutputs,
		SupportedFeatures:           vertexOpenAIGPTOSS20BFeatures,
		SupportedSamplingParameters: vertexOpenAIGPTOSSSamplingParams,
		SupportedReasoningEfforts:   vertexOpenAIGPTOSSReasoningTiers,
		DefaultReasoningEffort:      "medium",
		Quantization:                "fp4",
		HuggingFaceID:               "openai/gpt-oss-20b",
		Description:                 "OpenAI gpt-oss-20b MaaS on Vertex AI: open-weight reasoning model (Apache 2.0) with text-only I/O.",
	},
	"openai/gpt-oss-120b-maas": {
		Ratio:                       0.09 * ratio.MilliTokensUsd, // $0.09 per million input tokens
		CompletionRatio:             0.36 / 0.09,                 // Output/Input = $0.36 / $0.09 = 4
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             vertexOpenAIMaaSTextInputs,
		OutputModalities:            vertexOpenAIMaaSTextOutputs,
		SupportedFeatures:           vertexOpenAIGPTOSS120BFeatures,
		SupportedSamplingParameters: vertexOpenAIGPTOSSSamplingParams,
		SupportedReasoningEfforts:   vertexOpenAIGPTOSSReasoningTiers,
		DefaultReasoningEffort:      "medium",
		Quantization:                "fp4",
		HuggingFaceID:               "openai/gpt-oss-120b",
		Description:                 "OpenAI gpt-oss-120b MaaS on Vertex AI: open-weight long-context reasoning model (Apache 2.0).",
	},
}

// ModelList contains all OpenAI GPT-OSS models supported by VertexAI
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
