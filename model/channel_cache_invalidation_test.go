package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/logger"
)

// setupChannelCacheInvalidationTest installs isolated database and Redis
// backends for a cache invalidation behavior test and restores process globals
// when the test finishes. It returns a live context for arranging Redis state.
func setupChannelCacheInvalidationTest(t *testing.T) context.Context {
	t.Helper()

	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	t.Cleanup(func() { DB = originalDB })

	redisServer, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(redisServer.Close)

	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	originalRedisEnabled := common.IsRedisEnabled()
	originalRedisClient := common.RDB
	common.SetRedisEnabled(true)
	common.RDB = redisClient
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
		common.RDB = originalRedisClient
	})

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() { common.UsingSQLite.Store(originalUsingSQLite) })

	return context.Background()
}

// TestChannelModelCacheInvalidationContextPreservesLogger verifies that the
// detached cache context ignores request cancellation, retains the correlated
// logger, and has the required operational deadline.
func TestChannelModelCacheInvalidationContextPreservesLogger(t *testing.T) {
	requestLogger := logger.Logger.Named("cache-invalidation-correlation")
	requestCtx := gmw.SetLogger(context.Background(), requestLogger)
	canceledCtx, cancelRequest := context.WithCancel(requestCtx)
	cancelRequest()

	invalidationCtx, cancelInvalidation := newChannelModelCacheInvalidationContext(canceledCtx)
	defer cancelInvalidation()

	require.NoError(t, invalidationCtx.Err(), "request cancellation must not cancel post-commit work")
	require.Same(t, requestLogger, gmw.GetLogger(invalidationCtx),
		"request-scoped logger correlation must survive cancellation detachment")
	deadline, ok := invalidationCtx.Deadline()
	require.True(t, ok, "post-commit cache invalidation must be bounded")
	require.WithinDuration(t, time.Now().Add(channelModelCacheInvalidationTimeout), deadline, time.Second)
}

// TestChannelUpdateWithCanceledContextInvalidatesRedisCache verifies that a
// request canceled after a channel transaction commits cannot leave stale
// group-model entries in Redis. It also proves the database update completed.
func TestChannelUpdateWithCanceledContextInvalidatesRedisCache(t *testing.T) {
	redisCtx := setupChannelCacheInvalidationTest(t)
	group := fmt.Sprintf("canceled-update-%d", time.Now().UnixNano())
	channel := &Channel{
		Name:   "canceled-update",
		Type:   1,
		Status: ChannelStatusEnabled,
		Models: "old-model",
		Group:  group,
	}
	require.NoError(t, channel.Insert())

	legacyKey := fmt.Sprintf("group_models:%s", group)
	v2Key := fmt.Sprintf("group_models_v2:%s", group)
	require.NoError(t, common.RedisSet(redisCtx, legacyKey, "old-model", time.Minute))
	require.NoError(t, common.RedisSet(redisCtx, v2Key, `[{"model":"old-model"}]`, time.Minute))

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	channel.Models = "new-model"
	require.NoError(t, channel.UpdateWithContext(canceledCtx))

	var persisted Channel
	require.NoError(t, DB.First(&persisted, channel.Id).Error)
	require.Equal(t, "new-model", persisted.Models,
		"the transaction must commit before post-commit cache invalidation")

	legacyExists, err := common.RDB.Exists(redisCtx, legacyKey).Result()
	require.NoError(t, err)
	require.Zero(t, legacyExists, "the legacy group-model cache must not survive a committed update")
	v2Exists, err := common.RDB.Exists(redisCtx, v2Key).Result()
	require.NoError(t, err)
	require.Zero(t, v2Exists, "the v2 group-model cache must not survive a committed update")
}

// TestChannelStatusUpdateWithCanceledContextInvalidatesRedisCache verifies
// that a committed status change clears Redis routing entries even when its
// originating request is already canceled.
func TestChannelStatusUpdateWithCanceledContextInvalidatesRedisCache(t *testing.T) {
	redisCtx := setupChannelCacheInvalidationTest(t)
	group := fmt.Sprintf("canceled-status-%d", time.Now().UnixNano())
	channel := &Channel{
		Name:   "canceled-status",
		Type:   1,
		Status: ChannelStatusEnabled,
		Models: "status-model",
		Group:  group,
	}
	require.NoError(t, channel.Insert())

	legacyKey := fmt.Sprintf("group_models:%s", group)
	v2Key := fmt.Sprintf("group_models_v2:%s", group)
	require.NoError(t, common.RedisSet(redisCtx, legacyKey, "status-model", time.Minute))
	require.NoError(t, common.RedisSet(redisCtx, v2Key, `[{"model":"status-model"}]`, time.Minute))

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	UpdateChannelStatusByIdWithContext(canceledCtx, channel.Id, ChannelStatusManuallyDisabled)

	var persisted Channel
	require.NoError(t, DB.First(&persisted, channel.Id).Error)
	require.Equal(t, ChannelStatusManuallyDisabled, persisted.Status,
		"the status write must commit before post-commit cache invalidation")

	legacyExists, err := common.RDB.Exists(redisCtx, legacyKey).Result()
	require.NoError(t, err)
	require.Zero(t, legacyExists, "the legacy group-model cache must not survive a committed status change")
	v2Exists, err := common.RDB.Exists(redisCtx, v2Key).Result()
	require.NoError(t, err)
	require.Zero(t, v2Exists, "the v2 group-model cache must not survive a committed status change")
}
