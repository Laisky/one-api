// Package openai provides model pricing constants for OpenAI GPT-OSS models in Vertex AI.
package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	vertexOpenAIMaaSTextInputs          = []string{"text"}
	vertexOpenAIMaaSTextOutputs         = []string{"text"}
	vertexOpenAIGPTOSSReasoningFeatures = []string{"reasoning"}
	vertexOpenAIGPTOSS120BFeatures      = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	vertexOpenAIGPTOSSSamplingParams    = []string{"stop", "seed", "max_tokens"}
)

// ModelRatios contains pricing information for OpenAI GPT-OSS models
var ModelRatios = map[string]adaptor.ModelConfig{
	"openai/gpt-oss-20b-maas": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 per million tokens input
		CompletionRatio:             0.60 / 0.15,                 // Output/Input ratio = $0.60 / $0.15 = 4
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             vertexOpenAIMaaSTextInputs,
		OutputModalities:            vertexOpenAIMaaSTextOutputs,
		SupportedFeatures:           vertexOpenAIGPTOSSReasoningFeatures,
		SupportedSamplingParameters: vertexOpenAIGPTOSSSamplingParams,
		Quantization:                "fp4",
		HuggingFaceID:               "openai/gpt-oss-20b",
		Description:                 "OpenAI gpt-oss-20b MaaS on Vertex AI, an open-weight reasoning model with text-only I/O.",
	},
	"openai/gpt-oss-120b-maas": {
		Ratio:                       0.075 * ratio.MilliTokensUsd, // $0.075 per million tokens input
		CompletionRatio:             0.30 / 0.075,                 // Output/Input ratio = $0.30 / $0.075 = 4
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             vertexOpenAIMaaSTextInputs,
		OutputModalities:            vertexOpenAIMaaSTextOutputs,
		SupportedFeatures:           vertexOpenAIGPTOSS120BFeatures,
		SupportedSamplingParameters: vertexOpenAIGPTOSSSamplingParams,
		Quantization:                "fp4",
		HuggingFaceID:               "openai/gpt-oss-120b",
		Description:                 "OpenAI gpt-oss-120b MaaS on Vertex AI, an open-weight long-context reasoning model.",
	},
}

// ModelList contains all OpenAI GPT-OSS models supported by VertexAI
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
