package quota_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestGeminiLiveMixedSessionPricing uses the production resolver and a disjoint
// multimodal ledger. Parameters: t is a test. Returns: none. Reasoning tokens
// are a subset of output text, and transcript strings incur no extra surcharge.
func TestGeminiLiveMixedSessionPricing(t *testing.T) {
	t.Parallel()
	tokens := realtime.Tokens{Input: 1_000_000, Text: 100_000, Audio: 200_000, Image: 300_000, Video: 400_000, Output: 500_000, OutputText: 200_000, OutputAudio: 300_000, ReasoningTokens: 100_000}
	for _, tc := range []struct {
		name  string
		group float64
		want  int64
	}{
		{"ordinary", 1, 2937500}, {"group_once", 2, 5875000}, {"free", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := realtime.NewLedger()
			l.Records = []realtime.Record{{Tokens: tokens}}
			l.InputTokens, l.OutputTokens = tokens.Input, tokens.Output
			result := quota.Compute(quota.ComputeInput{Usage: &model.Usage{Realtime: l}, ModelName: "gemini-3.8-live", ModelRatio: .75 * ratio.MilliTokensUsd, GroupRatio: tc.group, PricingAdaptor: &gemini.Adaptor{}})
			// USD: .075+.600+.300+.400+.900+3.600 = 5.875. Reasoning is not added twice.
			require.Equal(t, tc.want, result.TotalQuota)
			require.False(t, result.UnpricedUsage)
		})
	}
}

// TestGeminiLiveOverridesAndUnknownCache covers price overrides, one final
// rounding and refusal to borrow GPT cache discounts. Parameters: t is a test.
// Returns: none; neither original records nor provider defaults are mutated.
func TestGeminiLiveOverridesAndUnknownCache(t *testing.T) {
	t.Parallel()
	l := realtime.NewLedger()
	l.Records = []realtime.Record{{Tokens: realtime.Tokens{Input: 1, Text: 1}}, {Tokens: realtime.Tokens{Input: 1, Text: 1}}}
	l.InputTokens = 2
	input := quota.ComputeInput{Usage: &model.Usage{Realtime: l}, ModelName: "gemini-3.8-live", ModelRatio: .75 * ratio.MilliTokensUsd, GroupRatio: 1, PricingAdaptor: &gemini.Adaptor{}}
	require.EqualValues(t, 1, quota.Compute(input).TotalQuota, "round only after summing the session")
	input.ChannelModelConfigs = map[string]dbmodel.ModelConfigLocal{"gemini-3.8-live": {Ratio: 1, CompletionRatio: 6}}
	input.ModelRatio = 1
	require.EqualValues(t, 2, quota.Compute(input).TotalQuota)
	input.ChannelModelConfigs = nil
	input.ModelRatio = .75 * ratio.MilliTokensUsd
	l.Records = []realtime.Record{{Tokens: realtime.Tokens{Input: 100, Audio: 100, CachedAudio: 50}}}
	l.InputTokens = 100
	result := quota.Compute(input)
	require.True(t, result.UnpricedUsage)
	require.NotEmpty(t, result.BillingIssues)
	require.EqualValues(t, 75, result.TotalQuota, "only the uncached, priceable audio is charged")
	require.EqualValues(t, 50, l.Records[0].Tokens.CachedAudio)
}
