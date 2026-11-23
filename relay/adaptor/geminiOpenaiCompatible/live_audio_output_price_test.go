package geminiOpenaiCompatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestLiveAudioOutputPricesMatchPublishedTariffs pins the audio-output price
// the relay derives from each Live catalog entry to Google's published USD
// per million tokens. Both the Live settlement (quota/realtime.go) and REST
// output-audio billing price audio output as text input rate x
// Audio.PromptRatio x Audio.CompletionRatio, so Audio.CompletionRatio is
// relative to the audio INPUT price, not to the text output price.
// Sources: https://aistudio.google.com/docs/pricing (verified 2026-11-23).
// Parameters: t is the test handle. Returns: none.
func TestLiveAudioOutputPricesMatchPublishedTariffs(t *testing.T) {
	for name, want := range map[string]struct{ audioIn, audioOut float64 }{
		"gemini-3.8-live":                               {audioIn: 3.00, audioOut: 12.00},
		"gemini-3.1-flash-live-preview":                 {audioIn: 3.00, audioOut: 12.00},
		"gemini-2.5-flash-native-audio":                 {audioIn: 3.00, audioOut: 12.00},
		"gemini-2.5-flash-native-audio-preview-09-2025": {audioIn: 3.00, audioOut: 12.00},
		"gemini-2.5-flash-native-audio-preview-12-2025": {audioIn: 3.00, audioOut: 12.00},
		"gemini-3.5-live-translate-preview":             {audioIn: 3.50, audioOut: 21.00},
	} {
		cfg, ok := ModelRatios[name]
		require.True(t, ok, name)
		require.NotNil(t, cfg.Audio, name)
		textIn := cfg.Ratio / ratio.MilliTokensUsd
		audioIn := textIn * cfg.Audio.PromptRatio
		require.InDelta(t, want.audioIn, audioIn, 1e-9, "%s audio input USD/M", name)
		require.InDelta(t, want.audioOut, audioIn*cfg.Audio.CompletionRatio, 1e-9, "%s audio output USD/M", name)
	}
}
