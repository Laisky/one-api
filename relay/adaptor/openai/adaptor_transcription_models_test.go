package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGPTTranscriptionModels verifies transcription metadata and the duration-based rate used by RelayAudioHelper.
func TestGPTTranscriptionModels(t *testing.T) {
	t.Parallel()

	expectations := map[string]struct {
		usdPerMinute float64
	}{
		"gpt-transcribe":      {usdPerMinute: 0.0045},
		"gpt-live-transcribe": {usdPerMinute: 0.017},
	}

	for modelName, expectation := range expectations {
		cfg, ok := ModelRatios[modelName]
		require.Truef(t, ok, "ModelRatios must contain %q", modelName)
		require.NotNilf(t, cfg.Audio, "%s must have audio pricing metadata", modelName)

		require.InDeltaf(t, expectation.usdPerMinute/60, cfg.Audio.UsdPerSecond, 1e-12, "%s USD per second", modelName)
		require.Equalf(t, []string{"audio"}, cfg.InputModalities, "%s input modalities", modelName)
		require.Equalf(t, []string{"text"}, cfg.OutputModalities, "%s output modalities", modelName)
		require.Greaterf(t, cfg.Audio.PromptTokensPerSecond, 0.0, "%s prompt tokens per second", modelName)
		require.Zero(t, cfg.Audio.CompletionRatio)

		effectiveUSDPerMinute := cfg.Ratio * cfg.Audio.PromptTokensPerSecond * 60 / ratio.QuotaPerUsd
		require.InDeltaf(t, expectation.usdPerMinute, effectiveUSDPerMinute, 1e-12, "%s RelayAudioHelper rate", modelName)
	}
}

// TestTranscriptionModelDescriptionsDocumentResponseFormatLimits pins the catalog guidance
// that resolves a real production failure: OpenAI answers
// 400 unsupported_value "response_format 'vtt' is not compatible with model
// 'gpt-transcribe-api-ev3'" because only whisper-1 accepts srt/vtt/verbose_json on
// /v1/audio/transcriptions. The description is what a caller sees in the model list, so it
// is the one place that can steer them to the right model before they send the request.
//
// Source: https://developers.openai.com/api/docs/guides/speech-to-text
func TestTranscriptionModelDescriptionsDocumentResponseFormatLimits(t *testing.T) {
	t.Parallel()

	whisper, ok := ModelRatios["whisper-1"]
	require.True(t, ok, "ModelRatios must contain whisper-1")
	require.Contains(t, whisper.Description, "srt/vtt/verbose_json",
		"whisper-1 must advertise that it is the model for subtitle formats")

	jsonTextOnly := []string{"gpt-transcribe", "gpt-4o-transcribe", "gpt-4o-mini-transcribe"}
	for _, modelName := range jsonTextOnly {
		cfg, ok := ModelRatios[modelName]
		require.Truef(t, ok, "ModelRatios must contain %q", modelName)
		require.Containsf(t, cfg.Description, "json or text only",
			"%s must advertise its response_format restriction", modelName)
		require.Containsf(t, cfg.Description, "whisper-1",
			"%s must point at the model that does support subtitle formats", modelName)
	}
}
