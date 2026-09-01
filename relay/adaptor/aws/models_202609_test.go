package aws

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestClaudeSeptember2026BedrockPricing verifies that the canonical Bedrock
// pricing registry exposes the September 2026 Claude rates and metadata. It
// accepts a testing handle and returns no value.
func TestClaudeSeptember2026BedrockPricing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		inputUsd         float64
		outputMultiplier float64
		cacheReadUsd     float64
		cacheWrite5mUsd  float64
		cacheWrite1hUsd  float64
	}{
		{name: "claude-fable-5-1", inputUsd: 10, outputMultiplier: 5, cacheReadUsd: 0.25, cacheWrite5mUsd: 12.5, cacheWrite1hUsd: 20},
		{name: "claude-mythos-5-1", inputUsd: 10, outputMultiplier: 5, cacheReadUsd: 0.25, cacheWrite5mUsd: 12.5, cacheWrite1hUsd: 20},
		{name: "claude-sonnet-5", inputUsd: 2, outputMultiplier: 5, cacheReadUsd: 0.2, cacheWrite5mUsd: 2.5, cacheWrite1hUsd: 4},
	}

	pricing := (&Adaptor{}).GetDefaultModelPricing()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config, ok := pricing[tt.name]
			require.True(t, ok)
			require.InDelta(t, tt.inputUsd*ratio.MilliTokensUsd, config.Ratio, 1e-12)
			require.InDelta(t, tt.outputMultiplier, config.CompletionRatio, 1e-12)
			require.InDelta(t, tt.cacheReadUsd*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
			require.InDelta(t, tt.cacheWrite5mUsd*ratio.MilliTokensUsd, config.CacheWrite5mRatio, 1e-12)
			require.InDelta(t, tt.cacheWrite1hUsd*ratio.MilliTokensUsd, config.CacheWrite1hRatio, 1e-12)
			require.EqualValues(t, 1000000, config.ContextLength)
			require.EqualValues(t, 128000, config.MaxOutputTokens)
			require.Equal(t, awsClaudeAdaptiveOnlySamplingParams, config.SupportedSamplingParameters)
			require.Empty(t, config.TimeWindows)
		})
	}
}

// TestClaudeSeptember2026BedrockModelsAreRoutable verifies that the parent AWS
// registry exposes adapters for both Claude 5.1 model IDs. It accepts a testing
// handle and returns no value.
func TestClaudeSeptember2026BedrockModelsAreRoutable(t *testing.T) {
	t.Parallel()

	modelList := (&Adaptor{}).GetModelList()
	for _, model := range []string{"claude-fable-5-1", "claude-mythos-5-1"} {
		require.Contains(t, modelList, model)
		require.NotNil(t, GetAdaptor(model))
	}
}
