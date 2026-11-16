package baiduv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// glmModels returns independently owned glm defaults for baiduv2.
// It takes no arguments and returns the complete family catalog.
// Sources checked 2026-10-12 and 2026-11-16:
// https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j
// https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya
func glmModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"GLM-5.3": {
			Ratio:            nativeRate(8),
			CompletionRatio:  3.5,
			CachedInputRatio: nativeRate(2),
			TimeWindows: []adaptor.TimeWindow{{
				Name:     "qianfan-holiday-peak",
				TimeZone: "Asia/Shanghai",
				Ranges: []adaptor.ClockRange{{
					Start: "08:00",
					End:   "22:00",
				},
				},
				DateFrom: "2026-09-24",
				DateTo:   "2026-10-08",
				Overlay: adaptor.ModelConfig{
					Ratio:            nativeRate(4.8),
					CompletionRatio:  3.5000000000000004,
					CachedInputRatio: nativeRate(1.2),
				},
			},
				{
					Name:     "qianfan-holiday-offpeak",
					TimeZone: "Asia/Shanghai",
					Ranges: []adaptor.ClockRange{{
						Start: "22:00",
						End:   "08:00",
					},
					},
					DateFrom: "2026-09-24",
					DateTo:   "2026-10-08",
					Overlay: adaptor.ModelConfig{
						Ratio:            nativeRate(4.8),
						CompletionRatio:  3.5000000000000004,
						CachedInputRatio: nativeRate(1.2),
					},
				},
				{
					Name:     "qianfan-peak",
					TimeZone: "Asia/Shanghai",
					Ranges: []adaptor.ClockRange{{
						Start: "08:00",
						End:   "22:00",
					},
					},
					Overlay: adaptor.ModelConfig{
						Ratio:            nativeRate(8),
						CompletionRatio:  3.5,
						CachedInputRatio: nativeRate(2),
					},
				},
			},
			Description: "GLM-5.3 on baiduv2; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.3-flash": {
			Ratio: nativeRate(.8), CompletionRatio: 3.5, CachedInputRatio: nativeRate(.23),
			ContextLength: 1000000, MaxOutputTokens: 131072,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "GLM-5.3-Flash on Qianfan v2. Published lowercase API ID; 1M context and 131072 total output tokens including reasoning. CNY 0.80 input, 0.23 cached input and 2.80 output per million tokens. No Z.AI-specific reasoning-effort, vision or tool controls are inferred.",
		},
		"glm-5.3": {
			Ratio: nativeRate(8), CompletionRatio: 3.5, CachedInputRatio: nativeRate(2),
			ContextLength: 1000000, MaxOutputTokens: 131072,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "GLM-5.3 on Qianfan v2 using the documented lowercase model parameter. 1M context and 131072 total output tokens. Current Standard CNY 8 input, 2 cached input and 28 output per million; the earlier holiday promotion is over. The retained uppercase entry is historical compatibility metadata, not an automatic request rewrite.",
		},
		"glm-5.2": {
			Ratio: nativeRate(8), CompletionRatio: 3.5, CachedInputRatio: nativeRate(2),
			ContextLength: 1000000, MaxOutputTokens: 131072,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "GLM-5.2 on Qianfan v2 using the exact lowercase API ID. 1M context, 131072 total output tokens, CNY 8 input, 2 cached input and 28 output per million. Host-specific tool and effort vocabularies are not inferred from another provider.",
		},
	}
}
