package quota_test

import (
	"github.com/Laisky/one-api/relay/quota"
	"math"
	"testing"
	"time"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestTierAdmissionHaikuBoundaries compares admission against settlement at the
// inclusive input boundary, without letting output or the buffer select a tier.
// Parameters: t owns assertions. Returns: none; both paths retain one tariff.
func TestTierAdmissionHaikuBoundaries(t *testing.T) {
	for _, prompt := range []int{99999, 100000, 100001} {
		input := quota.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: prompt, CompletionTokens: 128000}, ModelName: "claude-haiku-5-5", ModelRatio: .05, GroupRatio: 1, PricingAdaptor: &anthropic.Adaptor{}}
		settled := quota.Compute(input)
		quote, applies, err := quota.EstimateTierAdmission(input, 128000, 0, quota.AdmissionOptions{})
		require.NoError(t, err)
		require.True(t, applies)
		require.Equal(t, settled.TotalQuota, quote)
		buffered, _, err := quota.EstimateTierAdmission(input, 128000, 500, quota.AdmissionOptions{})
		require.NoError(t, err)
		require.Equal(t, int64(math.Ceil(float64(prompt)*settled.UsedModelRatio+128000*settled.UsedModelRatio*settled.UsedCompletionRatio+500*settled.UsedModelRatio)), buffered)
	}
	short := quota.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: 1}, ModelName: "claude-haiku-5-5", ModelRatio: .05, GroupRatio: 1, PricingAdaptor: &anthropic.Adaptor{}}
	quote, _, err := quota.EstimateTierAdmission(short, 128000, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.EqualValues(t, 32001, quote)
}

// TestTierAdmissionCacheBudget verifies separate Claude cache buckets select the
// full-input tier exactly once and only eligible write prices increase the hold.
// Parameters: t owns assertions. Returns: none; discounts never underreserve.
func TestTierAdmissionCacheBudget(t *testing.T) {
	input := quota.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: 2, PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 30000}, CacheWrite5mTokens: 30000, CacheWrite1hTokens: 39999}, ModelName: "claude-haiku-5-5", ModelRatio: .05, GroupRatio: 1, PricingAdaptor: &anthropic.Adaptor{}}
	quote, _, err := quota.EstimateTierAdmission(input, 128000, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.EqualValues(t, 210001, quote, "known write buckets remain eligible and full input is counted once")
	input.Usage = &relaymodel.Usage{PromptTokens: 100001}
	quote, _, err = quota.EstimateTierAdmission(input, 128000, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.EqualValues(t, 185001, quote)
	write5m, _, err := quota.EstimateTierAdmission(input, 128000, 0, quota.AdmissionOptions{CacheWrite5m: true})
	require.NoError(t, err)
	require.EqualValues(t, 191251, write5m)
	write1h, _, err := quota.EstimateTierAdmission(input, 128000, 0, quota.AdmissionOptions{CacheWrite1h: true})
	require.NoError(t, err)
	require.EqualValues(t, 210001, write1h)
	require.GreaterOrEqual(t, write1h, quota.Compute(input).TotalQuota)
}

// TestTierAdmissionOutputEnvelope covers decreasing output tiers and aggregate
// choices. Parameters: t owns assertions. Returns: none; every possible receipt
// through the provider-visible bound fits in the hold.
func TestTierAdmissionOutputEnvelope(t *testing.T) {
	cfg := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{OutputTokenThreshold: 10, CompletionRatio: 100}, {OutputTokenThreshold: 20, CompletionRatio: 1}}}
	input := quota.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: 100}, ModelName: "fixture", ModelRatio: 1, GroupRatio: 1, ChannelModelConfigs: map[string]model.ModelConfigLocal{"fixture": cfg}}
	for _, options := range []quota.AdmissionOptions{{}, {OutputCount: 2}} {
		cap := 30
		if options.OutputCount == 2 {
			cap = 10
		}
		quote, _, err := quota.EstimateTierAdmission(input, cap, 0, options)
		require.NoError(t, err)
		require.EqualValues(t, 2000, quote)
		for output := 0; output <= cap*max(options.OutputCount, 1); output++ {
			candidate := input
			candidate.Usage = &relaymodel.Usage{PromptTokens: 100, CompletionTokens: output}
			require.GreaterOrEqual(t, quote, quota.Compute(candidate).TotalQuota)
		}
	}
}

