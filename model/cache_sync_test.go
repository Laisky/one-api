package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common"
)

// TestCacheUpdateUserQuotaRefreshesFromDatabase verifies that cache refresh uses
// the authoritative database quota instead of reusing potentially stale cached values.
func TestCacheUpdateUserQuotaRefreshesFromDatabase(t *testing.T) {
	setupTestDatabase(t)

	redisServer, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(redisServer.Close)

	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, redisClient.Close())
	})

	originalRedisEnabled := common.IsRedisEnabled()
	originalRedisClient := common.RDB
	common.SetRedisEnabled(true)
	common.RDB = redisClient
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
		common.RDB = originalRedisClient
	})

	user := &User{
		Username: fmt.Sprintf("test-cache-sync-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    4321,
	}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE id = ?", user.Id)
	})

	ctx := context.Background()
	cacheKey := fmt.Sprintf("user_quota:%d", user.Id)
	require.NoError(t, common.RedisSet(ctx, cacheKey, "7", time.Minute))

	require.NoError(t, CacheUpdateUserQuota(ctx, user.Id))

	quota, err := common.RedisGet(ctx, cacheKey)
	require.NoError(t, err)
	require.Equal(t, "4321", quota)
}
