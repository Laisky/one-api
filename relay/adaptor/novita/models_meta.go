package novita

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// metaLlamaModelRatios contains Meta Llama family entries served by Novita.
// Llama 3 family uses bf16 weights; Llama 4 Maverick is served at FP8 (the
// "fp8" suffix on the model id reflects Novita's serving precision).
// Source: https://novita.ai/llm-api  (retrieved 2026-04-28)
var metaLlamaModelRatios = map[string]adaptor.ModelConfig{
	"meta-llama/llama-3.1-8b-instruct": {
		Ratio:                       0.02 * ratio.MilliTokensUsd,
		CompletionRatio:             2.5,
		ContextLength:               16384,
		MaxOutputTokens:             16384,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Llama-3.1-8B-Instruct",
		Description:                 "Meta Llama 3.1 8B instruct chat model with 16K context on Novita.",
	},
	"meta-llama/llama-3.3-70b-instruct": {
		Ratio:                       0.135 * ratio.MilliTokensUsd,
		CompletionRatio:             2.96296296296,
		ContextLength:               131072,
		MaxOutputTokens:             120000,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Llama-3.3-70B-Instruct",
		Description:                 "Meta Llama 3.3 70B instruct chat model with 128K context.",
	},
	"meta-llama/llama-3-70b-instruct": {
		Ratio:                       0.51 * ratio.MilliTokensUsd,
		CompletionRatio:             1.45098039216,
		ContextLength:               8192,
		MaxOutputTokens:             8000,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Meta-Llama-3-70B-Instruct",
		Description:                 "Meta Llama 3 70B instruct chat model (8K context legacy).",
	},
	"meta-llama/llama-3-8b-instruct": {
		Ratio:                       0.04 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Meta-Llama-3-8B-Instruct",
		Description:                 "Meta Llama 3 8B instruct chat model with 8K context.",
	},
	"meta-llama/llama-4-maverick-17b-128e-instruct-fp8": {
		Ratio:                       0.27 * ratio.MilliTokensUsd,
		CompletionRatio:             3.14814814815,
		ContextLength:               1048576,
		MaxOutputTokens:             8192,
		InputModalities:             novitaTextImageInModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "meta-llama/Llama-4-Maverick-17B-128E-Instruct",
		Description:                 "Meta Llama 4 Maverick 17B/128E instruct MoE served at FP8 with 1M context and image input.",
	},
	"meta-llama/llama-4-scout-17b-16e-instruct": {
		Ratio:                       0.18 * ratio.MilliTokensUsd,
		CompletionRatio:             3.27777777778,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             novitaTextImageInModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Llama-4-Scout-17B-16E-Instruct",
		Description:                 "Meta Llama 4 Scout 17B/16E instruct MoE with 128K context and image input.",
	},
}
