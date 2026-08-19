package groq

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

func TestCurrentGroqTokenPricing(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		inputUSDPerMillion  float64
		cachedUSDPerMillion float64
		outputUSDPerMillion float64
		contextLength       int32
		maxOutputTokens     int32
	}{
		"openai/gpt-oss-120b": {
			inputUSDPerMillion:  0.15,
			cachedUSDPerMillion: 0.075,
			outputUSDPerMillion: 0.60,
			contextLength:       131_072,
			maxOutputTokens:     65_536,
		},
		"openai/gpt-oss-20b": {
			inputUSDPerMillion:  0.075,
			cachedUSDPerMillion: 0.0375,
			outputUSDPerMillion: 0.30,
			contextLength:       131_072,
			maxOutputTokens:     65_536,
		},
		"meta-llama/llama-prompt-guard-2-22m": {
			inputUSDPerMillion:  0.03,
			outputUSDPerMillion: 0.03,
			contextLength:       512,
			maxOutputTokens:     512,
		},
		"meta-llama/llama-prompt-guard-2-86m": {
			inputUSDPerMillion:  0.04,
			outputUSDPerMillion: 0.04,
			contextLength:       512,
			maxOutputTokens:     512,
		},
		"openai/gpt-oss-safeguard-20b": {
			inputUSDPerMillion:  0.075,
			cachedUSDPerMillion: 0.0375,
			outputUSDPerMillion: 0.30,
			contextLength:       131_072,
			maxOutputTokens:     65_536,
		},
		"qwen/qwen3.6-27b": {
			inputUSDPerMillion:  0.60,
			outputUSDPerMillion: 3.00,
			contextLength:       131_072,
			maxOutputTokens:     16_384,
		},
	}

	for modelID, want := range tests {
		modelID, want := modelID, want
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()

			got, ok := ModelRatios[modelID]
			require.True(t, ok)
			require.InDelta(t, want.inputUSDPerMillion, got.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, want.cachedUSDPerMillion, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, want.outputUSDPerMillion, got.Ratio*got.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			require.Equal(t, want.contextLength, got.ContextLength)
			require.Equal(t, want.maxOutputTokens, got.MaxOutputTokens)
			require.NotEmpty(t, got.Description)
		})
	}
}

func TestCurrentGroqNonTokenPricing(t *testing.T) {
	t.Parallel()

	whisper, ok := ModelRatios["whisper-large-v3"]
	require.True(t, ok)
	require.NotNil(t, whisper.Audio)
	require.InDelta(t, 0.111/3600, whisper.Audio.UsdPerSecond, 1e-12)

	whisperTurbo, ok := ModelRatios["whisper-large-v3-turbo"]
	require.True(t, ok)
	require.NotNil(t, whisperTurbo.Audio)
	require.InDelta(t, 0.04/3600, whisperTurbo.Audio.UsdPerSecond, 1e-12)

	require.InDelta(t, 40.0, ModelRatios["canopylabs/orpheus-arabic-saudi"].Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 22.0, ModelRatios["canopylabs/orpheus-v1-english"].Ratio/ratio.MilliTokensUsd, 1e-12)

	for _, modelID := range []string{"groq/compound", "groq/compound-mini", "minimaxai/minimax-m2.7"} {
		require.Zero(t, ModelRatios[modelID].Ratio)
	}
}

func TestSafeguardCapabilities(t *testing.T) {
	t.Parallel()

	safeguard := ModelRatios["openai/gpt-oss-safeguard-20b"]
	require.Contains(t, safeguard.SupportedFeatures, "web_search")
	require.Contains(t, safeguard.SupportedFeatures, "structured_outputs")
	require.Equal(t, []string{"low", "medium", "high"}, safeguard.SupportedReasoningEfforts)
}
