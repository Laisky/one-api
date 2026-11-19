package quota_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// reportedCostAdaptor is a catalog-priced adaptor that declares its reported
// per-request cost authoritative.
type reportedCostAdaptor struct{ *stubQuotaAdaptor }

// BillsAtLeastReportedCost implements adaptor.ReportedCostBillingPolicy. It
// takes no parameters and returns true.
func (reportedCostAdaptor) BillsAtLeastReportedCost() bool { return true }

// TestComputeFloorsAtReportedCost proves settlement never bills less than a
// declared provider-reported cost times the group ratio, keeps the catalog
// price when it is higher, ignores the cost for adaptors that do not declare
// it authoritative, and keeps a free group free.
func TestComputeFloorsAtReportedCost(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	base := func(provider adaptor.Adaptor, cost, group float64) quotautil.ComputeResult {
		return quotautil.Compute(quotautil.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: 1_000, CompletionTokens: 100, UpstreamCostUSD: cost},
			ModelName: "gpt-4o-mini", ModelRatio: 0.075, GroupRatio: group, PricingAdaptor: provider, RequestTime: now})
	}
	catalogPricing := map[string]adaptor.ModelConfig{"gpt-4o-mini": {Ratio: 0.075, CompletionRatio: 4}}
	declared := &reportedCostAdaptor{&stubQuotaAdaptor{pricing: catalogPricing}}
	catalog := base(declared, 0, 1)
	require.Positive(t, catalog.TotalQuota)
	require.False(t, catalog.ReportedCostFloor)

	high := base(declared, 2, 1.5)
	require.Equal(t, int64(2*500_000*1.5), high.TotalQuota, "cost x QuotaPerUsd x group ratio")
	require.True(t, high.ReportedCostFloor)

	low := base(declared, 0.0000001, 1)
	require.Equal(t, catalog.TotalQuota, low.TotalQuota, "a cheaper reported cost keeps the catalog price")
	require.False(t, low.ReportedCostFloor)

	require.Equal(t, catalog.TotalQuota, base(&stubQuotaAdaptor{pricing: catalogPricing}, 2, 1).TotalQuota, "undeclared adaptors ignore the reported cost")
	require.Zero(t, base(declared, 2, 0).TotalQuota, "a free group stays free")
}
