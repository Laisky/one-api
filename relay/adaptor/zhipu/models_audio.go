package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// audioModels enumerates Zhipu's text-to-speech and speech-to-text models.
// GLM exposes OpenAI-compatible endpoints (/api/paas/v4/audio/speech and
// /api/paas/v4/audio/transcriptions), which the relay's audio helper forwards
// to directly. Sources:
//   - https://docs.bigmodel.cn/api-reference/模型-api/文本转语音
//   - https://docs.bigmodel.cn/api-reference/模型-api/语音转文本
//
// Published CNY prices were checked 2026-09-24:
// https://docs.bigmodel.cn/cn/guide/start/pricing
var audioModels = map[string]adaptor.ModelConfig{
	// CNY 2 per 10,000 Unicode source characters, with no output charge.
	"glm-tts": {
		Ratio: 200 * ratio.MilliTokensRmb, CompletionRatio: 0,
		InputModalities: []string{"text"}, OutputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			InputUnit: "characters", InputPriceUsd: 2.0 / float64(ratio.ExchangeRateRmb), InputPriceQuantity: 10000,
		},
		Description: "GLM-TTS: CNY 2 per 10,000 source characters; generated audio is not separately charged. Uses the published character billing unit.",
	},
	// CNY 16 per million audio input tokens; the duration estimate is unchanged.
	"glm-asr-2512": {
		Ratio: 16 * ratio.MilliTokensRmb, CompletionRatio: 0,
		InputModalities: []string{"audio"}, OutputModalities: []string{"text"},
		Audio:       &adaptor.AudioPricingConfig{PromptTokensPerSecond: 10},
		Description: "GLM-ASR-2512: CNY 16 per million input audio tokens; text output is free. Duration-based token estimation remains the existing relay fallback, not an exact provider per-second tariff.",
	},
	"glm-tts-clone": voiceCloneModelConfig(),
}

// realtimeModels enumerates Zhipu's realtime audio/video conversation models.
// Source: https://docs.bigmodel.cn/cn/guide/models/sound-and-video/glm-realtime
//
// Published pricing (per minute):
//   - GLM-Realtime-Flash: audio ¥0.18/min, video ¥1.2/min
//   - GLM-Realtime-Air:   audio ¥0.3/min,  video ¥2.1/min
//
// The relay pre-consumes quota using a 120s session estimate at the model's
// audio token rates (10 in + 20 out tokens/s). Ratio encodes the audio price
// per token so the estimate matches the published per-minute audio rate:
// per-token = per-minute price / (60s * 30 tokens/s): flash ¥0.18/1800 and
// air ¥0.3/1800.
var realtimeModels = map[string]adaptor.ModelConfig{
	"glm-realtime-flash": {
		Ratio:            (0.18 / 1800) * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "audio", "video"},
		OutputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
		Description: "GLM-Realtime-Flash: real-time audio/video conversation model; 2-minute call memory, tool calling; audio ¥0.18/min, video ¥1.2/min.",
	},
	"glm-realtime-air": {
		Ratio:            (0.3 / 1800) * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "audio", "video"},
		OutputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
		Description: "GLM-Realtime-Air: higher-quality real-time audio/video conversation model; audio ¥0.3/min, video ¥2.1/min.",
	},
}

// voiceCloneModelConfig returns the independently owned voice-cloning tariff.
// It takes no arguments and preserves the existing two-stage float conversion.
func voiceCloneModelConfig() adaptor.ModelConfig {
	usdPerClone := 6.0 / float64(ratio.ExchangeRateRmb)
	return adaptor.ModelConfig{
		Ratio: usdPerClone * ratio.QuotaPerUsd, CompletionRatio: 1,
		InputModalities: []string{"audio", "text"}, OutputModalities: []string{"audio"},
		PerCall:     &adaptor.PerCallPricingConfig{UsdPerThousandCalls: usdPerClone * 1000},
		Description: "GLM-TTS-Clone: voice cloning via /api/paas/v4/voice/clone, CNY 6 per clone. Converted without truncating the RMB-to-quota factor.",
	}
}
