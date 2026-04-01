// Package qwen provides model pricing constants for Qwen models in Vertex AI.
package qwen

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains pricing information for Qwen models
var ModelRatios = map[string]adaptor.ModelConfig{
	"qwen/qwen3-next-80b-a3b-instruct-maas": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 per million tokens input
		CompletionRatio: 1.20 / 0.15,                 // Output/Input ratio = $1.20 / $0.15 = 8
	},
	"qwen/qwen3-next-80b-a3b-thinking-maas": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 per million tokens input
		CompletionRatio: 1.20 / 0.15,                 // Output/Input ratio = $1.20 / $0.15 = 8
	},
	"qwen/qwen3-coder-480b-a35b-instruct-maas": {
		Ratio:           0.22 * ratio.MilliTokensUsd, // $0.22 per million tokens input
		CompletionRatio: 1.80 / 0.22,                 // Output/Input ratio = $1.80 / $0.22
	},
	"qwen/qwen3-235b-a22b-instruct-2507-maas": {
		Ratio:           0.22 * ratio.MilliTokensUsd, // $0.22 per million tokens input
		CompletionRatio: 0.88 / 0.22,                 // Output/Input ratio = $0.88 / $0.22 = 4
	},
}

// ModelList contains all Qwen models supported by VertexAI
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
