package stepfun

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// StepFun (阶跃星辰) hosts the Step-1 / Step-2 / Step-1V chat model families.
// Reference docs:
//   - https://platform.stepfun.com/docs/llm (model overview)
//   - https://platform.stepfun.com/docs/pricing (RMB rates per million tokens)
//
// All Step-* SKUs are closed-weight; HuggingFaceID and Quantization stay empty.
// step-1v-* are vision-capable; step-1x-medium accepts text + image as well.

// stepfunTextInputs is the input modality set for text-only Step-* models.
var stepfunTextInputs = []string{"text"}

// stepfunVisionInputs is the modality set for vision-capable Step-1V / 1X models.
var stepfunVisionInputs = []string{"text", "image"}

// stepfunTextOutputs is the text-only output modality used by Step models.
var stepfunTextOutputs = []string{"text"}

// stepfunChatFeatures captures tool / JSON capabilities offered by the
// OpenAI-compatible StepFun chat API.
var stepfunChatFeatures = []string{"tools", "json_mode"}

// stepfunSamplingParams lists sampling parameters accepted by StepFun chat
// completion endpoints.
var stepfunSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on StepFun pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// StepFun Models - estimated pricing
	"step-1-8k": {
		Ratio:                       1.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 closed-weight chat model with 8k context.",
	},
	"step-1-32k": {
		Ratio:                       2.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 closed-weight chat model with 32k context.",
	},
	"step-1-128k": {
		Ratio:                       4.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 long-context closed-weight chat model with 128k context.",
	},
	"step-1-256k": {
		Ratio:                       8.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               262144,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 long-context closed-weight chat model with 256k context.",
	},
	"step-1-flash": {
		Ratio:                       0.5 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 Flash low-latency closed-weight chat model.",
	},
	"step-2-16k": {
		Ratio:                       3.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-2 second-generation closed-weight chat model with 16k context.",
	},
	"step-1v-8k": {
		Ratio:                       1.5 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1V vision-capable chat model with 8k context.",
	},
	"step-1v-32k": {
		Ratio:                       3.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1V vision-capable chat model with 32k context.",
	},
	"step-1x-medium": {
		Ratio:                       2.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1X mid-tier multimodal chat model.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// StepFunToolingDefaults notes that StepFun publishes model pricing only; no tool-specific fees are documented (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://stepfun.ai/
var StepFunToolingDefaults = adaptor.ChannelToolConfig{}
