package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// flashXLegalModels returns the BigModel legal model with its own CNY tariff.
// It accepts no arguments and returns independently owned model configurations.
// Sources checked 2026-11-16:
// https://docs.bigmodel.cn/cn/guide/models/vlm/glm-5.3-flash
// https://docs.bigmodel.cn/cn/guide/start/model-overview
// https://docs.bigmodel.cn/cn/guide/start/pricing
func flashXLegalModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"glm-5.3-flashx-legal": {
			Ratio: 2 * ratio.MilliTokensRmb, CompletionRatio: 3.5,
			CachedInputRatio: .57 * ratio.MilliTokensRmb,
			ContextLength:    1_000_000, MaxOutputTokens: 131_072,
			InputModalities: textImageVideoFileInput(), OutputModalities: textOutput(),
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "stop", "reasoning_effort"},
			SupportedReasoningEfforts:   []string{"low", "high", "max"},
			DefaultReasoningEffort:      "max",
			Description:                 "GLM-5.3-FlashX-Legal on BigModel: legal-specialized FlashX with 1M context, 128K output and always-on reasoning. CNY 2 input, 0.57 cached input and 7 output per million tokens. Public API availability is separate from AutoClaw or Coding Plan entitlements; no proprietary legal-database tool integration is implied.",
		},
	}
}
