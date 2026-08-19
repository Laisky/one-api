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
// BigModel's pricing page (https://open.bigmodel.cn/pricing) does not list the
// audio models yet; the rates below are documented placeholders until official
// pricing is published. The relay bills TTS by input bytes (len(input)) and
// ASR by estimated audio tokens (duration * PromptTokensPerSecond).
var audioModels = map[string]adaptor.ModelConfig{
	// GLM-TTS: placeholder ¥0.2 per 1K Chinese characters (~3 bytes/char).
	"glm-tts": {
		Ratio:            (0.2 / 3000) * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Audio: &adaptor.AudioPricingConfig{
			CompletionTokensPerSecond: 20,
		},
		Description: "GLM-TTS: hyper-realistic Chinese text-to-speech with emotion-aware prosody; wav/pcm output, streaming supported. Placeholder pricing pending BigModel's audio pricing page.",
	},
	// GLM-ASR-2512: placeholder ¥0.5 per audio minute.
	"glm-asr-2512": {
		Ratio:            (0.5 / 600) * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"text"},
		Audio: &adaptor.AudioPricingConfig{
			PromptTokensPerSecond: 10,
		},
		Description: "GLM-ASR-2512: high-accuracy multilingual speech recognition (CER 0.0717); wav/mp3 up to 25MB / 30s, streaming supported. Placeholder pricing pending BigModel's audio pricing page.",
	},
	// GLM-TTS-Clone: voice cloning from a 3s audio sample (file_id from the
	// platform file API). Billed per clone via PerCall; placeholder ¥2/clone
	// until BigModel publishes official pricing.
	"glm-tts-clone": {
		Ratio:            2 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"audio", "text"},
		OutputModalities: []string{"audio"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.0 / 7 * 1000},
		Description:      "GLM-TTS-Clone: voice cloning from a 3-second sample via /api/paas/v4/voice/clone; returns a reusable voice id and preview audio. Placeholder ¥2/clone pending BigModel's pricing page.",
	},
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
