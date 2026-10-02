package model

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// AsyncCostMultiplier snapshots quota/USD and the administrator's group ratio
// as an exact bounded rational. Later configuration changes cannot reprice a job.
func AsyncCostMultiplier(quotaPerUSD, group float64) (string, error) {
	if math.IsNaN(quotaPerUSD) || math.IsInf(quotaPerUSD, 0) || quotaPerUSD <= 0 || math.IsNaN(group) || math.IsInf(group, 0) || group < 0 {
		return "", errors.New("invalid async price multiplier")
	}
	left, _ := new(big.Rat).SetString(strconv.FormatFloat(quotaPerUSD, 'g', -1, 64))
	right, _ := new(big.Rat).SetString(strconv.FormatFloat(group, 'g', -1, 64))
	value := left.Mul(left, right).RatString()
	if len(value) > 128 {
		return "", errors.New("async price multiplier exceeds limit")
	}
	return value, nil
}

// AsyncUpstreamCostQuota rounds an actual USD charge UP using the immutable
// pricing snapshot. Invalid/overflowing amounts cannot silently become zero.
func AsyncUpstreamCostQuota(multiplier, cost string) (int64, error) {
	if len(multiplier) == 0 || len(multiplier) > 128 || len(cost) == 0 || len(cost) > 128 {
		return 0, errors.New("invalid async charged amount")
	}
	// Bound exponents BEFORE big.Rat parsing; underflow may parse as float zero
	// without an error and must not authorize an unbounded big-integer allocation.
	if !json.Valid([]byte(cost)) {
		return 0, errors.New("invalid async charged USD number")
	}
	if at := strings.IndexAny(cost, "eE"); at >= 0 {
		exponent, err := strconv.Atoi(cost[at+1:])
		if err != nil || exponent < -308 || exponent > 308 {
			return 0, errors.New("async charged USD exponent exceeds limit")
		}
	}
	numeric, err := strconv.ParseFloat(cost, 64)
	if err != nil || math.IsNaN(numeric) || math.IsInf(numeric, 0) || numeric < 0 {
		return 0, errors.New("invalid async charged USD")
	}
	amount, ok := new(big.Rat).SetString(cost)
	if !ok || amount.Sign() < 0 {
		return 0, errors.New("invalid async charged USD decimal")
	}
	// Snapshots are produced by RatString: bounded decimal digits and at most
	// one slash. Reject exponent syntax before allocating big integers.
	for _, ch := range multiplier {
		if (ch < '0' || ch > '9') && ch != '/' {
			return 0, errors.New("invalid async price snapshot syntax")
		}
	}
	factor, ok := new(big.Rat).SetString(multiplier)
	if !ok || factor.Sign() < 0 {
		return 0, errors.New("invalid async price snapshot")
	}
	total := amount.Mul(amount, factor)
	rounded, remainder := new(big.Int), new(big.Int)
	rounded.QuoRem(total.Num(), total.Denom(), remainder)
	if remainder.Sign() > 0 {
		rounded.Add(rounded, big.NewInt(1))
	}
	if !rounded.IsInt64() || rounded.Int64() > math.MaxInt64/2 {
		return 0, errors.New("async charged quota exceeds limit")
	}
	return rounded.Int64(), nil
}

// collectAsyncTaskCost monotonically raises a dynamic reservation when an
// authoritative provider cost exceeds its estimate. Debt is explicit: a spent
// wallet cannot make the gateway forgive already-incurred cost. Fixed admin
// tariffs deliberately keep their original customer price. The caller owns tx.
func collectAsyncTaskCost(tx *gorm.DB, task *AsyncTask, cost string) (bool, error) {
	if cost == "" || task.CostQuotaPerUSD == "" {
		return false, nil
	}
	quota, err := AsyncUpstreamCostQuota(task.CostQuotaPerUSD, cost)
	if err != nil {
		return false, errors.Wrap(err, "calculate actual async task cost")
	}
	quota = max(quota, task.Quota)
	delta := quota - task.Quota
	if delta > 0 {
		result := tx.Model(&User{}).Where("id = ? AND uuid = ? AND quota >= ?", task.UserID, task.UserUUID, int64(math.MinInt64)+delta).Update("quota", gorm.Expr("quota - ?", delta))
		if result.Error != nil {
			return false, errors.Wrap(result.Error, "collect additional async owner cost")
		}
		if result.RowsAffected != 1 {
			return false, errors.New("async owner cost cannot be collected")
		}
		if !task.TokenUnlimited {
			result = tx.Model(&Token{}).Where("id = ? AND user_id = ? AND uuid = ? AND remain_quota >= ? AND used_quota <= ?", task.TokenID, task.UserID, task.TokenUUID, int64(math.MinInt64)+delta, int64(math.MaxInt64)-delta).
				Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", delta), "used_quota": gorm.Expr("used_quota + ?", delta)})
			if result.Error != nil {
				return false, errors.Wrap(result.Error, "collect additional async token cost")
			}
			if result.RowsAffected != 1 {
				var count int64
				if err := tx.Model(&Token{}).Where("id = ? AND user_id = ? AND uuid = ?", task.TokenID, task.UserID, task.TokenUUID).Count(&count).Error; err != nil {
					return false, errors.Wrap(err, "check async cost token")
				}
				if count != 0 {
					return false, errors.New("async token cost overflow")
				}
			}
		}
	}
	// Keep the largest observed charge as audit evidence for a conservative
	// debit. A later stale/discounted observation must not erase its basis.
	if task.UpstreamCostUSD != "" {
		previous, previousOK := new(big.Rat).SetString(task.UpstreamCostUSD)
		current, _ := new(big.Rat).SetString(cost)
		if previousOK && previous.Cmp(current) > 0 {
			cost = task.UpstreamCostUSD
		}
	}
	if err := tx.Model(&AsyncTask{}).Where("id = ?", task.ID).Updates(map[string]any{"quota": quota, "upstream_cost_usd": cost}).Error; err != nil {
		return false, errors.Wrap(err, "persist actual async task cost")
	}
	task.Quota, task.UpstreamCostUSD = quota, cost
	return delta > 0, nil
}
