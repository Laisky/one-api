package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// audioModelRatios captures pricing and metadata for OpenAI audio (TTS / STT)
// models. These models route through dedicated /audio endpoints; modalities
// reflect the actual request envelope (TTS = text in / audio out, STT = audio
// in / text out) so capability surfaces can advertise them accurately.
// Source: https://platform.openai.com/docs/models/whisper-1, /tts-1, /gpt-4o-transcribe.
var audioModelRatios = map[string]adaptor.ModelConfig{
	"whisper-1": {
		Ratio:           6.0 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0, // $0.006 per minute
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16,
			CompletionRatio:       0,
			PromptTokensPerSecond: 10,
			UsdPerSecond:          0.0001,
		},
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"text"},
		Description:      "Whisper v1: speech-to-text transcription model.",
	},
	"tts-1": {
		Ratio:            15.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0, // $15.00 per 1M characters
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description:      "TTS-1: standard-quality text-to-speech model.",
	},
	"tts-1-1106": {
		Ratio:            15.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description:      "TTS-1 snapshot from 2023-11-06.",
	},
	"tts-1-hd": {
		Ratio:            30.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0, // $30.00 per 1M characters
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description:      "TTS-1 HD: higher-fidelity text-to-speech model.",
	},
	"tts-1-hd-1106": {
		Ratio:            30.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description:      "TTS-1 HD snapshot from 2023-11-06.",
	},
	"gpt-4o-transcribe": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           6.0 / 2.5,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:    16000,
		MaxOutputTokens:  2000,
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"text"},
		Description:      "GPT-4o transcribe: high-quality speech-to-text successor to Whisper.",
	},
	"gpt-4o-mini-transcribe": {
		Ratio:           1.25 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           3.0 / 1.25,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:    16000,
		MaxOutputTokens:  2000,
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"text"},
		Description:      "GPT-4o mini transcribe: cost-efficient speech-to-text model.",
	},
	"gpt-4o-mini-tts": {
		Ratio:            0.6 * ratio.MilliTokensUsd,
		CompletionRatio:  20.0, // $0.60 input, $12.00 output per 1M tokens
		ContextLength:    2000,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description:      "GPT-4o mini TTS: low-latency neural text-to-speech model.",
	},
	// gpt-4o-mini-tts-2025-12-15: latest dated snapshot per OpenAI deprecation page.
	// Source: https://developers.openai.com/api/docs/models/gpt-4o-mini-tts
	"gpt-4o-mini-tts-2025-12-15": {
		Ratio:            0.6 * ratio.MilliTokensUsd,
		CompletionRatio:  20.0,
		ContextLength:    2000,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description:      "GPT-4o mini TTS snapshot from 2025-12-15.",
	},
	// gpt-audio-1.5: first generally available chat-completions audio model.
	// Replaces gpt-4o-audio-preview family. Released 2025-08-28 as gpt-audio,
	// upgraded to 1.5 generation late 2025. Text $2.50/$10, audio $32/$64 per 1M tokens.
	// Source: https://developers.openai.com/api/docs/models/gpt-audio-1.5
	"gpt-audio-1.5": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 10.0 / 2.5,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           32.0 / 2.5, // $32/$2.5 = 12.8x
			CompletionRatio:       2,          // $64/$32 = 2x
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Audio 1.5: GA chat-completions audio model (text+audio in/out).",
	},
	// gpt-audio: aliased to gpt-audio-2025-08-28 (the original GA release).
	// Source: https://developers.openai.com/api/docs/models/gpt-audio
	"gpt-audio": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 10.0 / 2.5,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           32.0 / 2.5,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Audio: first GA audio chat-completions model.",
	},
	"gpt-audio-2025-08-28": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 10.0 / 2.5,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           32.0 / 2.5,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Audio snapshot from 2025-08-28 (initial GA release).",
	},
	// gpt-audio-mini: cost-efficient audio chat model. Text $0.60/$2.40 per 1M tokens.
	// Audio rates inherit the standard audio multipliers (~$10 input / $20 output per docs).
	// Source: https://developers.openai.com/api/docs/models/gpt-audio-mini
	"gpt-audio-mini": {
		Ratio:           0.6 * ratio.MilliTokensUsd,
		CompletionRatio: 2.4 / 0.6,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           10.0 / 0.6, // $10/$0.6 ≈ 16.67x
			CompletionRatio:       2,          // $20/$10 = 2x
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Audio mini: cost-efficient audio chat model.",
	},
	"gpt-audio-mini-2025-12-15": {
		Ratio:           0.6 * ratio.MilliTokensUsd,
		CompletionRatio: 2.4 / 0.6,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           10.0 / 0.6,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Audio mini snapshot from 2025-12-15.",
	},
	"gpt-audio-mini-2025-10-06": {
		Ratio:           0.6 * ratio.MilliTokensUsd,
		CompletionRatio: 2.4 / 0.6,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           10.0 / 0.6,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Audio mini snapshot from 2025-10-06.",
	},
}
