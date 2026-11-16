package minimax

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestMiniMaxNovemberPublishedFlash checks the released pay-as-you-go ID,
// independent tariff and effective controls using t. It returns no values.
func TestMiniMaxNovemberPublishedFlash(t *testing.T) {
	t.Parallel()
	const id = "MiniMax-M3.1-Flash"
	a := &Adaptor{}
	cfg, ok := a.GetDefaultModelPricing()[id]
	require.True(t, ok)
	require.Contains(t, a.GetModelList(), id)
	require.Contains(t, ModelList, id)
	require.InDelta(t, 1, a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 4, a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, .1, cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.EqualValues(t, 1_000_000, cfg.ContextLength)
	require.EqualValues(t, 524_288, cfg.MaxOutputTokens)
	require.ElementsMatch(t, []string{"text", "image", "video"}, cfg.InputModalities)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "max", cfg.DefaultReasoningEffort)
	require.NotContains(t, cfg.SupportedReasoningEfforts, "none")
	require.NotContains(t, cfg.SupportedSamplingParameters, "temperature")
	require.NotContains(t, cfg.SupportedSamplingParameters, "top_p")
	require.Empty(t, cfg.Tiers, "M3 long-input pricing does not apply to Flash")
	require.Empty(t, cfg.TimeWindows)
	require.Zero(t, cfg.CacheWrite5mRatio, "M2 write prices are not a Flash tariff")
	require.Contains(t, cfg.Description, "Native transport is not implemented")
	require.NotContains(t, a.GetModelList(), "MiniMax-M3.1-Flash-Preview")
	m3 := a.GetDefaultModelPricing()["MiniMax-M3"]
	require.EqualValues(t, 524_288, m3.MaxOutputTokens)
	require.NotContains(t, m3.SupportedSamplingParameters, "temperature")
	require.NotContains(t, m3.SupportedSamplingParameters, "top_p")
	require.Empty(t, m3.SupportedReasoningEfforts)
	copy := minimaxM31FlashModels()[id]
	copy.SupportedReasoningEfforts[0] = "changed"
	copy.InputModalities[0] = "changed"
	require.Equal(t, "low", minimaxM31FlashModels()[id].SupportedReasoningEfforts[0])
	require.Equal(t, "text", a.GetDefaultModelPricing()[id].InputModalities[0])
}
