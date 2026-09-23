package model

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// TestQuotaRefundCacheRefreshDeadline verifies the post-credit Redis refresh
// cannot perform an unbounded database read after its worker is cancelled.
// The stale cache must remain untouched rather than receiving a partial result.
func TestQuotaRefundCacheRefreshDeadline(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis := common.RDB
	common.RDB = client
	common.SetRedisEnabled(true)
	t.Cleanup(func() {
		common.SetRedisEnabled(false)
		common.RDB = oldRedis
		require.NoError(t, client.Close())
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, common.RedisSet(ctx, "user_quota:101", "17", time.Minute))
	var observed context.Context
	const hook = "refund:observe_cache_read_context"
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			observed = tx.Statement.Context
		}
	}))
	cancel()
	require.ErrorIs(t, CacheUpdateUserQuota(ctx, intent.UserID), context.Canceled)
	require.Same(t, ctx, observed, "the database read must honor the recovery deadline")
	cached, err := server.Get("user_quota:101")
	require.NoError(t, err)
	require.Equal(t, "17", cached)
}
