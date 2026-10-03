package alibailian

import "github.com/Laisky/one-api/relay/adaptor"

// deepseekModels returns the deepseek model defaults for alibailian.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func deepseekModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"deepseek-r1": {
			Ratio:                       nativeRate(4),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.8),
			ContextLength:               65536,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			MaxReasoningTokens:          32768,
			Quantization:                "fp8",
			HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
			Description:                 "deepseek-r1 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-r1-0528": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "deepseek-r1-0528 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-r1-distill-qwen-1.5b": {
			Ratio:           0,
			CompletionRatio: 1,
			Description:     "deepseek-r1-distill-qwen-1.5b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-r1-distill-qwen-14b": {
			Ratio:           nativeRate(1),
			CompletionRatio: 3,
			Description:     "deepseek-r1-distill-qwen-14b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-r1-distill-qwen-32b": {
			Ratio:           nativeRate(2),
			CompletionRatio: 3,
			Description:     "deepseek-r1-distill-qwen-32b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-r1-distill-qwen-7b": {
			Ratio:           nativeRate(0.5),
			CompletionRatio: 2,
			Description:     "deepseek-r1-distill-qwen-7b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v3": {
			Ratio:                       nativeRate(2),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.4),
			ContextLength:               65536,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp8",
			HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
			Description:                 "deepseek-v3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v3.1": {
			Ratio:           nativeRate(4),
			CompletionRatio: 3,
			Description:     "deepseek-v3.1 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v3.2": {
			Ratio:           nativeRate(2),
			CompletionRatio: 1.5,
			Description:     "deepseek-v3.2 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v3.2-exp": {
			Ratio:           nativeRate(2),
			CompletionRatio: 1.5,
			Description:     "deepseek-v3.2-exp on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v4-flash": {
			Ratio:           nativeRate(1),
			CompletionRatio: 2,
			Description:     "deepseek-v4-flash on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v4-flash-0731": {
			Ratio:           nativeRate(1.5),
			CompletionRatio: 3,
			TimeWindows: []adaptor.TimeWindow{adaptor.TimeWindow{
				Name:     "beijing-peak",
				TimeZone: "Asia/Shanghai",
				Ranges: []adaptor.ClockRange{adaptor.ClockRange{
					Start: "08:00",
					End:   "22:00",
				},
				},
				Overlay: adaptor.ModelConfig{
					Ratio:           nativeRate(3),
					CompletionRatio: 3,
				},
			},
			},
			Description: "deepseek-v4-flash-0731 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v4-pro": {
			Ratio:           nativeRate(12),
			CompletionRatio: 2,
			Description:     "deepseek-v4-pro on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v4-pro-0813": {
			Ratio:           nativeRate(4.5),
			CompletionRatio: 3,
			TimeWindows: []adaptor.TimeWindow{adaptor.TimeWindow{
				Name:     "beijing-peak",
				TimeZone: "Asia/Shanghai",
				Ranges: []adaptor.ClockRange{adaptor.ClockRange{
					Start: "08:00",
					End:   "22:00",
				},
				},
				Overlay: adaptor.ModelConfig{
					Ratio:           nativeRate(9),
					CompletionRatio: 3,
				},
			},
			},
			Description: "deepseek-v4-pro-0813 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"deepseek-v4.1-flash": {
			Ratio:           nativeRate(1),
			CompletionRatio: 4,
			TimeWindows: []adaptor.TimeWindow{adaptor.TimeWindow{
				Name:     "beijing-peak",
				TimeZone: "Asia/Shanghai",
				Ranges: []adaptor.ClockRange{adaptor.ClockRange{
					Start: "08:00",
					End:   "22:00",
				},
				},
				Overlay: adaptor.ModelConfig{
					Ratio:           nativeRate(2),
					CompletionRatio: 4,
				},
			},
			},
			Description: "deepseek-v4.1-flash on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
