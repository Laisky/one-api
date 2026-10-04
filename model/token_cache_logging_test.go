package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/helper"
)

// TestClearTokenCacheFailureLogsNoCredentials captures actual JSON log output
// after a local Redis DEL failure, including logs from canceled request cleanup.
func TestClearTokenCacheFailureLogsNoCredentials(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "active"
		if canceled {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			originalEnabled, originalClient := common.IsRedisEnabled(), common.RDB
			common.SetRedisEnabled(true)
			common.RDB = client
			t.Cleanup(func() {
				common.SetRedisEnabled(originalEnabled)
				common.RDB = originalClient
			})

			const key = "sk-synthetic-cache-redaction-sentinel-493451"
			const cacheKey = "token:" + key
			const failure = "ERR synthetic Redis DEL failure"
			require.NoError(t, server.Set(cacheKey, "cached-authorization"))
			server.SetError(failure)
			hook := &tokenCacheLifecycleHook{}
			client.AddHook(hook)

			logPath := filepath.Join(t.TempDir(), "token-cache.log")
			captured, err := glog.New(
				glog.WithName("token-cache-redaction-test"),
				glog.WithLevel(glog.LevelDebug),
				glog.WithEncoding(glog.EncodingJSON),
				glog.WithOutputPaths([]string{logPath}),
				glog.WithErrorOutputPaths([]string{logPath}),
			)
			require.NoError(t, err)
			ctx := gmw.SetLogger(context.Background(), captured.With(zap.String("request_id", "synthetic-request")))
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if canceled {
				cancel()
			}

			clearTokenCache(ctx, key)
			require.NoError(t, captured.Sync())
			require.Equal(t, 1, hook.calls)
			require.NoError(t, hook.contextErr)
			require.True(t, hook.hasDeadline)
			require.True(t, server.Exists(cacheKey), "failed DEL must leave the entry present")

			output, err := os.ReadFile(logPath)
			require.NoError(t, err)
			var event map[string]any
			require.NoError(t, json.Unmarshal(output, &event))
			require.Equal(t, "failed to clear token cache, continuing", event["message"])
			require.Equal(t, "synthetic-request", event["request_id"])
			require.Equal(t, helper.MaskAPIKey(key), event["key"])
			require.Contains(t, event["error"], failure, "retain the Redis failure diagnostic")
			require.NotContains(t, string(output), key, "raw credentials must never reach any encoded log field")
		})
	}
}