// TestTierAdmissionOverridesAndArithmetic preserves scalar, channel, window,
// group, free-price and checked-range contracts. Parameters: t owns assertions.
// Returns: none; valid rates match settlement and invalid quotes fail closed.
func TestTierAdmissionOverridesAndArithmetic(t *testing.T) {
	cfg := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 3}}}
	input := quota.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: 100, CompletionTokens: 10}, ModelName: "fixture", ModelRatio: 7, GroupRatio: 1.5, ChannelModelRatio: map[string]float64{"fixture": 7}, ChannelModelConfigs: map[string]model.ModelConfigLocal{"fixture": cfg}}
	quote, _, err := quota.EstimateTierAdmission(input, 10, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.Equal(t, quota.Compute(input).TotalQuota, quote)
	require.EqualValues(t, 1365, quote)
	input.GroupRatio = 0
	quote, _, err = quota.EstimateTierAdmission(input, 10, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.Zero(t, quote)
	input.GroupRatio = math.MaxFloat64
	_, _, err = quota.EstimateTierAdmission(input, 10, 0, quota.AdmissionOptions{})
	require.Error(t, err)
	input.GroupRatio = 1
	_, _, err = quota.EstimateTierAdmission(input, math.MaxInt, 0, quota.AdmissionOptions{OutputCount: 2})
	require.Error(t, err)
	_, _, err = quota.EstimateTierAdmission(input, 0, 0, quota.AdmissionOptions{})
	require.Error(t, err, "configured tier without known cap must fail closed")
	input.ModelName = "claude-haiku-5-5"
	input.ChannelModelConfigs = map[string]model.ModelConfigLocal{input.ModelName: {Ratio: 2}}
	input.PricingAdaptor = &anthropic.Adaptor{}
	_, applies, err := quota.EstimateTierAdmission(input, 128000, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.False(t, applies, "channel flat config intentionally suppresses provider tiers")
	input.ChannelModelConfigs = nil
	input.ChannelModelRatio = nil
	input.ModelRatio = .05
	quote, applies, err = quota.EstimateTierAdmission(input, 0, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.True(t, applies)
	require.EqualValues(t, 32005, quote, "known model output bound prices omitted pass-through limits")
	input.ModelName = "fixture"
	input.ModelRatio = 1
	input.RequestTime = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	cfg.TimeWindows = []model.TimeWindowLocal{{TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "11:00", End: "13:00"}}, Overlay: model.ModelConfigLocal{Ratio: 4}}}
	input.ChannelModelConfigs = map[string]model.ModelConfigLocal{"fixture": cfg}
	quote, _, err = quota.EstimateTierAdmission(input, 10, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.Equal(t, quota.Compute(input).TotalQuota, quote)
}

// TestTierAdmissionFreeGroupNeedsNoOutputBound preserves free admission for an
// unknown configured tier model whose output limit is omitted. Parameters: t
// owns assertions. Returns: none; a zero group rate needs no spending bound,
// while invalid token estimates still fail before the free-group shortcut.
func TestTierAdmissionFreeGroupNeedsNoOutputBound(t *testing.T) {
	t.Parallel()
	const name = "free-group-custom-tier"
	input := quota.ComputeInput{
		Usage: &relaymodel.Usage{PromptTokens: 100}, ModelName: name,
		ModelRatio: 1, GroupRatio: 0,
		ChannelModelConfigs: map[string]model.ModelConfigLocal{name: {
			Ratio: 1, CompletionRatio: 2,
			Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 3}},
		}},
	}
	quote, applies, err := quota.EstimateTierAdmission(input, 0, 0, quota.AdmissionOptions{})
	require.NoError(t, err)
	require.True(t, applies)
	require.Zero(t, quote)

	input.Usage = &relaymodel.Usage{PromptTokens: -1}
	_, applies, err = quota.EstimateTierAdmission(input, 0, 0, quota.AdmissionOptions{})
	require.True(t, applies)
	require.ErrorContains(t, err, "invalid tier admission token estimate")
}
