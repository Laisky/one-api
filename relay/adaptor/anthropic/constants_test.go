package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

func TestClaudeWebSearchPricingApplied(t *testing.T) {
	pricing, ok := AnthropicToolingDefaults.Pricing["web_search"]
	require.True(t, ok, "web search pricing missing for anthropic defaults")
	require.InDelta(t, 0.01, pricing.UsdPerCall, 1e-9, "expected $0.01 per call for web search")
	require.Empty(t, AnthropicToolingDefaults.Whitelist, "expected anthropic default allowlist to be inferred from pricing")

	keys := make([]string, 0, len(AnthropicToolingDefaults.Pricing))
	for name := range AnthropicToolingDefaults.Pricing {
		keys = append(keys, name)
	}
	require.ElementsMatch(t, []string{"web_search"}, keys, "expected pricing map to enumerate anthropic built-in tools")
}

func TestClaudeOpus47PricingMatchesPublishedRatios(t *testing.T) {
	pricing, ok := ModelRatios["claude-opus-4-7"]
	require.True(t, ok, "Claude Opus 4.7 pricing missing from Anthropic model ratios")
	require.InDelta(t, 5*ratio.MilliTokensUsd, pricing.Ratio, 1e-12)
	require.InDelta(t, 5.0, pricing.CompletionRatio, 1e-12)
	require.InDelta(t, 0.5*ratio.MilliTokensUsd, pricing.CachedInputRatio, 1e-12)
	require.InDelta(t, 6.25*ratio.MilliTokensUsd, pricing.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 10.0*ratio.MilliTokensUsd, pricing.CacheWrite1hRatio, 1e-12)
}
