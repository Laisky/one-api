package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// kimiModels returns independently owned kimi defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
// Additional model and cache sources checked 2026-11-22:
// https://help.aliyun.com/zh/model-studio/newly-released-models
// https://help.aliyun.com/zh/model-studio/context-cache
// https://help.aliyun.com/zh/model-studio/kimi-k3-1-pro
// https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions
func kimiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"Moonshot-Kimi-K2-Instruct": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "Moonshot-Kimi-K2-Instruct on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2-thinking": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "kimi-k2-thinking on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 5.25,
			Description:     "kimi-k2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.6": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi-k2.6 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.7-code": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi-k2.7-code on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k3": {
			Ratio:           nativeRate(20),
			CompletionRatio: 5,
			Description:     "kimi-k3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			ContextLength: 1_048_576, MaxOutputTokens: 1_048_576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Alibaba-deployed Kimi K3.1 Pro, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Supports text and image, not video. Distinct from Moonshot-supplied kimi/kimi-k3.1-pro; the gateway does not forward this model's upstream effort controls.",
		},
		"kimi-k3.1-pro-prime": {
			Ratio: nativeRate(26), CompletionRatio: 136.0 / 26, CachedInputRatio: nativeRate(1.3),
			ContextLength: 1_048_576, MaxOutputTokens: 1_048_576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Alibaba-deployed Kimi K3.1 Pro Prime, released 2026-11-19 with the same capabilities and limits as Pro. Beijing CNY 26 input, 1.3 cached input and 136 output per million tokens; cached input does not double. Supports text and image, not video. Distinct from Moonshot-supplied kimi/kimi-k3.1-pro-highspeed; the gateway does not forward this model's upstream effort controls.",
		},
		"kimi/kimi-k2.5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 5.25,
			Description:     "kimi/kimi-k2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.6": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi/kimi-k2.6 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.7-code": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 27.0 / 6.5,
			Description:     "kimi/kimi-k2.7-code on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.7-code-highspeed": {
			Ratio:           nativeRate(13),
			CompletionRatio: 54.0 / 13.0,
			Description:     "kimi/kimi-k2.7-code-highspeed on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k3": {
			Ratio:                       nativeRate(20),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(2),
			ContextLength:               1048576,
			MaxOutputTokens:             1048576,
			InputModalities:             []string{"text", "image", "video"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"max_tokens", "stop"},
			SupportedReasoningEfforts:   []string{"max"},
			DefaultReasoningEffort:      "max",
			Description:                 "kimi/kimi-k3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro through Alibaba, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Hosted control vocabulary and limits are not copied from Moonshot's direct API; adjustable effort is not forwarded by the current gateway.",
		},
		"kimi/kimi-k3.1-pro-highspeed": {
			Ratio: nativeRate(26), CompletionRatio: 136.0 / 26, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro Highspeed through Alibaba, Beijing CNY 26 input, 1.3 cached input and 136 output per million tokens. The 5% cached-input tariff does not double with input/output. Adjustable effort is not forwarded by the current gateway.",
		},
	}
}
