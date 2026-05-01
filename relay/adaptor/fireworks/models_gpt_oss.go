package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gptOssModels contains OpenAI gpt-oss open-weight models hosted on Fireworks
// (gpt-oss-120b, gpt-oss-20b). Sources:
//   - https://fireworks.ai/models/fireworks/gpt-oss-120b
//   - https://fireworks.ai/models/fireworks/gpt-oss-20b
var gptOssModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/gpt-oss-120b": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             0.60 / 0.15,
		CachedInputRatio:            0.01 * ratio.MilliTokensUsd,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwReasoningSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "openai/gpt-oss-120b",
		Description:                 "OpenAI gpt-oss-120b open-weight MoE reasoning model that fits on a single H100 GPU for high-reasoning use-cases.",
	},
	"accounts/fireworks/models/gpt-oss-20b": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             0.30 / 0.07,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwReasoningSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "openai/gpt-oss-20b",
		Description:                 "OpenAI gpt-oss-20b compact open-weight MoE reasoning model for lower-latency agentic and developer use cases.",
	},
}
