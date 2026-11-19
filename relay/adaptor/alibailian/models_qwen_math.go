package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// qwenMathModels returns independently owned qwen math defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func qwenMathModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen-math-plus": {
			Ratio:           nativeRate(4),
			CompletionRatio: 3,
			Description:     "qwen-math-plus on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen-math-turbo": {
			Ratio:           nativeRate(2),
			CompletionRatio: 3,
			Description:     "qwen-math-turbo on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwq-32b-preview": {
			Ratio:                       nativeRate(2.06),
			CompletionRatio:             2.995,
			CachedInputRatio:            nativeRate(0.412),
			ContextLength:               32768,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"seed", "max_tokens"},
			MaxReasoningTokens:          38912,
			Quantization:                "bf16",
			HuggingFaceID:               "Qwen/QwQ-32B-Preview",
			Description:                 "QwQ-32B-Preview on Bailian: experimental open-weight reasoning model.",
		},
		"qwq-plus": {
			Ratio:                       nativeRate(1.6),
			CompletionRatio:             2.5,
			CachedInputRatio:            nativeRate(0.33),
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"seed", "max_tokens"},
			MaxReasoningTokens:          38912,
			Quantization:                "bf16",
			HuggingFaceID:               "Qwen/QwQ-32B",
			Description:                 "qwq-plus on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
