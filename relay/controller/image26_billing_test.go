package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestGPTImage26Billing verifies exact published IDs use reservation validation,
// five-bucket settlement, ambiguity fallback, and refunds without guessed aliases.
func TestGPTImage26Billing(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-image-2.6-sunburst", "gpt-image-2.6-flare"} {
		t.Run(name, func(t *testing.T) {
			require.True(t, isGPTImage25Model(name))
			require.Error(t, validateGPTImage25Billing(name, 0, 1, 1, 1))
			require.NoError(t, validateGPTImage25Billing(name, 0.1, 1, 1, 1))
			require.Equal(t, int64(-75), imagePostConsumeDelta(name, 25, 100))

			usage := &relaymodel.Usage{
				PromptTokens:     150,
				CompletionTokens: 20,
				PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
					TextTokens:   100,
					ImageTokens:  50,
					CachedTokens: 30,
					CachedTokensDetails: &relaymodel.UsageCachedTokensDetails{
						TextTokens:  20,
						ImageTokens: 10,
					},
				},
			}
			want := (80*5.0 + 20*1.25 + 40*8.0 + 10*2.0 + 20*30.0) * billingratio.MilliTokensUsd
			require.InDelta(t, want, computeGptImageTokenQuota(name, usage, 1), 1e-9)
			usage.PromptTokensDetails.CachedTokensDetails = nil
			require.Zero(t, computeGptImageTokenQuota(name, usage, 1), "mixed cached input requires the operator fallback when the provider omits its split")
		})
	}
	for _, name := range []string{"gpt-image-2.6", "gpt-image-2.6-flare-2026-11-04", "gpt-image-2.6-sunburst-extra"} {
		require.False(t, isGPTImage25Model(name), "unpublished IDs must not inherit the exact-model billing policy")
	}
}
