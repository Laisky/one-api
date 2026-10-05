package common

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

// TestConsumeTotpCodeMemory verifies the in-memory replay cache accepts a code
// once per user, rejects empty codes, and accepts an entry again only after it
// expires and is pruned.
func TestConsumeTotpCodeMemory(t *testing.T) {
	oldRedis := IsRedisEnabled()
	SetRedisEnabled(false)
	t.Cleanup(func() { SetRedisEnabled(oldRedis) })
	ctx := context.Background()
	uid := int(time.Now().UTC().UnixNano()%1_000_000_000) + 4_000_000

	first, err := ConsumeTotpCode(ctx, uid, "123456")
	require.NoError(t, err)
	require.True(t, first)
	again, err := ConsumeTotpCode(ctx, uid, "123456")
	require.NoError(t, err)
	require.False(t, again, "a consumed code was accepted twice")
	require.True(t, IsTotpCodeUsed(ctx, uid, "123456"))
	other, err := ConsumeTotpCode(ctx, uid+1, "123456")
	require.NoError(t, err)
	require.True(t, other, "codes are scoped per user")
	empty, err := ConsumeTotpCode(ctx, uid, "")
	require.NoError(t, err)
	require.False(t, empty)

	key := totpCodeKey(uid, "123456")
	totpMemoryMu.Lock()
	totpMemoryCache[key] = time.Now().UTC().Add(-time.Second)
	totpMemoryMu.Unlock()
	CleanupExpiredTotpCodes()
	totpMemoryMu.Lock()
	_, present := totpMemoryCache[key]
	totpMemoryMu.Unlock()
	require.False(t, present, "expired entry was not pruned")
	reused, err := ConsumeTotpCode(ctx, uid, "123456")
	require.NoError(t, err)
	require.True(t, reused, "an expired entry must not block a later window")
}

// TestConsumeTotpCodeRedis verifies the Redis path is a single SET NX with the
// replay TTL and reports a Redis failure to the caller.
func TestConsumeTotpCodeRedis(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRDB, oldRedis := RDB, IsRedisEnabled()
	RDB = client
	SetRedisEnabled(true)
	t.Cleanup(func() {
		RDB = oldRDB
		SetRedisEnabled(oldRedis)
		require.NoError(t, client.Close())
	})
	ctx := context.Background()

	first, err := ConsumeTotpCode(ctx, 7, "654321")
	require.NoError(t, err)
	require.True(t, first)
	again, err := ConsumeTotpCode(ctx, 7, "654321")
	require.NoError(t, err)
	require.False(t, again)
	require.Equal(t, TotpCodeCacheDuration, server.TTL(totpCodeKey(7, "654321")))
	require.True(t, IsTotpCodeUsed(ctx, 7, "654321"))

	server.Close()
	failed, err := ConsumeTotpCode(ctx, 8, "654321")
	require.Error(t, err)
	require.False(t, failed)
}
