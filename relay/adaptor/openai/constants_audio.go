package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// audioModelRatios captures pricing and metadata for OpenAI audio (TTS / STT)
// models. These models route through dedicated /audio endpoints; their
// chat-completions InputModalities is "text" only because OpenRouter's
// modality vocabulary does not include audio.
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
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "Whisper v1: speech-to-text transcription model.",
	},
	"tts-1": {
		Ratio:            15.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0, // $15.00 per 1M characters
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "TTS-1: standard-quality text-to-speech model.",
	},
	"tts-1-1106": {
		Ratio:            15.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "TTS-1 snapshot from 2023-11-06.",
	},
	"tts-1-hd": {
		Ratio:            30.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0, // $30.00 per 1M characters
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "TTS-1 HD: higher-fidelity text-to-speech model.",
	},
	"tts-1-hd-1106": {
		Ratio:            30.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
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
		InputModalities:  []string{"text"},
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
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "GPT-4o mini transcribe: cost-efficient speech-to-text model.",
	},
	"gpt-4o-mini-tts": {
		Ratio:            0.6 * ratio.MilliTokensUsd,
		CompletionRatio:  20.0, // $0.60 input, $12.00 output per 1M tokens
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "GPT-4o mini TTS: low-latency neural text-to-speech model.",
	},
}
