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
