package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gemmaModelRatios contains complete gemma catalog entries.
var gemmaModelRatios = map[string]adaptor.ModelConfig{
	"gemma-2-2b-it": {
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             1.4,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputTextOnly...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeaturesGemma...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingGemma...),
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-2b-it",
		Description:                 "Gemma 2 2B instruction-tuned open-weight model served via the Gemini API.",
	},
	"gemma-2-9b-it": {
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             1.4,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputTextOnly...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeaturesGemma...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingGemma...),
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-9b-it",
		Description:                 "Gemma 2 9B instruction-tuned open-weight model served via the Gemini API.",
	},
	"gemma-2-27b-it": {
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             1.4,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputTextOnly...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeaturesGemma...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingGemma...),
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-27b-it",
		Description:                 "Gemma 2 27B instruction-tuned open-weight model served via the Gemini API.",
	},
	"gemma-3-27b-it": {
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             1.4,
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputTextImageFile...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeaturesGemma...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingGemma...),
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-3-27b-it",
		Description:                 "Gemma 3 27B instruction-tuned open-weight multimodal model served via the Gemini API.",
	},
}
