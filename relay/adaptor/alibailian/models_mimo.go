package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// mimoModels returns independently owned mimo defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
// Additional model and cache sources checked 2026-11-16:
// https://help.aliyun.com/zh/model-studio/context-cache
// https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions
func mimoModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"xiaomi/mimo-v2.5-pro": {
			Ratio:           nativeRate(7),
			CompletionRatio: 3,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(14),
				CompletionRatio:     3,
				InputTokenThreshold: 256001,
			},
			},
			Description: "xiaomi/mimo-v2.5-pro on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"xiaomi/mimo-v2.6-pro-ultraspeed": {
			Ratio: nativeRate(30), CompletionRatio: 2,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			Description: "Xiaomi-supplied MiMo V2.6 Pro Ultraspeed through Alibaba, Beijing CNY 30 input and 60 output per million tokens up to 1M input. Cache price and generation limits remain unspecified; they are not inferred from Xiaomi's direct API.",
		},
	}
}
