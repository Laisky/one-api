package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
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

func TestClaudeOpus48PricingMatchesPublishedRatios(t *testing.T) {
	pricing, ok := ModelRatios["claude-opus-4-8"]
	require.True(t, ok, "Claude Opus 4.8 pricing missing from Anthropic model ratios")
	require.InDelta(t, 5*ratio.MilliTokensUsd, pricing.Ratio, 1e-12)
	require.InDelta(t, 5.0, pricing.CompletionRatio, 1e-12)
	require.InDelta(t, 0.5*ratio.MilliTokensUsd, pricing.CachedInputRatio, 1e-12)
	require.InDelta(t, 6.25*ratio.MilliTokensUsd, pricing.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 10.0*ratio.MilliTokensUsd, pricing.CacheWrite1hRatio, 1e-12)
	require.EqualValues(t, 1000000, pricing.ContextLength)
	require.EqualValues(t, 128000, pricing.MaxOutputTokens)
	require.Equal(t, claudeOpus47SamplingParams, pricing.SupportedSamplingParameters,
		"Opus 4.8 must inherit Opus 4.7's adaptive-only sampling restriction")
}

func TestClaudeFable5PricingMatchesPublishedRatios(t *testing.T) {
	pricing, ok := ModelRatios["claude-fable-5"]
	require.True(t, ok, "Claude Fable 5 pricing missing from Anthropic model ratios")
	require.InDelta(t, 10*ratio.MilliTokensUsd, pricing.Ratio, 1e-12)
	require.InDelta(t, 5.0, pricing.CompletionRatio, 1e-12)
	// Cache read = 0.1x base input ($10) = $1.00/MTok; the prior 1.25 value was the
	// 5-minute cache-WRITE multiplier (1.25x) miscopied as the cache-read price.
	require.InDelta(t, 1.0*ratio.MilliTokensUsd, pricing.CachedInputRatio, 1e-12)
	require.InDelta(t, 12.5*ratio.MilliTokensUsd, pricing.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 20.0*ratio.MilliTokensUsd, pricing.CacheWrite1hRatio, 1e-12)
	require.EqualValues(t, 1000000, pricing.ContextLength)
	require.EqualValues(t, 128000, pricing.MaxOutputTokens)
	// Fable 5 supports only adaptive thinking; thinking.budget_tokens is rejected, so
	// MaxReasoningTokens is intentionally unset (mirrors claude-opus-4-7/4-8).
	require.EqualValues(t, 0, pricing.MaxReasoningTokens)
}

func TestIsClaudeOpus47ModelCoversAdaptiveOpusVariants(t *testing.T) {
	cases := map[string]bool{
		"claude-opus-4-7":           true,
		"Claude-Opus-4-7":           true,
		"  claude-opus-4-7-future ": true,
		"claude-opus-4-8":           true,
		"claude-opus-4-8-fast":      true,
		"claude-opus-4-6":           false,
		"claude-opus-4-1":           false,
		"claude-opus-4-5-20251101":  false,
		"":                          false,
	}
	for name, want := range cases {
		require.Equalf(t, want, IsClaudeOpus47Model(name), "IsClaudeOpus47Model(%q)", name)
	}
}
