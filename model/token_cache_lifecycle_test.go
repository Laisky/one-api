package model

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
)

type tokenCacheContextKey struct{}

type tokenCacheLifecycleHook struct {
	calls       int
	contextErr  error
	value       any
	deadline    time.Time
	hasDeadline bool
}

// BeforeProcess captures the actual Redis deletion context without changing it.
func (h *tokenCacheLifecycleHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "del" {
		h.calls++
		h.contextErr = ctx.Err()
		h.value = ctx.Value(tokenCacheContextKey{})
		h.deadline, h.hasDeadline = ctx.Deadline()
	}
	return ctx, nil
}

// AfterProcess leaves Redis command results unchanged.
func (h *tokenCacheLifecycleHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

// BeforeProcessPipeline leaves pipeline contexts unchanged.
func (h *tokenCacheLifecycleHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

// AfterProcessPipeline leaves pipeline results unchanged.
func (h *tokenCacheLifecycleHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

// TestClearTokenCacheLifecycle verifies cancellation-independent, bounded cache
// invalidation while preserving request metadata and supporting nil callers.
func TestClearTokenCacheLifecycle(t *testing.T) {
	for _, state := range []string{"active", "canceled", "expired", "nil"} {
		t.Run(state, func(t *testing.T) {
			server, err := miniredis.Run()
			require.NoError(t, err)
			t.Cleanup(server.Close)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			originalEnabled, originalClient := common.IsRedisEnabled(), common.RDB
			common.SetRedisEnabled(true)
			common.RDB = client
			t.Cleanup(func() {
				common.SetRedisEnabled(originalEnabled)
				common.RDB = originalClient
			})

			const key = "cache-lifecycle-test-credential"
			const cacheKey = "token:" + key
			require.NoError(t, server.Set(cacheKey, "cached-authorization"))
			require.True(t, server.Exists(cacheKey))
			hook := &tokenCacheLifecycleHook{}
			client.AddHook(hook)

			var ctx context.Context = context.WithValue(context.Background(), tokenCacheContextKey{}, "request-metadata")
			switch state {
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "expired":
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			case "nil":
				ctx = nil
			}

			started := time.Now()
			clearTokenCache(ctx, key)
			require.False(t, server.Exists(cacheKey), "stale authorization must actually be removed")
			require.Equal(t, 1, hook.calls)
			require.NoError(t, hook.contextErr, "Redis must not receive request cancellation")
			require.True(t, hook.hasDeadline, "detached cleanup must remain bounded")
			require.True(t, hook.deadline.After(started))
			require.LessOrEqual(t, hook.deadline.Sub(started), 5*time.Second+time.Second)
			if state != "nil" {
				require.Equal(t, "request-metadata", hook.value)
			}
		})
	}
}

// TestClearTokenCacheDisabled verifies that disabled Redis needs neither a
// client nor a caller context and cannot panic during token mutation cleanup.
func TestClearTokenCacheDisabled(t *testing.T) {
	originalEnabled, originalClient := common.IsRedisEnabled(), common.RDB
	common.SetRedisEnabled(false)
	common.RDB = nil
	t.Cleanup(func() {
		common.SetRedisEnabled(originalEnabled)
		common.RDB = originalClient
	})
	require.NotPanics(t, func() { clearTokenCache(nil, "disabled-cache-test") })
}
