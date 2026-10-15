package common

import (
	"encoding/json"
	"testing"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// TestRedisConfigurationParseLoggingRedactsURI exercises the real parse-failure
// callers with a malformed fake-secret URI; no Redis network operation is reached.
func TestRedisConfigurationParseLoggingRedactsURI(t *testing.T) {
	previousLogger := logger.Logger
	previousURI, previousMaster, previousFrequency := config.RedisConnString, config.RedisMasterName, config.SyncFrequency
	previousEnabled, previousRDB := IsRedisEnabled(), RDB
	t.Cleanup(func() {
		logger.Logger = previousLogger
		config.RedisConnString, config.RedisMasterName, config.SyncFrequency = previousURI, previousMaster, previousFrequency
		SetRedisEnabled(previousEnabled)
		RDB = previousRDB
	})
	for _, target := range []string{"init", "options"} {
		t.Run(target, func(t *testing.T) {
			core, entries := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewConsoleWithName("redis-test", glog.LevelDebug,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
				zap.WithFatalHook(zapcore.WriteThenPanic))
			require.NoError(t, err)
			logger.Logger = lg
			const password = "fake-redis-password"
			config.RedisConnString = "redis://operator:" + password + "@redis.invalid:invalid"
			config.RedisMasterName = ""
			config.SyncFrequency = 120
			require.Panics(t, func() {
				if target == "init" {
					_ = InitRedisClient()
				} else {
					_ = ParseRedisOption()
				}
			})
			failures := entries.FilterMessage("failed to parse Redis connection string").All()
			require.Len(t, failures, 1)
			fields := failures[0].ContextMap()
			encoded, marshalErr := json.Marshal(fields)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(encoded), password)
			require.Equal(t, logger.Redact(config.RedisConnString), fields["connection_string"])
			require.NotEmpty(t, fields["error"])
			require.NotEmpty(t, fields["error_type"])
		})
	}
}
