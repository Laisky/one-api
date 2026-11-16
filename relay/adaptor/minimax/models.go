package minimax

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Legacy metadata is retained for existing configurations, not as a claim that
// every historical endpoint remains available. Current M-series definitions are
// separate from these legacy defaults and use the documented pay-as-you-go API.
var (
	minimaxTextInputs        = []string{"text"}
	minimaxVisionInputs      = []string{"text", "image"}
	minimaxTextOutputs       = []string{"text"}
	minimaxChatFeatures      = []string{"tools", "json_mode"}
	minimaxReasoningFeatures = []string{"tools", "json_mode", "reasoning"}
	minimaxSamplingParams    = []string{"temperature", "top_p", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"}
)

// ModelRatios contains one complete definition per MiniMax API model ID.
// Current model and tariff sources were checked on 2026-11-16:
// https://platform.minimax.cn/docs/guides/models-intro
// https://platform.minimax.cn/docs/pricing/overview
// https://platform.minimax.cn/docs/api-reference/text-openai-api
// https://platform.minimax.cn/docs/api-reference/text-chat-openai
// Native MiniMax transport is not implemented by this catalogue-only adaptor.
var ModelRatios = adaptor.JoinModelCatalogs(minimaxMSeriesModels(), minimaxLegacyModels())

// ModelList retains the compatibility list derived from the complete catalogue.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MinimaxToolingDefaults remains empty because native server-tool transport is
// not implemented here. MiniMax publishes a separate CNY 0.03 web_search fee;
// omitting its default is not a claim that server-side search is free.
var MinimaxToolingDefaults = adaptor.ChannelToolConfig{}

const minimaxM2RetirementNotice = " Pay-as-you-go access closes to new users on 2026-10-19 and retires on 2026-11-18. The 2026-10-19 M3.1 Flash Preview rerouting applies only to M Plan/Token Plan subscriptions, not ordinary API keys."

// minimaxMSeriesModels returns owned metadata for the documented pay-as-you-go
// M-series models. It takes no parameters. Amounts are CNY per million tokens;
// Standard and Priority are separate tariffs, not different model identifiers.
func minimaxMSeriesModels() map[string]adaptor.ModelConfig {
	models := map[string]adaptor.ModelConfig{
		"MiniMax-M3": {
			Ratio: 2.10 * ratio.MilliTokensRmb, CompletionRatio: 8.40 / 2.10,
			CachedInputRatio: 0.42 * ratio.MilliTokensRmb,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio: 4.20 * ratio.MilliTokensRmb, CompletionRatio: 16.80 / 4.20,
				CachedInputRatio: 0.84 * ratio.MilliTokensRmb, InputTokenThreshold: 512001,
			}},
			ContextLength: 1_000_000, MaxOutputTokens: 524288,
			InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			HuggingFaceID:               "MiniMaxAI/MiniMax-M3",
			Description:                 "MiniMax M3 multimodal pay-as-you-go model with 1M context. Thinking defaults on and can be disabled; reasoning_effort does not tune this model. Standard CNY/M input/cache/output is 2.10/0.42/8.40, doubled above 512K input. The published 50% discount is permanent. Priority requests cost 1.5 times Standard and require separate effective pricing; these defaults are Standard only. Native transport is not implemented by this catalogue adaptor.",
		},
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
			SupportedSamplingParameters: minimaxMSeriesSamplingParameters(),
			Description:                 description,
		}
	}
	return models
}

// minimaxMSeriesSamplingParameters returns the documented effective sampling
// and output-length controls. It takes no parameters and returns a fresh slice.
// Ignored penalties and undocumented seed/stop controls are not advertised.
func minimaxMSeriesSamplingParameters() []string {
	return []string{"temperature", "top_p", "max_tokens", "max_completion_tokens"}
}

// minimaxLegacyModels returns independently constructed legacy catalogue records.
// It takes no parameters. Historical prices are not claims of current availability.
func minimaxLegacyModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"M2-her": {
			Ratio: 2.1 * ratio.MilliTokensRmb, CompletionRatio: 8.4 / 2.1,
			ContextLength: 65536, MaxOutputTokens: 2048,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax M2-her dialogue / role-play specialized variant.",
		},
		"abab6.5-chat": {
			Ratio: 30 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 245760, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6.5 long-context closed-weight chat model (legacy).",
		},
		"abab6.5s-chat": {
			Ratio: 10 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 245760, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6.5s cost-optimized variant of abab6.5 (legacy).",
		},
		"abab6.5t-chat": {
			Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 8192, MaxOutputTokens: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6.5t tiny cost-optimized chat model (legacy).",
		},
		"abab6-chat": {
			Ratio: 100 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 32768, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6 closed-weight chat model with 32k context (legacy).",
		},
		"abab5.5-chat": {
			Ratio: 15 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 16384, MaxOutputTokens: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab5.5 closed-weight chat model (legacy).",
		},
		"abab5.5s-chat": {
			Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 8192, MaxOutputTokens: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab5.5s cost-optimized chat model (legacy).",
		},
		"MiniMax-VL-01": {
			Ratio: 20 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 1000000, MaxOutputTokens: 8192,
			InputModalities: minimaxVisionInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax-VL-01 multimodal chat model accepting text and image inputs (legacy estimated pricing).",
		},
		"MiniMax-Text-01": {
			Ratio: 15 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 1000000, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Quantization: "bf16", HuggingFaceID: "MiniMaxAI/MiniMax-Text-01",
			Description: "MiniMax-Text-01 long-context chat model with open weights on HuggingFace (legacy estimated pricing).",
		},
		"MiniMax-M1": {
			Ratio: 2.8 * ratio.MilliTokensRmb, CompletionRatio: 16.8 / 2.8,
			ContextLength: 1024000, MaxOutputTokens: 40960,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxReasoningFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Quantization: "bf16", HuggingFaceID: "MiniMaxAI/MiniMax-M1-80k",
			Description: "MiniMax-M1 reasoning MoE model with 1M context (open weights, legacy estimated pricing).",
		},
		"embo-01": {
			Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1, ContextLength: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			Description: "MiniMax embo-01 text embedding model (legacy estimated pricing).",
		},
	}
}
