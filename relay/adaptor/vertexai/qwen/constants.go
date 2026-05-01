// Package qwen provides model pricing constants for Qwen models in Vertex AI.
package qwen

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	qwenTextInputs              = []string{"text"}
	qwenTextOutputs             = []string{"text"}
	qwenSamplingParams          = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}
	qwenChatFeatures            = []string{"tools"}
	qwenChatReasoningFeatures   = []string{"tools", "reasoning"}
)

// ModelRatios contains pricing information for Qwen models served on Vertex AI Model-as-a-Service.
// Source: https://cloud.google.com/vertex-ai/generative-ai/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	"qwen/qwen3-next-80b-a3b-instruct-maas": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 per million tokens input
		CompletionRatio: 1.20 / 0.15,                 // Output/Input ratio = $1.20 / $0.15 = 8
		ContextLength:   262144, MaxOutputTokens: 32768,
		InputModalities: qwenTextInputs, OutputModalities: qwenTextOutputs,
		SupportedFeatures: qwenChatFeatures, SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-Next-80B-A3B-Instruct",
		Description:   "Alibaba Qwen3-Next 80B/A3B instruct mixture-of-experts on Vertex AI MaaS.",
	},
	"qwen/qwen3-next-80b-a3b-thinking-maas": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 per million tokens input
		CompletionRatio: 1.20 / 0.15,                 // Output/Input ratio = $1.20 / $0.15 = 8
		ContextLength:   262144, MaxOutputTokens: 32768,
		InputModalities: qwenTextInputs, OutputModalities: qwenTextOutputs,
		SupportedFeatures: qwenChatReasoningFeatures, SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-Next-80B-A3B-Thinking",
		Description:   "Alibaba Qwen3-Next 80B/A3B reasoning (thinking) mixture-of-experts on Vertex AI MaaS.",
	},
	"qwen/qwen3-coder-480b-a35b-instruct-maas": {
		Ratio:           0.22 * ratio.MilliTokensUsd, // $0.22 per million tokens input
		CompletionRatio: 1.80 / 0.22,                 // Output/Input ratio = $1.80 / $0.22
		ContextLength:   262144, MaxOutputTokens: 32768,
		InputModalities: qwenTextInputs, OutputModalities: qwenTextOutputs,
		SupportedFeatures: qwenChatFeatures, SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-Coder-480B-A35B-Instruct",
		Description:   "Alibaba Qwen3 Coder 480B/A35B instruct on Vertex AI MaaS.",
	},
	"qwen/qwen3-235b-a22b-instruct-2507-maas": {
		Ratio:           0.22 * ratio.MilliTokensUsd, // $0.22 per million tokens input
		CompletionRatio: 0.88 / 0.22,                 // Output/Input ratio = $0.88 / $0.22 = 4
		ContextLength:   262144, MaxOutputTokens: 32768,
		InputModalities: qwenTextInputs, OutputModalities: qwenTextOutputs,
		SupportedFeatures: qwenChatFeatures, SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-235B-A22B-Instruct-2507",
		Description:   "Alibaba Qwen3 235B/A22B instruct (July 2025) on Vertex AI MaaS.",
	},
}

// ModelList contains all Qwen models supported by VertexAI
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
