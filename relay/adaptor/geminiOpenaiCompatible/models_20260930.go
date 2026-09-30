package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Gemini catalog sources, verified 2026-09-30:
//   - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-tts
//   - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-lite-tts
//   - https://ai.google.dev/gemini-api/docs/pricing
//   - https://ai.google.dev/gemini-api/docs/changelog
//   - https://ai.google.dev/gemini-api/docs/deprecations
//
// Prices below are paid Standard Gemini Developer API USD per million tokens,
// not Batch/Flex/Priority prices or account-specific free allowances. Cache
// storage is a separate time-based charge and is not a cached-input token price.

// gemini2027PriceWindow builds the full-day UTC transition to published 2027 rates.
// Parameters: inputUsd, outputUsd, and cachedInputUsd are USD per million tokens.
// Returns: an independently allocated pricing window that preserves other metadata.
func gemini2027PriceWindow(inputUsd, outputUsd, cachedInputUsd float64) adaptor.TimeWindow {
	return adaptor.TimeWindow{
		Name:     "standard-pricing-from-2027",
		TimeZone: "UTC",
		DateFrom: "2027-01-01",
		Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
		Overlay: adaptor.ModelConfig{
			Ratio:            inputUsd * ratio.MilliTokensUsd,
			CompletionRatio:  outputUsd / inputUsd,
			CachedInputRatio: cachedInputUsd * ratio.MilliTokensUsd,
		},
	}
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
			// Audio completion pricing is relative to audio input in quota.Compute.
			// A unit prompt multiplier anchors that calculation to text input for
			// this text-only input model; it does not advertise audio input.
			PromptRatio:               1,
			CompletionRatio:           outputUsd / inputUsd,
			CompletionTokensPerSecond: 25,
		},
		TimeWindows: []adaptor.TimeWindow{
			// Input, audio output, and cached-input rates all double. The audio
			// multipliers remain unchanged and are preserved by the overlay.
			gemini2027PriceWindow(1.00, outputUsd*2, 0.25),
		},
		ContextLength:    8_192,
		MaxOutputTokens:  16_384,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description: description + " Generally available in the Gemini Developer API since September 22, 2026. " +
			"Uses speech_metadata for speaker/style and defaults to WAV for unary output. " +
			"Catalog metadata only: the Gemini 3.8 TTS request/response protocol is not implemented by this REST adaptor.",
	}
}

var geminiSeptember30LifecycleDescriptions = map[string]string{
	"gemini-3.1-flash-tts-preview": "Gemini 3.1 Flash TTS preview; no shutdown date is announced as of September 30, 2026. " +
		"For new workloads use gemini-3.8-flash-lite-tts; migrating requires the Gemini 3.8 TTS speech_metadata schema and WAV output handling.",
	"gemini-2.5-flash-preview-tts": "Gemini 2.5 Flash TTS preview; no shutdown date is announced as of September 30, 2026. " +
		"Consider gemini-3.8-flash-lite-tts for new workloads; migrating requires the Gemini 3.8 TTS protocol rather than an identifier-only replacement.",
	"gemini-2.5-pro-preview-tts": "Gemini 2.5 Pro TTS preview; no shutdown date is announced as of September 30, 2026. " +
		"Consider gemini-3.8-flash-tts for new workloads; migrating requires the Gemini 3.8 TTS protocol rather than an identifier-only replacement.",
	"gemini-omni-flash-preview": "Gemini Omni Flash preview video model; published earliest shutdown September 30, 2026. " +
		"Use gemini-omni-1.1-flash. Retaining this identifier does not guarantee upstream availability.",
}

// refreshGeminiSeptember30Catalog adds the September 22 TTS release and updates
// lifecycle guidance without deleting or redirecting existing model IDs.
// Parameters: none. Returns: none. The catalog init calls this before rebuilding
// ModelList so downstream adaptors observe the complete shared catalog.
func refreshGeminiSeptember30Catalog() {
	ModelRatios["gemini-3.8-flash-tts"] = gemini38TTSConfig(9.00,
		"Gemini 3.8 Flash TTS for expressive speech and long-form multi-speaker narration.")
	ModelRatios["gemini-3.8-flash-lite-tts"] = gemini38TTSConfig(6.00,
		"Gemini 3.8 Flash-Lite TTS for high-throughput, low-latency speech generation.")

	for model, description := range geminiSeptember30LifecycleDescriptions {
		config, ok := ModelRatios[model]
		if !ok {
			continue
		}
		config.Description = description
		ModelRatios[model] = config
	}
	for _, model := range []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"} {
		config := ModelRatios[model]
		config.Description = geminiSeptember2026LifecycleDescriptions[model] + " Since September 18, 2026, Gemini Developer API access is limited to users with prior active usage. " +
			"These models are not deprecated; use gemini-3.8-flash or gemini-3.5-flash-lite for new projects."
		ModelRatios[model] = config
	}
}
