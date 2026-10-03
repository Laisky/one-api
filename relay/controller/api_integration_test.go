package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// This file used to contain 285 lines of test theater: every case computed the
// quota with a formula reimplemented inside the test and then asserted that
// reimplementation against itself, e.g.
//
//	responseQuota := int64((float64(in)+float64(out)*cr)*mr*gr) + tools
//	chatQuota     := int64((float64(in)+float64(out)*cr)*mr*gr) + tools
//	require.Equal(t, chatQuota, responseQuota)
//
// Zero production code ran, so a change to relay/quota.Compute — dropping the
// completion ratio, applying tools cost before the ratio multiply — passed
// unnoticed while the file's name promised the billing path was covered. It is
// replaced here by tests that call the real function.

// quotaPricingAdaptor prices a single model so Compute has a provider layer to
// resolve against, without depending on the live adaptor catalog.
type quotaPricingAdaptor struct {
	adaptor.Adaptor
	pricing map[string]adaptor.ModelConfig
}

// GetDefaultModelPricing returns the fixture's pricing table.
//
// Return values:
//   - map[string]adaptor.ModelConfig: the configured pricing.
func (a *quotaPricingAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return a.pricing
}

// TestComputeAppliesCompletionRatioAndToolsCost pins the shape of the billing
// formula against the real relay/quota.Compute.
//
// Each expectation is written as an independent literal, not as a restatement of
// the implementation: the point is to fail when the implementation changes.
func TestComputeAppliesCompletionRatioAndToolsCost(t *testing.T) {
	t.Parallel()

	const modelName = "test-model"
	// 1 quota unit per token in, so the arithmetic below is readable.
	provider := &quotaPricingAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {Ratio: 1, CompletionRatio: 3},
	}}

	for _, tc := range []struct {
		name       string
		prompt     int
		completion int
		toolsCost  int64
		groupRatio float64
		want       int64
	}{
		{
			// 100 in + 10 out x3 = 130, group ratio 1.
			name: "completion tokens cost the completion ratio", prompt: 100, completion: 10, groupRatio: 1, want: 130,
		},
		{
			// Tools cost is added AFTER the ratio multiply, not before: with a group
			// ratio of 2 a pre-multiply tools cost would give (130+8)*2 = 276.
			name: "tools cost is added after the group ratio", prompt: 100, completion: 10, toolsCost: 8, groupRatio: 2, want: 268,
		},
		{
			name: "a request with no completion tokens bills input only", prompt: 50, completion: 0, groupRatio: 1, want: 50,
		},
		{
			name: "group ratio scales the whole token cost", prompt: 100, completion: 10, groupRatio: 0.5, want: 65,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := quota.Compute(quota.ComputeInput{
				Usage: &relaymodel.Usage{
					PromptTokens:     tc.prompt,
					CompletionTokens: tc.completion,
					ToolsCost:        tc.toolsCost,
				},
				ModelName:      modelName,
				ModelRatio:     1,
				GroupRatio:     tc.groupRatio,
				PricingAdaptor: provider,
				RequestTime:    time.Now(),
			})

			require.Equal(t, tc.want, result.TotalQuota)
			require.Equal(t, tc.prompt, result.PromptTokens)
			require.Equal(t, tc.completion, result.CompletionTokens)
		})
	}
}

// TestComputeChargesCachedPromptTokensAtTheCachedRate pins that a model with a
// distinct cached-input price bills cache hits more cheaply than fresh input —
// the single most valuable property of the cached-pricing path, and one no test
// in this file previously exercised.
func TestComputeChargesCachedPromptTokensAtTheCachedRate(t *testing.T) {
	t.Parallel()

	const modelName = "cached-model"
	provider := &quotaPricingAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {Ratio: 10, CompletionRatio: 1, CachedInputRatio: 1},
	}}

	compute := func(cached int) quota.ComputeResult {
		return quota.Compute(quota.ComputeInput{
			Usage: &relaymodel.Usage{
				PromptTokens: 100,
				PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
					CachedTokens: cached,
				},
			},
			ModelName:      modelName,
			ModelRatio:     10,
			GroupRatio:     1,
			PricingAdaptor: provider,
			RequestTime:    time.Now(),
		})
	}

	uncached := compute(0)
	halfCached := compute(50)

	require.Equal(t, 50, halfCached.CachedPromptTokens)
	require.Less(t, halfCached.TotalQuota, uncached.TotalQuota,
		"cache hits must cost less than fresh input tokens")
	// 50 fresh at 10 + 50 cached at 1 = 550, against 100 fresh at 10 = 1000.
	require.Equal(t, int64(1000), uncached.TotalQuota)
	require.Equal(t, int64(550), halfCached.TotalQuota)
}

// TestComputeIsSafeOnMissingUsage pins that the billing path cannot panic or
// invent a charge when an upstream returns no usage block.
func TestComputeIsSafeOnMissingUsage(t *testing.T) {
	t.Parallel()

	result := quota.Compute(quota.ComputeInput{
		Usage:       nil,
		ModelName:   "test-model",
		ModelRatio:  1,
		GroupRatio:  1,
		RequestTime: time.Now(),
	})
	require.Zero(t, result.TotalQuota)

	// A model the provider cannot price must still produce a finite, non-negative
	// charge rather than NaN or a negative quota.
	unpriced := quota.Compute(quota.ComputeInput{
		Usage:       &relaymodel.Usage{PromptTokens: 10, CompletionTokens: 10},
		ModelName:   "no-such-model",
		ModelRatio:  billingratio.MilliTokensUsd,
		GroupRatio:  1,
		RequestTime: time.Now(),
	})
	require.GreaterOrEqual(t, unpriced.TotalQuota, int64(0))
}
