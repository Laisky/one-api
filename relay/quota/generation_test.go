package quota_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// TestComputeGenerationReceiptErrorDistinction verifies a per-generation tariff
// keeps an unpriceable receipt distinct from an authoritative charge. Parameters:
// t owns the assertions. Returns: none; an invalid tools_cost keeps the flat price
// with an explicit issue, and an unresolved tariff reports no authoritative total.
func TestComputeGenerationReceiptErrorDistinction(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	paid := adaptor.ModelConfig{CompletionRatio: 1, PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 40},
		PricingProvenance: &adaptor.PricingProvenance{State: adaptor.TariffStatePaid, Unit: adaptor.TariffUnitGeneration, Source: "https://example.test/tariff", VerifiedAt: "2026-10-05"}}
	unknown := paid.Clone()
	unknown.PricingProvenance.State = adaptor.TariffStateUnknown
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{"music": paid, "unknown-music": unknown}}
	compute := func(name string, toolsCost int64) quotautil.ComputeResult {
		return quotautil.Compute(quotautil.ComputeInput{ModelName: name, GroupRatio: 1, PricingAdaptor: provider, RequestTime: at,
			Usage: &relaymodel.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30, ToolsCost: toolsCost}})
	}

	valid := compute("music", 0)
	require.False(t, valid.UnpricedUsage)
	require.Empty(t, valid.BillingIssues)
	require.EqualValues(t, 20000, valid.TotalQuota)

	for _, toolsCost := range []int64{-1, math.MaxInt64} {
		result := compute("music", toolsCost)
		require.True(t, result.UnpricedUsage)
		require.EqualValues(t, 20000, result.TotalQuota, "the flat generation stays priced")
		require.Equal(t, []string{"generation receipt tools_cost is invalid or exceeds supported range"}, result.BillingIssues)
	}

	unresolved := compute("unknown-music", 0)
	require.True(t, unresolved.UnpricedUsage)
	require.Zero(t, unresolved.TotalQuota)
	require.Equal(t, []string{"generation tariff could not be resolved"}, unresolved.BillingIssues)
}
