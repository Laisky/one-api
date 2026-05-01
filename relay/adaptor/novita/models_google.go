package novita

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// googleGemmaModelRatios contains Google Gemma instruction-tuned chat models.
// Gemma 3 has vision variants upstream but Novita exposes them as text-only
// chat endpoints on the public pricing page.
// Source: https://novita.ai/llm-api  (retrieved 2026-04-28)
var googleGemmaModelRatios = map[string]adaptor.ModelConfig{
	"google/gemma-3-27b-it": {
		Ratio:                       0.119 * ratio.MilliTokensUsd,
		CompletionRatio:             1.68067226891,
		ContextLength:               98304,
		MaxOutputTokens:             16384,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-3-27b-it",
		Description:                 "Google Gemma 3 27B instruction-tuned chat model with 96K context.",
	},
	"google/gemma-4-26b-a4b-it": {
		Ratio:                       0.13 * ratio.MilliTokensUsd,
		CompletionRatio:             3.07692307692,
		ContextLength:               262144,
		MaxOutputTokens:             131072,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-4-26b-a4b-it",
		Description:                 "Google Gemma 4 26B/A4B MoE instruct chat model with 256K context.",
	},
	"google/gemma-4-31b-it": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             2.85714285714,
		ContextLength:               262144,
		MaxOutputTokens:             131072,
		InputModalities:             novitaTextOnlyModalities,
		OutputModalities:            novitaTextOnlyModalities,
		SupportedFeatures:           novitaChatFeatures,
		SupportedSamplingParameters: novitaSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-4-31b-it",
		Description:                 "Google Gemma 4 31B instruct chat model with 256K context.",
	},
}
