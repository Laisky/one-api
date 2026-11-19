package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// qwenTranslationModels returns independently owned qwen translation defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func qwenTranslationModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen-mt-plus": {
			Ratio:                       nativeRate(1.8),
			CompletionRatio:             (5.4 * 1.0) / 1.8,
			CachedInputRatio:            0,
			ContextLength:               16384,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: nil,
			Quantization:                "",
			Description:                 "qwen-mt-plus translation model; Beijing standard CNY 1.8/5.4 per million input/output tokens, verified 2026-10-12. No context caching or tool/structured-output support. Requires a single user turn and translation_options; this retained catalogue entry does not add those provider-specific request fields. Other regions and negotiated tariffs require overrides.",
			SupportedFeatures:           nil,
		},
		"qwen-mt-turbo": {
			Ratio:                       nativeRate(0.72),
			CompletionRatio:             2.79,
			CachedInputRatio:            nativeRate(0.144),
			ContextLength:               4096,
			MaxOutputTokens:             2048,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "bf16",
			Description:                 "Qwen MT Turbo on Bailian: cost-efficient machine-translation model.",
		},
	}
}
