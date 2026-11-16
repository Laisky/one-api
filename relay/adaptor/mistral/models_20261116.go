package mistral

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers November-reviewed hosted chat and offline audio models. It takes
// no parameters and returns no values. Complete definitions retain Mistral's own
// tariffs and use only the HTTP contracts already handled by this adaptor.
func init() {
	ModelRatios = adaptor.JoinModelCatalogs(ModelRatios, novemberChatModels(), voxtral3Models())
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}

// novemberChatModels returns independently owned hosted-model defaults verified
// on 2026-11-16. It takes no parameters. Sources:
// https://docs.mistral.ai/inference/pricing
// https://docs.mistral.ai/models/zai-glm-5-3-flash
// https://docs.mistral.ai/models/deepseek-v4-flash-1017
// https://docs.mistral.ai/inference/conversations/reasoning
func novemberChatModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"zai-glm-5-3-flash": {
			Ratio: 0.15 * ratio.MilliTokensUsd, CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.50 / 0.15,
			ContextLength: 1_000_000, MaxOutputTokens: 128_000,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedReasoningEfforts:   []string{"low", "high", "max"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice", "reasoning_effort"},
			Description:                 "Mistral-hosted GLM 5.3 Flash, generally available since 2026-10-22, with text/image input, 1M context and 128K maximum output. Mistral USD/M input/cache/output: 0.15/0.03/0.50. Native low/high/max reasoning; no unsupported none value or undocumented default is advertised.",
		},
		"deepseek-v4-1-flash-1017": {
			Ratio: 0.30 * ratio.MilliTokensUsd, CachedInputRatio: 0.007 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30,
			ContextLength: 1_000_000, MaxOutputTokens: 128_000,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice"},
			Description:                 "Mistral-hosted DeepSeek V4.1 Flash 1017 preview, released 2026-10-28, with text/image input, 1M context and 128K maximum output. Exact API ID includes v4-1 even though the model-card URL uses v4. Mistral USD/M input/cache/output: 0.30/0.007/1.20. No undocumented reasoning-effort vocabulary or checkpoint precision is inferred.",
		},
	}
}

// voxtral3Models returns independently owned documented offline audio IDs and
// aliases. It takes no parameters; realtime WebSocket models are not included.
// Sources verified 2026-11-16:
// https://docs.mistral.ai/models/voxtral-mini-transcribe-3-0
// https://docs.mistral.ai/models/voxtral-small-transcribe-3-0
// https://docs.mistral.ai/models/voxtral-mini-tts-3-0
// https://docs.mistral.ai/inference/pricing
func voxtral3Models() map[string]adaptor.ModelConfig {
	models := make(map[string]adaptor.ModelConfig)
	for _, family := range []struct {
		prefix string
		usd    float64
	}{
		{"voxtral-mini-transcribe", 0.0035},
		{"voxtral-small-transcribe", 0.004},
	} {
		for _, suffix := range []string{"-3-0", "-3", "-latest"} {
			models[family.prefix+suffix] = adaptor.ModelConfig{
				CompletionRatio: 1,
				ContextLength:   32_000,
				InputModalities: []string{"audio"}, OutputModalities: []string{"text"},
				Audio: &adaptor.AudioPricingConfig{
					InputUnit: "seconds", InputPriceUsd: family.usd, InputPriceQuantity: 60,
					UsdPerSecond: family.usd / 60,
				},
				Description: "Voxtral Transcribe 3 preview, released 2026-10-20, for the native /v1/audio/transcriptions endpoint. Billed by input audio duration, not text tokens. The documented context is 32k; no undocumented maximum output, cached-audio billing or realtime transport is implied.",
			}
		}
	}
	for _, suffix := range []string{"-3-0", "-3", "-latest"} {
		models["voxtral-mini-tts"+suffix] = adaptor.ModelConfig{
			Ratio: 13 * ratio.MilliTokensUsd, CompletionRatio: 1,
			InputModalities: []string{"text"}, OutputModalities: []string{"audio"},
			Audio:       &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceUsd: 13, InputPriceQuantity: 1e6},
			Description: "Voxtral Mini TTS 3 preview, released 2026-10-20, for the native /v1/audio/speech endpoint at $13 per million input characters. The latest alias now targets generation 3; the March 2026 pinned IDs retain their historical tariff. Context and output ceilings are not published.",
		}
	}
	return models
}
