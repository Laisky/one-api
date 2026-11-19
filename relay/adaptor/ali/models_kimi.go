package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// kimiModels returns independently owned kimi defaults for ali.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
// Additional model and cache sources checked 2026-11-16:
// https://help.aliyun.com/zh/model-studio/context-cache
// https://help.aliyun.com/zh/model-studio/kimi-k3-1-pro
// https://help.aliyun.com/zh/model-studio/qwen-api-via-dashscope
func kimiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"Moonshot-Kimi-K2-Instruct": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "Moonshot-Kimi-K2-Instruct on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2-thinking": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "kimi-k2-thinking on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 5.25,
			Description:     "kimi-k2.5 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.6": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi-k2.6 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.7-code": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi-k2.7-code on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k3": {
			Ratio:           nativeRate(20),
			CompletionRatio: 5,
			Description:     "kimi-k3 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			ContextLength: 1_048_576, MaxOutputTokens: 1_048_576,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "Alibaba-deployed Kimi K3.1 Pro, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Upstream supports text and image input; native Ali forwards text only and does not forward the thinking switch or effort. This ID is distinct from Moonshot-supplied kimi/kimi-k3.1-pro.",
		},
		"kimi/kimi-k2.5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 5.25,
			Description:     "kimi/kimi-k2.5 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.6": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi/kimi-k2.6 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.7-code": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi/kimi-k2.7-code on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.7-code-highspeed": {
			Ratio:           nativeRate(13),
			CompletionRatio: 54.0 / 13.0,
			Description:     "kimi/kimi-k2.7-code-highspeed on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k3": {
			Ratio:           nativeRate(20),
			CompletionRatio: 5,
			Description:     "kimi/kimi-k3 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro through Alibaba, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Native Ali forwards text only. Hosted control vocabulary and limits are not copied from Moonshot's direct API.",
		},
		"kimi/kimi-k3.1-pro-highspeed": {
			Ratio: nativeRate(26), CompletionRatio: 136.0 / 26, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro Highspeed through Alibaba, Beijing CNY 26 input, 1.3 cached input and 136 output per million tokens. The 5% cached-input tariff does not double with input/output. Native Ali forwards text only.",
		},
	}
}
