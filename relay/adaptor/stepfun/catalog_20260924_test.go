package stepfun

import (
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestStep5PublishedCatalog pins the domestic tariff and API capabilities with
// t. It does not conflate international USD prices or promised open weights.
func TestStep5PublishedCatalog(t *testing.T) {
	t.Parallel()
	require.Contains(t, ModelList, "step-5-preview")
	c := ModelRatios["step-5-preview"]
	require.InDelta(t, 7, c.Ratio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 20, c.Ratio*c.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, .35, c.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.EqualValues(t, 1_000_000, c.ContextLength)
	require.EqualValues(t, 64_000, c.MaxOutputTokens)
	require.Zero(t, c.MaxTokens)
	require.Equal(t, []string{"text", "image", "video"}, c.InputModalities)
	require.Equal(t, []string{"low", "medium", "high"}, c.SupportedReasoningEfforts)
	require.Empty(t, c.HuggingFaceID)
	require.Empty(t, c.Quantization)
}
