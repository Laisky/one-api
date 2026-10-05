package controller

import (
	"math"
	"math/big"
	"strconv"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// cohereSearchUnitsQuota prices a positive provider search-unit count using the
// selected decimal tariff and group rate. It rejects unrepresentable results
// instead of converting a floating-point overflow into a free or negative charge.
func cohereSearchUnitsQuota(units int64, unitRate, groupRate float64) (int64, bool) {
	if units <= 0 || math.IsNaN(unitRate) || math.IsInf(unitRate, 0) || unitRate < 0 ||
		math.IsNaN(groupRate) || math.IsInf(groupRate, 0) || groupRate < 0 {
		return 0, false
	}
	if unitRate == 0 || groupRate == 0 {
		return 0, true
	}
	unit, unitOK := new(big.Rat).SetString(strconv.FormatFloat(unitRate, 'g', -1, 64))
	group, groupOK := new(big.Rat).SetString(strconv.FormatFloat(groupRate, 'g', -1, 64))
	if !unitOK || !groupOK {
		return 0, false
	}
	cost := new(big.Rat).Mul(unit, group)
	cost.Mul(cost, new(big.Rat).SetInt64(units))
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(cost.Num(), cost.Denom(), remainder)
	if remainder.Sign() > 0 {
		whole.Add(whole, big.NewInt(1))
	}
	if !whole.IsInt64() || whole.Sign() < 0 {
		return 0, false
	}
	return whole.Int64(), true
}

// reconcileCohereSearchUnits chooses the measured search charge or retains the
// existing quote with explicit uncertainty. It is called only for Cohere's
// search-priced rerank path, never for unrelated token-priced rerank providers.
func reconcileCohereSearchUnits(usage *relaymodel.Usage, quoted int64, unitRate, groupRate float64) int64 {
	quoted = max(quoted, 0)
	if unitRate == 0 || groupRate == 0 {
		return 0
	}
	if usage == nil {
		return quoted
	}
	if usage.BilledSearchUnits == nil {
		if usage.BillingEstimateReason == "" {
			usage.BillingEstimateReason = "cohere_search_units_missing"
		}
		return quoted
	}
	if *usage.BilledSearchUnits <= 0 {
		usage.BillingEstimateReason = "cohere_search_units_invalid"
		return quoted
	}
	quota, ok := cohereSearchUnitsQuota(*usage.BilledSearchUnits, unitRate, groupRate)
	if !ok {
		usage.BillingEstimateReason = "cohere_search_units_cost_overflow"
		return quoted
	}
	return quota
}
