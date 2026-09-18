package quota_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestGeminiLivePublishedPricesThroughBilling exercises the production quota
// resolver, not just catalog multipliers. Parameters: t is the test handle.
// Returns: none. Prices are USD per million tokens, verified 2026-09-18 at
// https://ai.google.dev/gemini-api/docs/pricing#gemini-3.8-live.
func TestGeminiLivePublishedPricesThroughBilling(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gemini-3.8-live", "gemini-3.8-live-extended-thinking"} {
		for _, tc := range []struct {
			name   string
			tokens realtime.Tokens
			usd    float64
		}{
			{"text_input", realtime.Tokens{Input: 1_000_000, Text: 1_000_000}, 0.75},
			{"audio_input", realtime.Tokens{Input: 1_000_000, Audio: 1_000_000}, 3},
			{"image_input", realtime.Tokens{Input: 1_000_000, Image: 1_000_000}, 1},
			{"text_output", realtime.Tokens{Output: 1_000_000, OutputText: 1_000_000}, 4.5},
			{"audio_output", realtime.Tokens{Output: 1_000_000, OutputAudio: 1_000_000}, 12},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				ledger := realtime.NewLedger()
				ledger.Records = []realtime.Record{{Tokens: tc.tokens}}
				ledger.InputTokens, ledger.OutputTokens = tc.tokens.Input, tc.tokens.Output
				result := quota.Compute(quota.ComputeInput{
					Usage: &model.Usage{Realtime: ledger}, ModelName: name,
					ModelRatio: 0.75 * ratio.MilliTokensUsd, GroupRatio: 1,
					PricingAdaptor: &gemini.Adaptor{}, RequestTime: time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC),
				})
				t.Logf("model=%s bucket=%s quota=%d expected_usd=%g issues=%v", name, tc.name, result.TotalQuota, tc.usd, result.BillingIssues)
				require.False(t, result.UnpricedUsage, "published modality must have a price")
				require.EqualValues(t, tc.usd*ratio.QuotaPerUsd, result.TotalQuota)
			})
		}
	}
}
