package quota

import (
	"math"

	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// reportedCostFloorQuota converts a provider-reported request cost into the
// quota floor settlement must not go below. The floor applies the same group
// ratio as the catalog price, so a free group stays free. Parameters: input
// carries the usage, group ratio and pricing adaptor. It returns the floor and
// true only when the adaptor declares its reported cost authoritative and the
// receipt carries a positive, finite cost.
func reportedCostFloorQuota(input ComputeInput) (int64, bool) {
	if input.Usage == nil || !adaptor.BillsAtLeastReportedCost(input.PricingAdaptor) {
		return 0, false
	}
	costUSD := input.Usage.UpstreamCostUSD
	if math.IsNaN(costUSD) || math.IsInf(costUSD, 0) || costUSD <= 0 || input.GroupRatio <= 0 {
		return 0, false
	}
	floor := math.Ceil(costUSD * billingratio.QuotaPerUsd * input.GroupRatio)
	if math.IsNaN(floor) || math.IsInf(floor, 0) || floor >= float64(math.MaxInt64/2) {
		return 0, false
	}
	return int64(floor), true
}
