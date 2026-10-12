package minimax

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

const minimaxM2RetirementNotice = " Pay-as-you-go access closes to new users on 2026-10-19 and retires on 2026-11-18. The 2026-10-19 M3.1 Flash Preview rerouting applies only to M Plan/Token Plan subscriptions, not ordinary API keys."

// minimaxCurrentModels returns owned metadata for the documented pay-as-you-go
// M-series models. It takes no parameters. Amounts are CNY per million tokens;
// Standard and Priority are separate tariffs, not different model identifiers.
func minimaxCurrentModels() map[string]adaptor.ModelConfig {
	models := map[string]adaptor.ModelConfig{
		"MiniMax-M3": {
			Ratio: 2.10 * ratio.MilliTokensRmb, CompletionRatio: 8.40 / 2.10,
			CachedInputRatio: 0.42 * ratio.MilliTokensRmb,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio: 4.20 * ratio.MilliTokensRmb, CompletionRatio: 16.80 / 4.20,
				CachedInputRatio: 0.84 * ratio.MilliTokensRmb, InputTokenThreshold: 512001,
			}},
			ContextLength: 1_000_000, MaxOutputTokens: 131072,
			InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: minimaxCurrentSamplingParameters(),
			HuggingFaceID:               "MiniMaxAI/MiniMax-M3",
			Description: "MiniMax M3 multimodal pay-as-you-go model with 1M context. Thinking defaults on and can be disabled; reasoning_effort does not tune this model. Standard CNY/M input/cache/output is 2.10/0.42/8.40, doubled above 512K input. The published 50% discount is permanent. Priority requests cost 1.5 times Standard and require separate effective pricing; these defaults are Standard only. Native transport is not implemented by this catalogue adaptor.",
		},
	}
	for _, spec := range []struct {
		id, description       string
		input, cached, output float64
	}{
		{"MiniMax-M2.7", "MiniMax M2.7 reasoning model.", 2.1, 0.42, 8.4},
		{"MiniMax-M2.7-highspeed", "MiniMax M2.7 high-speed tier.", 4.2, 0.42, 16.8},
		{"MiniMax-M2.5", "MiniMax M2.5 reasoning model.", 2.1, 0.21, 8.4},
		{"MiniMax-M2.5-highspeed", "MiniMax M2.5 high-speed tier.", 4.2, 0.21, 16.8},
		{"MiniMax-M2.1", "MiniMax M2.1 legacy reasoning model.", 2.1, 0.21, 8.4},
		{"MiniMax-M2.1-highspeed", "MiniMax M2.1 legacy high-speed tier.", 4.2, 0.21, 16.8},
		{"MiniMax-M2", "MiniMax M2 legacy reasoning model.", 2.1, 0.21, 8.4},
	} {
		description := spec.description + " Thinking cannot be disabled; a disabled switch is accepted but ignored upstream. No tunable reasoning_effort is advertised. Native transport is not implemented by this catalogue adaptor."
		if spec.id == "MiniMax-M2.7" || spec.id == "MiniMax-M2.7-highspeed" || spec.id == "MiniMax-M2.5" || spec.id == "MiniMax-M2.5-highspeed" {
			description += minimaxM2RetirementNotice
		}
		models[spec.id] = adaptor.ModelConfig{
			Ratio: spec.input * ratio.MilliTokensRmb, CompletionRatio: spec.output / spec.input,
			CachedInputRatio: spec.cached * ratio.MilliTokensRmb, CacheWrite5mRatio: 2.625 * ratio.MilliTokensRmb,
			ContextLength: 204800, MaxOutputTokens: 131072,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: minimaxCurrentSamplingParameters(),
			Description:                 description,
		}
	}
	return models
}

// minimaxCurrentSamplingParameters returns the documented effective sampling
// and output-length controls. It takes no parameters and returns a fresh slice.
// Ignored penalties and undocumented seed/stop controls are not advertised.
func minimaxCurrentSamplingParameters() []string {
	return []string{"temperature", "top_p", "max_tokens", "max_completion_tokens"}
}
