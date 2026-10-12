package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// octoberChatModels returns independently owned public DeepInfra chat defaults.
// It takes no parameters and returns ordinary Go definitions verified 2026-10-12.
// Source: https://api.deepinfra.com/models/list
func octoberChatModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"anthropic/claude-sonnet-5-5": {
			Ratio:             nativeRate(2),
			CompletionRatio:   (10 * 1.0) / 2,
			ContextLength:     1000000,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			MaxOutputTokens:   128000,
			Description:       "anthropic/claude-sonnet-5-5 on DeepInfra; public host catalogue verified 2026-10-12. Prices and limits are host-specific, not copied from another provider. Unpublished output limits, cache prices, sampling controls and reasoning-effort levels remain unspecified. This is a public partner-hosted endpoint; free anonymous access is not advertised. No native Anthropic cache tariff is inferred.",
		},
		"tencent/Hy4-preview": {
			Ratio:             nativeRate(0.834),
			CompletionRatio:   (2.501 * 1.0) / 0.834,
			CachedInputRatio:  nativeRate(0.04199999814),
			ContextLength:     1048576,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp8",
			Description:       "tencent/Hy4-preview on DeepInfra; public host catalogue verified 2026-10-12. Prices and limits are host-specific, not copied from another provider. Unpublished output limits, cache prices, sampling controls and reasoning-effort levels remain unspecified.",
		},
		"XiaomiMiMo/MiMo-V2.6-Pro": {
			Ratio:             nativeRate(0.43),
			CompletionRatio:   (0.87 * 1.0) / 0.43,
			CachedInputRatio:  nativeRate(0.0035999987),
			ContextLength:     1048576,
			InputModalities:   []string{"text", "image", "video", "audio"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp8",
			Description:       "XiaomiMiMo/MiMo-V2.6-Pro on DeepInfra; public host catalogue verified 2026-10-12. Prices and limits are host-specific, not copied from another provider. Unpublished output limits, cache prices, sampling controls and reasoning-effort levels remain unspecified.",
		},
		"XiaomiMiMo/MiMo-V2.6-Flash": {
			Ratio:             nativeRate(0.14),
			CompletionRatio:   (0.28 * 1.0) / 0.14,
			CachedInputRatio:  nativeRate(0.0028),
			ContextLength:     1048576,
			InputModalities:   []string{"text", "image", "video", "audio"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp8",
			Description:       "XiaomiMiMo/MiMo-V2.6-Flash on DeepInfra; public host catalogue verified 2026-10-12. Prices and limits are host-specific, not copied from another provider. Unpublished output limits, cache prices, sampling controls and reasoning-effort levels remain unspecified.",
		},
	}
}
