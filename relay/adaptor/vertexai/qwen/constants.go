// Package qwen provides model pricing constants for Qwen models in Vertex AI.
package qwen

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains pricing information for Qwen models
var ModelRatios = map[string]adaptor.ModelConfig{
	"qwen/qwen3-coder-480b-a35b-instruct-maas": {
		Ratio:           1.00 * ratio.MilliTokensUsd, // $1.00 per million tokens input
		CompletionRatio: 4.00 * ratio.MilliTokensUsd, // $4.00 per million tokens output
	},
	"qwen/qwen3-235b-a22b-instruct-2507-maas": {
		Ratio:           0.25 * ratio.MilliTokensUsd, // $0.25 per million tokens input
		CompletionRatio: 1.00 * ratio.MilliTokensUsd, // $1.00 per million tokens output
	},
	"qwen/qwen3-next-80b-a3b-instruct-maas": {
		Ratio:           0.15 * ratio.MilliTokensUsd, // $0.15 per million tokens input
		CompletionRatio: 1.20 * ratio.MilliTokensUsd, // $1.20 per million tokens output
	},
}

// ModelList contains all Qwen models supported by VertexAI
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
