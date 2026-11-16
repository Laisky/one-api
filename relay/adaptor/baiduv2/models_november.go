package baiduv2

import "github.com/Laisky/one-api/relay/adaptor"

// novemberModels returns independent defaults for Qianfan's newly documented
// lowercase API IDs. It takes no arguments and returns a new model map.
// Sources checked 2026-11-16:
// https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j
// https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya
// https://cloud.baidu.com/product/qianfan_home/model_3.html
func novemberModels() map[string]adaptor.ModelConfig {
	models := map[string]adaptor.ModelConfig{
		"ernie-5.5-flash-preview": {
			Ratio: nativeRate(1), CompletionRatio: 4, CachedInputRatio: nativeRate(.1),
			ContextLength: 1_000_000, MaxOutputTokens: 1_000_000,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			TimeWindows: []adaptor.TimeWindow{
				{
					Name: "ernie55-preview-promotion-peak", TimeZone: "Asia/Shanghai", DateTo: "2026-12-01",
					Ranges:  []adaptor.ClockRange{{Start: "08:00", End: "22:00"}},
					Overlay: adaptor.ModelConfig{Ratio: nativeRate(1), CompletionRatio: 4, CachedInputRatio: nativeRate(.1)},
				},
				{
					Name: "ernie55-preview-promotion-offpeak", TimeZone: "Asia/Shanghai", DateTo: "2026-12-01",
					Ranges:  []adaptor.ClockRange{{Start: "22:00", End: "08:00"}},
					Overlay: adaptor.ModelConfig{Ratio: nativeRate(.5), CompletionRatio: 4, CachedInputRatio: nativeRate(.05)},
				},
				{
					Name: "qianfan-peak", TimeZone: "Asia/Shanghai",
					Ranges:  []adaptor.ClockRange{{Start: "08:00", End: "22:00"}},
					Overlay: adaptor.ModelConfig{Ratio: nativeRate(2), CompletionRatio: 4, CachedInputRatio: nativeRate(.2)},
				},
			},
			Description: "ERNIE 5.5 Flash Preview on Qianfan v2; 1M context and published total output ceiling, with 64K upstream default output. CNY/M input/cache/output 2/0.2/8 at 08:00-22:00 Asia/Shanghai and 1/0.1/4 otherwise. The 50% promotion ends at 2026-11-30 23:59:59 Asia/Shanghai; dated windows restore list prices at December 1 midnight. Effort is not adjustable.",
		},
		"ernie-light-dt-1.0": {
			Ratio: nativeRate(8), CompletionRatio: 3.5, CachedInputRatio: nativeRate(.8),
			ContextLength: 1_000_000, MaxOutputTokens: 128_000,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "ERNIE Light DT 1.0 on Qianfan v2, CNY 8 input, 0.8 cached input and 28 output per million tokens at all hours. Published 1M context and 128K total output, with K normalized to 1000. The compatible converter does not forward this model's upstream reasoning_effort controls.",
		},
	}
	for _, id := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-1017"} {
		models[id] = adaptor.ModelConfig{
			Ratio: nativeRate(1), CompletionRatio: 4, CachedInputRatio: nativeRate(.02),
			ContextLength: 1_000_000, MaxOutputTokens: 393_216,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			TimeWindows: []adaptor.TimeWindow{{
				Name: "qianfan-peak", TimeZone: "Asia/Shanghai",
				Ranges:  []adaptor.ClockRange{{Start: "08:00", End: "22:00"}},
				Overlay: adaptor.ModelConfig{Ratio: nativeRate(2), CompletionRatio: 4, CachedInputRatio: nativeRate(.04)},
			}},
			Description: "DeepSeek V4.1 Flash on Qianfan v2 using the published lowercase API ID. 1M context and 393216 total output tokens. CNY/M input/cache/output 2/0.04/8 at 08:00-22:00 Asia/Shanghai, otherwise 1/0.02/4. No inherited holiday promotion, direct-provider USD price or native tool policy applies.",
		}
	}
	return models
}
