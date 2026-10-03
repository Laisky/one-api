package controller

import (
	"math"
	"math/big"
	"strconv"

	"github.com/Laisky/errors/v2"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// videoQuota prices rendered seconds and input images at the configured group
// rate, returning rounded-up quota units. Decimal rational arithmetic avoids
// charging an extra unit for binary noise such as (0.40+0.01)*500000.
func videoQuota(perSecond, multiplier, duration, perImage float64, images int, group float64) (int64, error) {
	values := []float64{perSecond, multiplier, duration, perImage, group}
	rates := make([]*big.Rat, len(values))
	for i, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return 0, errors.New("video prices and group ratio must be finite and nonnegative")
		}
		rate, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'f', -1, 64))
		if !ok {
			return 0, errors.New("invalid decimal video rate")
		}
		rates[i] = rate
	}
	if images < 0 || multiplier <= 0 || duration <= 0 {
		return 0, errors.New("invalid video billing quantities")
	}
	cost := new(big.Rat).Mul(rates[0], rates[1])
	cost.Mul(cost, rates[2])
	cost.Add(cost, new(big.Rat).Mul(rates[3], big.NewRat(int64(images), 1)))
	cost.Mul(cost, rates[4])
	cost.Mul(cost, big.NewRat(billingratio.QuotaPerUsd, 1))
	quota, remainder := new(big.Int), new(big.Int)
	quota.QuoRem(cost.Num(), cost.Denom(), remainder)
	if remainder.Sign() != 0 {
		quota.Add(quota, big.NewInt(1))
	}
	if !quota.IsInt64() {
		return 0, errors.New("video charge exceeds supported quota range")
	}
	return quota.Int64(), nil
}
