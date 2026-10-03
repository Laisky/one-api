package cloudflare

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var cloudflareToolsAndReasoningFeatures = []string{"tools", "reasoning"}

// init applies the Cloudflare Workers AI catalog changes published in August
// 2026 and rebuilds ModelList after removing models whose deprecation date has
// passed.
func init() {
	ModelRatios["@cf/deepseek-ai/deepseek-v4-flash-0731"] = adaptor.ModelConfig{
		Ratio:                       0.440 * ratio.MilliTokensUsd,
		CompletionRatio:             1.320 / 0.440,
		CachedInputRatio:            0.014 * ratio.MilliTokensUsd,
		ContextLength:               1_048_576,
		InputModalities:             cfTextInputs,
		OutputModalities:            cfTextOutputs,
		SupportedFeatures:           cloudflareToolsAndReasoningFeatures,
		SupportedSamplingParameters: cfBasicSamplingParams,
		Description:                 "DeepSeek V4 Flash 0731 long-context reasoning and function-calling model on Cloudflare Workers AI.",
	}
	ModelRatios["@cf/deepseek-ai/deepseek-v4-pro-0813"] = adaptor.ModelConfig{
		Ratio:                       1.320 * ratio.MilliTokensUsd,
		CompletionRatio:             3.960 / 1.320,
		CachedInputRatio:            0.044 * ratio.MilliTokensUsd,
		ContextLength:               1_048_576,
		InputModalities:             cfVisionInputs,
		OutputModalities:            cfTextOutputs,
		SupportedFeatures:           cloudflareToolsAndReasoningFeatures,
		SupportedSamplingParameters: cfBasicSamplingParams,
		Description:                 "DeepSeek V4 Pro 0813 long-context multimodal reasoning and function-calling model on Cloudflare Workers AI.",
	}
	ModelRatios["@cf/qwen/qwen3.8-27b"] = adaptor.ModelConfig{
		Ratio:                       0.450 * ratio.MilliTokensUsd,
		CompletionRatio:             3.200 / 0.450,
		ContextLength:               262_144,
		InputModalities:             cfVisionInputs,
		OutputModalities:            cfTextOutputs,
		SupportedFeatures:           cloudflareToolsAndReasoningFeatures,
		SupportedSamplingParameters: cfBasicSamplingParams,
		Description:                 "Qwen 3.8 27B multimodal reasoning and function-calling model on Cloudflare Workers AI.",
	}

	for _, modelName := range deprecatedCloudflareModels20260530 {
		delete(ModelRatios, modelName)
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}

// deprecatedCloudflareModels20260530 contains models whose Cloudflare Workers AI
// deprecation date passed on May 30, 2026.
var deprecatedCloudflareModels20260530 = []string{
	"@cf/moonshotai/kimi-k2.5",
	"@hf/meta-llama/meta-llama-3-8b-instruct",
	"@cf/meta/llama-3-8b-instruct",
	"@cf/meta/llama-3-8b-instruct-awq",
	"@cf/meta/llama-3.1-8b-instruct",
	"@cf/meta/llama-3.1-8b-instruct-awq",
	"@cf/meta/llama-3.1-70b-instruct",
	"@cf/meta/llama-2-7b-chat-int8",
	"@cf/meta/llama-2-7b-chat-fp16",
	"@cf/mistral/mistral-7b-instruct-v0.1",
	"@hf/mistral/mistral-7b-instruct-v0.2",
	"@hf/google/gemma-7b-it",
	"@cf/google/gemma-3-12b-it",
	"@hf/nousresearch/hermes-2-pro-mistral-7b",
	"@cf/microsoft/phi-2",
	"@cf/defog/sqlcoder-7b-2",
	"@cf/unum/uform-gen2-qwen-500m",
	"@cf/facebook/bart-large-cnn",
}
