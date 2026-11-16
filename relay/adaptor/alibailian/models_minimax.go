package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// minimaxModels returns independently owned minimax defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
// Additional model and cache sources checked 2026-11-16:
// https://help.aliyun.com/zh/model-studio/context-cache
// https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions
func minimaxModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"MiniMax-M2.1": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax-M2.1 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax-M2.5": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax-M2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M2.1": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax/MiniMax-M2.1 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M2.5": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax/MiniMax-M2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M2.7": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax/MiniMax-M2.7 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M3": {
			Ratio: nativeRate(2.1), CompletionRatio: 4, CachedInputRatio: nativeRate(.42),
			Tiers: []adaptor.ModelRatioTier{{
				InputTokenThreshold: 512001,
				Ratio:               nativeRate(4.2), CompletionRatio: 4, CachedInputRatio: nativeRate(.84),
			}},
			// Alibaba's own tariff and cache policy, checked 2026-11-16:
			// https://help.aliyun.com/zh/model-studio/model-pricing
			// https://help.aliyun.com/zh/model-studio/context-cache
			Description: "MiniMax M3 supplied through Alibaba, Beijing CNY/M input/cache/output 2.1/0.42/8.4 up to 512K input, then 4.2/0.84/16.8. Native MiniMax Priority pricing is not an Alibaba tariff.",
		},
		"MiniMax/MiniMax-M3.1-Flash": {
			Ratio: nativeRate(1), CompletionRatio: 4, CachedInputRatio: nativeRate(.1),
			ContextLength:   1_000_000,
			InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "MiniMax M3.1 Flash supplied through Alibaba Model Studio. Beijing CNY 1 input, 0.1 cached input and 4 output per million tokens. Sampling is fixed upstream; adjustable effort is not advertised because the gateway's compatible converter does not forward it for this ID. Hosted output limits are not copied from MiniMax's direct API.",
		},
	}
}
