package quota

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestPromptTokensForTier checks bucket conventions, invalid counts and overflow.
// It takes a test handle and returns nothing.
func TestPromptTokensForTier(t *testing.T) {
	t.Parallel()
	usage := &model.Usage{PromptTokens: 2, CompletionTokens: 999999,
		PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 49999},
		CacheWrite5mTokens:  25000, CacheWrite1hTokens: 25000}
	require.Equal(t, 100001, promptTokensForTier("claude-haiku-5-5", usage))
	require.Equal(t, 100001, promptTokensForTier("us.anthropic.CLAUDE-haiku-5-5", usage))
	require.Equal(t, 2, promptTokensForTier("other-model", usage), "do not double-count inclusive usage")
	require.Zero(t, promptTokensForTier("claude-haiku-5-5", nil))
	usage.PromptTokens = math.MaxInt - 1
	require.Equal(t, math.MaxInt, promptTokensForTier("claude-haiku-5-5", usage))
	usage.PromptTokens = 1
	usage.PromptTokensDetails.CachedTokens = -100
	usage.CacheWrite5mTokens, usage.CacheWrite1hTokens = -200, -300
	require.Equal(t, 1, promptTokensForTier("claude-haiku-5-5", usage))
}
