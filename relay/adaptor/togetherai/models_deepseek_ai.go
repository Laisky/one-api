package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// deepseek_aiModels returns the deepseek_ai model defaults for togetherai.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func deepseek_aiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"deepseek-ai/DeepSeek-R1": {
			Ratio:                       nativeRate(3),
			CompletionRatio:             2.3333333333333335,
			ContextLength:               128000,
			MaxOutputTokens:             32768,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "seed", "max_tokens"},
			Quantization:                "fp8",
			HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
			Description:                 "DeepSeek-R1 671B full reasoning model with extended CoT; restrict temperature to 0.5-0.7. (RETIRED 2026-05-14; use deepseek-ai/DeepSeek-V4-Flash)",
		},
		"deepseek-ai/DeepSeek-V3.1": {
			Ratio:                       nativeRate(0.6),
			CompletionRatio:             2.8333333333333335,
			ContextLength:               128000,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp8",
			HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
			Description:                 "DeepSeek V3.1 MoE (671B/37B active) with 128K context. (RETIRED 2026-05-14)",
		},
		"deepseek-ai/DeepSeek-V4-Flash-0731": {
			Ratio:             nativeRate(0.14),
			CompletionRatio:   2,
			CachedInputRatio:  nativeRate(0.03),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
			Description:       "deepseek-ai/DeepSeek-V4-Flash-0731 on togetherai; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-ai/DeepSeek-V4-Pro": {
			Ratio:                       nativeRate(1.74),
			CompletionRatio:             2,
			CachedInputRatio:            nativeRate(0.2),
			ContextLength:               512000,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Pro",
			Description:                 "DeepSeek V4-Pro flagship MoE with 512K context and prompt-cache pricing.",
		},
		"deepseek-ai/DeepSeek-V4-Pro-0813": {
			Ratio:             nativeRate(1.32),
			CompletionRatio:   3,
			CachedInputRatio:  nativeRate(0.13),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
			Description:       "deepseek-ai/DeepSeek-V4-Pro-0813 on togetherai; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-ai/DeepSeek-V4.1-Flash": {
			Ratio:             nativeRate(0.3),
			CompletionRatio:   4,
			CachedInputRatio:  nativeRate(0.006),
			ContextLength:     1000000,
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
			Description:       "deepseek-ai/DeepSeek-V4.1-Flash on togetherai; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
