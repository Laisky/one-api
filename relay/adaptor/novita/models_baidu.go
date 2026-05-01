package novita

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// baiduErnieModelRatios contains Baidu ERNIE 4.5 family entries served by Novita.
// ERNIE 4.5 weights are released under Apache 2.0 on HuggingFace; Novita serves
// the standard instruct and thinking variants. Quantization defaults to bf16
// because Novita's public spec pages do not advertise low-precision serving.
// Source: https://novita.ai/llm-api  (retrieved 2026-04-28)
var baiduErnieModelRatios = map[string]adaptor.ModelConfig{
	"baidu/ernie-4.5-21B-a3b": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             4,
		ContextLength:               120000,
		MaxOutputTokens:             8000,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baidu/ERNIE-4.5-21B-A3B-PT",
		Description:                 "Baidu's 21B/A3B ERNIE 4.5 instruct MoE chat model with 120K context.",
	},
	"baidu/ernie-4.5-21B-a3b-thinking": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             4,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaReasoningFeatures,
		SupportedSamplingParameters: novitaReasoningSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baidu/ERNIE-4.5-21B-A3B-Thinking",
		Description:                 "Thinking-mode ERNIE 4.5 21B/A3B with 128K context for reasoning workloads.",
	},
	"baidu/ernie-4.5-300b-a47b-paddle": {
		Ratio:                       0.28 * ratio.MilliTokensUsd,
		CompletionRatio:             3.92857142857,
		ContextLength:               123000,
		MaxOutputTokens:             12000,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baidu/ERNIE-4.5-300B-A47B-PT",
		Description:                 "Flagship Baidu ERNIE 4.5 300B/A47B served via PaddlePaddle backend with 123K context.",
	},
	"baidu/ernie-4.5-vl-28b-a3b": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             4,
		ContextLength:               30000,
		MaxOutputTokens:             8000,
		InputModalities:             novitaTextImageInModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baidu/ERNIE-4.5-VL-28B-A3B-PT",
		Description:                 "ERNIE 4.5 vision-language 28B/A3B with text+image input and 30K context.",
	},
	"baidu/ernie-4.5-vl-28b-a3b-thinking": {
		Ratio:                       0.39 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             novitaTextImageInModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaReasoningFeatures,
		SupportedSamplingParameters: novitaReasoningSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baidu/ERNIE-4.5-VL-28B-A3B-Thinking",
		Description:                 "Thinking-mode ERNIE 4.5 vision-language 28B/A3B with 128K context.",
	},
	"baidu/ernie-4.5-vl-424b-a47b": {
		Ratio:                       0.42 * ratio.MilliTokensUsd,
		CompletionRatio:             2.97619047619,
		ContextLength:               123000,
		MaxOutputTokens:             16000,
		InputModalities:             novitaTextImageInModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baidu/ERNIE-4.5-VL-424B-A47B-PT",
		Description:                 "ERNIE 4.5 vision-language flagship 424B/A47B with 123K text+image context.",
	},
}
