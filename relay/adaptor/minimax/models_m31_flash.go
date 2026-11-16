package minimax

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// minimaxM31FlashModels returns independently owned metadata for the published
// pay-as-you-go Flash model. It accepts no parameters and returns a new map.
// Sources checked 2026-11-16:
// https://platform.minimax.cn/docs/pricing/overview
// https://platform.minimax.cn/docs/api-reference/text-chat-openai
// https://platform.minimax.cn/docs/api-reference/text-openai-api
func minimaxM31FlashModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"MiniMax-M3.1-Flash": {
			Ratio: 1 * ratio.MilliTokensRmb, CompletionRatio: 4,
			CachedInputRatio: .1 * ratio.MilliTokensRmb,
			ContextLength:    1_000_000, MaxOutputTokens: 524_288,
			InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
			DefaultReasoningEffort:      "max",
			Description:                 "MiniMax M3.1 Flash: published pay-as-you-go model with 1M context and up to 524288 output tokens (131072 recommended). Always-on adaptive thinking; effort defaults to max. Temperature/top_p are fixed at 1/0.95 and service_tier is unsupported. CNY 1 input, 0.1 cached input and 4 output per million tokens. Native transport is not implemented by this catalogue-only adaptor.",
		},
	}
}
