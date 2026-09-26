package controller

import (
	"math"
	"math/big"
	"strconv"
	"strings"

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
	return quotaFromUsd(cost)
}

// videoQuotaFromTotalDecimal converts one exact provider USD quote directly to quota. Parameters are the original decimal quote and the channel group multiplier. It returns the rounded-up quota charge or a validation error.
func videoQuotaFromTotalDecimal(totalUsd string, group float64) (int64, error) {
	if math.IsNaN(group) || math.IsInf(group, 0) || group < 0 {
		return 0, errors.New("video prices and group ratio must be finite and nonnegative")
	}
	total, ok := new(big.Rat).SetString(strings.TrimSpace(totalUsd))
	if !ok || total.Sign() <= 0 {
		return 0, errors.New("invalid exact video quote")
	}
	groupRate, ok := new(big.Rat).SetString(strconv.FormatFloat(group, 'f', -1, 64))
	if !ok {
		return 0, errors.New("invalid decimal video group ratio")
	}
	cost := new(big.Rat).Mul(total, groupRate)
	cost.Mul(cost, big.NewRat(billingratio.QuotaPerUsd, 1))
	return quotaFromUsd(cost)
}

// quotaFromUsd rounds a rational USD cost up to the next quota unit. The parameter is a nonnegative rational USD amount already multiplied by the quota rate. It returns the int64 quota charge or an overflow error.
func quotaFromUsd(cost *big.Rat) (int64, error) {
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

// videoQuotaFromTotal converts a legacy floating-point whole-request quote directly to quota. Parameters are the quoted USD amount and the channel group multiplier. It returns the rounded-up quota charge or a validation error.
func videoQuotaFromTotal(totalUsd, group float64) (int64, error) {
	return videoQuota(totalUsd, 1, 1, 0, 0, group)
}
