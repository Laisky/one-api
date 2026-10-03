package baiduv2

import "github.com/Laisky/one-api/relay/adaptor"

// otherModels returns the other model defaults for baiduv2.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func otherModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"GLM-5.3": {
			Ratio:            nativeRate(8),
			CompletionRatio:  3.5,
			CachedInputRatio: nativeRate(2),
			TimeWindows: []adaptor.TimeWindow{adaptor.TimeWindow{
				Name:     "qianfan-holiday-peak",
				TimeZone: "Asia/Shanghai",
				Ranges: []adaptor.ClockRange{adaptor.ClockRange{
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
				adaptor.TimeWindow{
					Name:     "qianfan-holiday-offpeak",
					TimeZone: "Asia/Shanghai",
					Ranges: []adaptor.ClockRange{adaptor.ClockRange{
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
				adaptor.TimeWindow{
					Name:     "qianfan-peak",
					TimeZone: "Asia/Shanghai",
					Ranges: []adaptor.ClockRange{adaptor.ClockRange{
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
	}
}
