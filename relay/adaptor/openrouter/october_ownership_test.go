package openrouter

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberOpenRouterOwnership checks independent constructor allocations for
// slices, audio prices, tiers and schedules using t. It returns no values.
func TestOctoberOpenRouterOwnership(t *testing.T) {
	t.Parallel()
	first, second := octoberChatModels(), octoberChatModels()
	first["meta/muse-wonder"].InputModalities[0] = "changed"
	first["meta/muse-wonder"].SupportedReasoningEfforts[0] = "changed"
	first["anthropic/claude-haiku-5.5"].Tiers[0].Ratio = 99
	first["google/gemini-3.8-flash"].Audio.PromptRatio = 99
	first["tencent/hy3"].TimeWindows[0].Ranges[0].Start = "12:00"
	require.Equal(t, "text", second["meta/muse-wonder"].InputModalities[0])
	require.Equal(t, "max", second["meta/muse-wonder"].SupportedReasoningEfforts[0])
	require.NotEqualValues(t, 99, second["anthropic/claude-haiku-5.5"].Tiers[0].Ratio)
	require.EqualValues(t, 1, second["google/gemini-3.8-flash"].Audio.PromptRatio)
	require.Equal(t, "00:00", second["tencent/hy3"].TimeWindows[0].Ranges[0].Start)
}
