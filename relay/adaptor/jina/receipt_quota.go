package jina

import (
	"math"
	"math/big"
	"strconv"

	"github.com/Laisky/errors/v2"
)

// MaxReceiptQuota is the same monetary representability ceiling used by
// admission. Reaching it requires an estimated-charge marker and reconciliation.
const MaxReceiptQuota int64 = 1 << 52

// ReceiptQuota prices nonnegative receipt evidence without reapplying admission's
// token-count limit. Large evidence must not be dropped or passed through an
// unchecked float-to-int conversion. Normal budgets reuse BudgetQuota exactly;
// larger evidence uses bounded decimal arithmetic. An unrepresentable positive
// cost returns the monetary ceiling AND an error, never a wrapped or zero fee.
func ReceiptQuota(input, output int, inputRatio, completionRatio, groupRatio float64) (int64, error) {
	if input < 0 || output < 0 {
		return 0, errors.New("negative Jina receipt evidence")
	}
	if input <= MaxBillingTokens && output <= MaxBillingTokens-input {
		amount, err := BudgetQuota(BillingBudget{Input: input, Output: output}, inputRatio, completionRatio, groupRatio)
		if err == nil {
			return amount, nil
		}
		// Distinguish invalid rates from an unrepresentable positive charge below.
	}
	rates := make([]*big.Rat, 3)
	for i, rate := range []float64{inputRatio, completionRatio, groupRatio} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			return 0, errors.New("invalid Jina receipt billing rate")
		}
		value, ok := new(big.Rat).SetString(strconv.FormatFloat(rate, 'g', -1, 64))
		if !ok {
			return 0, errors.New("invalid Jina receipt decimal rate")
		}
		rates[i] = value
	}
	cost := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(output)), rates[1])
	cost.Add(cost, new(big.Rat).SetInt64(int64(input)))
	cost.Mul(cost, rates[0])
	cost.Mul(cost, rates[2])
	if cost.Cmp(new(big.Rat).SetInt64(MaxReceiptQuota)) > 0 {
		return MaxReceiptQuota, errors.New("Jina receipt exceeds monetary range; reconciliation required")
	}
	units, remainder := new(big.Int), new(big.Int)
	units.QuoRem(cost.Num(), cost.Denom(), remainder)
	if remainder.Sign() > 0 {
		units.Add(units, big.NewInt(1))
	}
	return units.Int64(), nil
}
