// Package qwen provides model pricing constants for Qwen models in Vertex AI.
package qwen

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	qwenTextInputs            = []string{"text"}
	qwenTextOutputs           = []string{"text"}
	qwenSamplingParams        = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}
	qwenChatFeatures          = []string{"tools", "structured_outputs"}
	qwenChatReasoningFeatures = []string{"tools", "structured_outputs", "reasoning"}
)

// ModelRatios contains pricing information for Qwen models served on Vertex AI Model-as-a-Service.
// Sources (retrieved 2026-05-19):
//   - https://discuss.google.dev/t/now-ga-openais-gpt-oss-qwen3-models-on-vertex-ai-as-open-model-apis/253945
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/qwen/qwen3-next-instruct
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/qwen/qwen3-next-thinking
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/qwen/qwen3-coder
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/qwen/qwen3-235b
//   - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/maas/capabilities/thinking
//   - https://cloudprice.net/models/vertex_ai/qwen/qwen3-next-80b-a3b-instruct-maas
//   - https://cloudprice.net/models/vertex_ai/qwen/qwen3-next-80b-a3b-thinking-maas
//   - https://cloudprice.net/models/vertex_ai/qwen/qwen3-coder-480b-a35b-instruct-maas
//   - https://cloudprice.net/models/vertex_ai/qwen/qwen3-235b-a22b-instruct-2507-maas
var ModelRatios = map[string]adaptor.ModelConfig{
	// Qwen3-Next-80B-A3B-Instruct - Vertex GA 2025-09-15; global endpoint.
	"qwen/qwen3-next-80b-a3b-instruct-maas": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 per million input tokens
		CompletionRatio:             1.20 / 0.15,                 // Output/Input = $1.20 / $0.15 = 8
		ContextLength:               262144,
		MaxOutputTokens:             262144,
		InputModalities:             qwenTextInputs,
		OutputModalities:            qwenTextOutputs,
		SupportedFeatures:           qwenChatFeatures,
		SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID:               "Qwen/Qwen3-Next-80B-A3B-Instruct",
		Description:                 "Alibaba Qwen3-Next 80B/A3B instruct mixture-of-experts on Vertex AI MaaS (262K context).",
	},
	// Qwen3-Next-80B-A3B-Thinking - Vertex GA 2025-09-15; global endpoint.
	// Thinking is always-on (no reasoning_effort or thinking budget exposed by Vertex MaaS or the
	// upstream Qwen API); reasoning surfaces in `reasoning_content`, response in `content`.
	"qwen/qwen3-next-80b-a3b-thinking-maas": {
		Ratio:                       0.15 * ratio.MilliTokensUsd, // $0.15 per million input tokens
		CompletionRatio:             1.20 / 0.15,                 // Output/Input = $1.20 / $0.15 = 8
		ContextLength:               262144,
		MaxOutputTokens:             262144,
		InputModalities:             qwenTextInputs,
		OutputModalities:            qwenTextOutputs,
		SupportedFeatures:           qwenChatReasoningFeatures,
		SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID:               "Qwen/Qwen3-Next-80B-A3B-Thinking",
		Description:                 "Alibaba Qwen3-Next 80B/A3B thinking mixture-of-experts on Vertex AI MaaS (262K context, always-on reasoning).",
	},
	// Qwen3-Coder-480B-A35B-Instruct - Vertex GA 2025-08-13 in us-south1 and global.
	"qwen/qwen3-coder-480b-a35b-instruct-maas": {
		Ratio:                       1.00 * ratio.MilliTokensUsd, // $1.00 per million input tokens (Vertex AI MaaS GA rate)
		CompletionRatio:             4.00 / 1.00,                 // Output/Input = $4.00 / $1.00 = 4
		ContextLength:               262144,
		MaxOutputTokens:             65536,
		InputModalities:             qwenTextInputs,
		OutputModalities:            qwenTextOutputs,
		SupportedFeatures:           qwenChatFeatures,
		SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID:               "Qwen/Qwen3-Coder-480B-A35B-Instruct",
		Description:                 "Alibaba Qwen3 Coder 480B/A35B instruct on Vertex AI MaaS (262K context, 65K max output).",
	},
	// Qwen3-235B-A22B-Instruct-2507 - Vertex GA 2025-08-13 in us-south1 and global.
	"qwen/qwen3-235b-a22b-instruct-2507-maas": {
		Ratio:                       0.25 * ratio.MilliTokensUsd, // $0.25 per million input tokens (Vertex AI MaaS GA rate)
		CompletionRatio:             1.00 / 0.25,                 // Output/Input = $1.00 / $0.25 = 4
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             qwenTextInputs,
		OutputModalities:            qwenTextOutputs,
		SupportedFeatures:           qwenChatFeatures,
		SupportedSamplingParameters: qwenSamplingParams,
		HuggingFaceID:               "Qwen/Qwen3-235B-A22B-Instruct-2507",
		Description:                 "Alibaba Qwen3 235B/A22B instruct (July 2025 hybrid-thinking variant) on Vertex AI MaaS (262K context).",
	},
}

// ModelList contains all Qwen models supported by VertexAI
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
