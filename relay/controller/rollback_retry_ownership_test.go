package controller

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestPR421RollbackRetryOwnership forces the retry reset to race a pending or
// completed explicit media refund. Real ledger records must return exactly to
// the original balance, while a conservatively retained hold still gets refunded.
func TestPR421RollbackRetryOwnership(t *testing.T) {
	for _, task := range []string{"audio", "video"} {
		for _, state := range []string{"pending", "completed", "conservative_retained"} {
			t.Run(task+"/"+state, func(t *testing.T) {
				const initial, hold = int64(50000), int64(1234)
				start := billingAccountingSetup(t, initial)
				// The shared fixture starts with an unlimited token. Use an explicit
				// limited balance so both user and token movements are exercised.
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).
					Updates(map[string]any{"unlimited_quota": false, "remain_quota": start, "used_quota": 0}).Error)
				cache := miniredis.RunT(t)
				redisClient := redis.NewClient(&redis.Options{Addr: cache.Addr()})
				oldRedis, wasRedisEnabled := common.RDB, common.IsRedisEnabled()
				common.RDB = redisClient
				common.SetRedisEnabled(true)
				t.Cleanup(func() {
					drainBilling(t)
					common.RDB = oldRedis
					common.SetRedisEnabled(wasRedisEnabled)
					require.NoError(t, redisClient.Close())
				})
				// Let both legacy refund writers complete deterministically, rather
				// than having SQLite locks accidentally hide a duplicate refund.
				sqlDB, err := model.DB.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)
				preConsume(t, hold)
				require.Equal(t, start-hold, reloadUserQuota(t))
				cacheKey := fmt.Sprintf("user_quota:%d", fallbackUserID)
				require.NoError(t, redisClient.Set(context.Background(), cacheKey, start-hold, 0).Err())
				c, _ := newRollbackContext(t, t.Name(), false)
				markPreConsumed(c, hold)
				c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
				markBillingReconciled(c)
				gate := make(chan struct{})
				closed := false
				t.Cleanup(func() {
					if !closed {
						close(gate)
					}
					drainBilling(t)
				})
				if state != "conservative_retained" {
					goRollbackPreConsumed(c, task+"RetryOwnership", fallbackTokenID, hold, gate, nil)
				}
				if state == "completed" {
					close(gate)
					closed = true
					drainBilling(t)
					require.Equal(t, start, reloadUserQuota(t))
				}
				ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
				if !closed {
					close(gate)
					closed = true
				}
				drainBilling(t)
				require.Equal(t, start, reloadUserQuota(t), "a retry must not issue a second owned refund")
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, start, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				cached, err := redisClient.Get(context.Background(), cacheKey).Result()
				require.NoError(t, err)
				require.Equal(t, strconv.FormatInt(start, 10), cached, "the refunded balance must also reach Redis")
				require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
				require.False(t, c.GetBool(ctxkey.BillingReconciled))
			})
		}
	}
}
