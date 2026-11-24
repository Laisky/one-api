package quota_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// TestGPT56SolPromotionCutoff checks the dated default tariff across the UTC
// cutoff and context threshold for both admission and settlement. Parameter t
// reports any rate or request-time mismatch.
func TestGPT56SolPromotionCutoff(t *testing.T) {
	t.Parallel()

	provider := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	cutoff := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"gpt-5.6-sol", "gpt-5.6"} {
		for _, row := range []struct {
			at                           time.Time
			prompt, threshold            int
			input, cached, write, output float64
		}{
			{cutoff.Add(-time.Nanosecond), 272_000, 0, 4, .40, 5, 20},
			{cutoff.Add(-time.Nanosecond), 272_001, 272_001, 8, .80, 10, 30},
			{cutoff, 272_000, 0, 5, .50, 6.25, 30},
			{cutoff, 272_001, 272_001, 10, 1, 12.50, 45},
			{cutoff.AddDate(0, 0, 1), 272_000, 0, 5, .50, 6.25, 30},
		} {
			t.Run(fmt.Sprintf("%s/%s/%d", name, row.at.Format(time.RFC3339Nano), row.prompt), func(t *testing.T) {
				cfg, ok := pricing.ResolveModelConfig(name, nil, provider, row.at)
				require.True(t, ok)
				effective := pricing.ResolveEffectivePricingForUsageFromConfig(row.prompt, 200, cfg)
				require.Equal(t, row.threshold, effective.AppliedTierThreshold)
				require.InDelta(t, row.input, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, row.cached, effective.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, row.write, effective.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, row.output, effective.OutputRatio/ratio.MilliTokensUsd, 1e-9)
				require.Zero(t, effective.CacheWrite1hRatio)

				// Admission and final usage both carry the original request start
				// time, including a request admitted just before the cutoff.
				input := quota.ComputeInput{
					ModelName: name, PricingAdaptor: provider, RequestTime: row.at,
					ModelRatio: pricing.ResolveModelRatioAt(name, nil, nil, provider, row.at),
					GroupRatio: 1,
					Usage:      &relaymodel.Usage{PromptTokens: row.prompt, CompletionTokens: 200},
				}
				hold, applied, err := quota.EstimateTierAdmission(input, 200, 0, quota.AdmissionOptions{CacheWrite5m: true})
				require.NoError(t, err)
				require.True(t, applied)
				require.Equal(t, int64(math.Ceil((float64(row.prompt)*row.write+200*row.output)*ratio.MilliTokensUsd)), hold)

				input.Usage.CacheWrite5mTokens = 300
				input.Usage.PromptTokensDetails = &relaymodel.UsagePromptTokensDetails{CachedTokens: 400}
				want := (float64(row.prompt-700)*row.input + 400*row.cached + 300*row.write + 200*row.output) * ratio.MilliTokensUsd
				actual := quota.Compute(input).TotalQuota
				require.Equal(t, int64(math.Ceil(want)), actual)
				require.GreaterOrEqual(t, hold, actual)
			})
		}
	}
}

// TestGPT56SolPromotionPreservesPricingBoundaries checks that a channel tariff,
// other OpenAI model IDs and inherited provider pricing are not rescheduled.
// Parameter t reports unexpected pricing changes.
func TestGPT56SolPromotionPreservesPricingBoundaries(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	native := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	const name = "gpt-5.6-sol"
	overrides := map[string]model.ModelConfigLocal{
		name: {Ratio: 7, CompletionRatio: 3, CachedInputRatio: 2, CacheWrite5mRatio: 8},
	}
	cfg, ok := pricing.ResolveModelConfig(name, overrides, native, at)
	require.True(t, ok)
	override := pricing.ResolveEffectivePricingForUsageFromConfig(272_001, 200, cfg)
	require.Equal(t, float64(7), override.InputRatio)
	require.Equal(t, float64(21), override.OutputRatio)
	require.Equal(t, float64(2), override.CachedInputRatio)
	require.Equal(t, float64(8), override.CacheWrite5mRatio)
	require.Zero(t, override.AppliedTierThreshold)

	for _, p := range []struct {
		name                  string
		provider              *openai.Adaptor
		input, cached, output float64
	}{
		{name, &openai.Adaptor{ChannelType: channeltype.Azure}, 4, .4, 20},
		{"gpt-daybreak-blue-latest", native, 4, .4, 20},
		{"gpt-5.6-cyber", native, 12.5, 1.25, 75},
		{"gpt-6.2-sol", native, 2, .1, 10},
	} {
		cfg, ok := pricing.ResolveModelConfig(p.name, nil, p.provider, at)
		require.True(t, ok, p.name)
		actual := pricing.ResolveEffectivePricingFromConfig(100, cfg)
		require.InDelta(t, p.input, actual.InputRatio/ratio.MilliTokensUsd, 1e-9, p.name)
		require.InDelta(t, p.cached, actual.CachedInputRatio/ratio.MilliTokensUsd, 1e-9, p.name)
		require.InDelta(t, p.output, actual.OutputRatio/ratio.MilliTokensUsd, 1e-9, p.name)
	}
}
