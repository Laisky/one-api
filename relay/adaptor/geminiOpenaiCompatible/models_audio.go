package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// audioModelRatios contains complete audio catalog entries.
var audioModelRatios = map[string]adaptor.ModelConfig{
	"gemini-3.1-flash-live-preview": {
		Ratio:                       0.75 * ratio.MilliTokensUsd,
		CompletionRatio:             4.50 / 0.75,
		Audio:                       gemini31FlashLivePreviewPricing.Audio,
		ContextLength:               32_768,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputLive...),
		OutputModalities:            append([]string(nil), geminiOutputTextAudio...),
		SupportedFeatures:           append([]string(nil), []string{"tools", "json_mode"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		Description:                 "Gemini 3.1 Flash Live preview; published earliest shutdown November 17, 2026. Use gemini-3.8-live with its dedicated native Live WebSocket API.",
	},
	"gemini-3.5-live-translate-preview": {
		Ratio:           3.50 * ratio.MilliTokensUsd,
		CompletionRatio: 21.0 / 3.50,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
		ContextLength:               131_072,
		MaxOutputTokens:             65_536,
		InputModalities:             append([]string(nil), []string{"audio"}...),
		OutputModalities:            append([]string(nil), geminiOutputTextAudio...),
		SupportedFeatures:           append([]string(nil), []string{"tools"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		Description:                 "Gemini 3.5 Live Translate preview: low-latency audio-to-audio real-time speech translation tier (131K ctx, 65K out).",
	},
	"gemini-3.1-flash-tts-preview": {
		Ratio:           1.0 * ratio.MilliTokensUsd,
		CompletionRatio: 20.0 / 1.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputTextOnly...),
		OutputModalities:            append([]string(nil), geminiOutputAudio...),
		SupportedSamplingParameters: append([]string(nil), []string{"temperature", "top_p", "top_k"}...),
		Description:                 "Gemini 3.1 Flash TTS preview; published earliest shutdown November 17, 2026. Use gemini-3.8-flash-tts or gemini-3.8-flash-lite-tts; migration requires the new speech_metadata protocol.",
	},
	"gemini-2.5-flash-native-audio": {
		Ratio:                       0.50 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0 / 0.50,
		Audio:                       gemini25FlashNativeAudioPricing.Audio,
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputLive...),
		OutputModalities:            append([]string(nil), geminiOutputTextAudio...),
		SupportedFeatures:           append([]string(nil), []string{"tools", "json_mode", "reasoning"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini25ReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini25FlashMaxThinkingBudget,
		Description:                 "Gemini 2.5 Flash native-audio dialog tier with low-latency audio in/out.",
	},
	"gemini-2.5-flash-native-audio-preview-09-2025": {
		Ratio:                       0.50 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0 / 0.50,
		Audio:                       gemini25FlashNativeAudioPricing.Audio,
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputLive...),
		OutputModalities:            append([]string(nil), geminiOutputTextAudio...),
		SupportedFeatures:           append([]string(nil), []string{"tools", "json_mode", "reasoning"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini25ReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini25FlashMaxThinkingBudget,
		Description:                 "Gemini 2.5 Flash native-audio preview snapshot dated 09-2025.",
	},
	"gemini-2.5-flash-native-audio-preview-12-2025": {
		Ratio:                       0.50 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0 / 0.50,
		Audio:                       gemini25FlashNativeAudioPricing.Audio,
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputLive...),
		OutputModalities:            append([]string(nil), geminiOutputTextAudio...),
		SupportedFeatures:           append([]string(nil), []string{"tools", "json_mode", "reasoning"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini25ReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini25FlashMaxThinkingBudget,
		Description:                 "Gemini 2.5 Flash native-audio preview December 2025 snapshot; published earliest shutdown November 17, 2026. Use gemini-3.8-live and its native Live WebSocket transport.",
	},
	"gemini-2.5-flash-preview-tts": {
		Ratio:           0.50 * ratio.MilliTokensUsd,
		CompletionRatio: 10.0 / 0.50,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputTextOnly...),
		OutputModalities:            append([]string(nil), geminiOutputAudio...),
		SupportedSamplingParameters: append([]string(nil), []string{"temperature", "top_p", "top_k"}...),
		Description:                 "Gemini 2.5 Flash TTS preview; published earliest shutdown November 17, 2026. Use gemini-3.8-flash-lite-tts; migration requires the new speech_metadata protocol.",
	},
	"gemini-2.5-pro-preview-tts": {
		Ratio:           1.0 * ratio.MilliTokensUsd,
		CompletionRatio: 20.0 / 1.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             append([]string(nil), geminiInputTextOnly...),
		OutputModalities:            append([]string(nil), geminiOutputAudio...),
		SupportedSamplingParameters: append([]string(nil), []string{"temperature", "top_p", "top_k"}...),
		Description:                 "Gemini 2.5 Pro TTS preview; published earliest shutdown November 17, 2026. Use gemini-3.8-flash-tts; migration requires the new speech_metadata protocol.",
	},
	"gemini-3.5-transcribe": geminiTranscribeConfig(
		2.00,
		12.00,
		"Gemini 3.5 Transcribe stable audio-to-text model with language detection, diarization, word timestamps, smart transcription, and vocabulary biasing.",
	),
	"gemini-3.5-transcribe-live": geminiTranscribeConfig(
		3.50,
		21.00,
		"Gemini 3.5 Transcribe Live stable low-latency streaming audio-to-text model over the Live API.",
	),
	"gemini-3.8-live":                   gemini38LiveConfig(false),
	"gemini-3.8-live-extended-thinking": gemini38LiveConfig(true),
	"gemini-3.8-flash-tts":              gemini38TTSConfig(9.00, "Gemini 3.8 Flash TTS for expressive speech and long-form multi-speaker narration."),
	"gemini-3.8-flash-lite-tts":         gemini38TTSConfig(6.00, "Gemini 3.8 Flash-Lite TTS for high-throughput, low-latency speech generation."),
}

// geminiTranscribeConfig builds the pricing and modality metadata for
// a Gemini 3.5 transcription endpoint. Parameters: inputUsd and outputUsd are paid-tier
// prices per million tokens, and description summarizes the endpoint. Returns: a model configuration.
func geminiTranscribeConfig(inputUsd float64, outputUsd float64, description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:           inputUsd * ratio.MilliTokensUsd,
		CompletionRatio: outputUsd / inputUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           1,
			PromptTokensPerSecond: 25,
		},
		InputModalities:  []string{"audio"},
		OutputModalities: geminiOutputText,
		Description:      description,
	}
}

// gemini38LiveConfig builds the documented text/audio prices and Live capabilities.
// Parameters: extendedThinking selects the configurable background-reasoning variant.
// Returns: an independently allocated configuration without Flash pricing overlays.
func gemini38LiveConfig(extendedThinking bool) adaptor.ModelConfig {
	config := adaptor.ModelConfig{
		Ratio:           0.75 * ratio.MilliTokensUsd,
		CompletionRatio: 4.50 / 0.75,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:               3.00 / 0.75,
			CompletionRatio:           12.00 / 3.00,
			PromptTokensPerSecond:     25,
			CompletionTokensPerSecond: 25,
		},
		ContextLength:     131_072,
		MaxOutputTokens:   65_536,
		InputModalities:   []string{"text", "image", "audio", "video"},
		OutputModalities:  []string{"text", "audio"},
		SupportedFeatures: []string{"tools", "web_search", "reasoning"},
		Description: "Gemini 3.8 Live stable voice model with automatic interleaved thinking; " +
			"thinking configuration is not supported. Requires the native Live WebSocket API, " +
			"not implemented by this REST adaptor. Text output uses audio transcription.",
	}
	if extendedThinking {
		config.SupportedReasoningEfforts = []string{"low", "medium", "high"}
		config.Description = "Gemini 3.8 Live Extended Thinking stable voice model with background reasoning " +
			"and non-blocking function calls. Requires the native Live WebSocket API, " +
			"not implemented by this REST adaptor. Text output uses audio transcription."
	}
	// Do not infer a default thinking level, integer thinking budget, caching
	// discount, or the Flash family's 2027 price transition for either Live model.
	return config
}

// gemini38TTSConfig builds a Gemini 3.8 text-to-speech catalog entry.
// Parameters: outputUsd is the promotional audio price and description names the tier.
// Returns: isolated text-input/audio-output metadata without chat or Live capabilities.
func gemini38TTSConfig(outputUsd float64, description string) adaptor.ModelConfig {
	const inputUsd = 0.50
	return adaptor.ModelConfig{
		Ratio:            inputUsd * ratio.MilliTokensUsd,
		CompletionRatio:  outputUsd / inputUsd,
		CachedInputRatio: 0.125 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			// Receipt-based audio completion pricing is relative to audio input.
			// A unit prompt multiplier anchors that calculation to text input for
			// this text-only input model; it does not advertise audio input.
			PromptRatio:               1,
			CompletionRatio:           outputUsd / inputUsd,
			CompletionTokensPerSecond: 25,
		},
		TimeWindows: []adaptor.TimeWindow{
			// Input, audio output, and cached-input rates all double. The audio
			// multipliers remain unchanged and are preserved by the overlay.
			geminiStandardPriceWindow(1.00, outputUsd*2, 0.25),
		},
		ContextLength:    8_192,
		MaxOutputTokens:  16_384,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description: description + " Generally available in the Gemini Developer API since September 22, 2026. " +
			"Uses speech_metadata for speaker/style and defaults to WAV for unary output. " +
			"Available through /v1/audio/speech with native Google transport; Vertex requires explicit channel token prices. Not a Live or chat-completion protocol.",
	}
}
