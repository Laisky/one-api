package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
)

// TestCacheGetTokenByKeySetFailureLogsNoCredentials captures actual JSON output
// after a Redis SET-only failure on the production token cache-fill path.
func TestCacheGetTokenByKeySetFailureLogsNoCredentials(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tokens.db")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Token{}))
	originalDB, originalPostgreSQL := DB, common.UsingPostgreSQL.Load()
	DB = db
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() {
		DB = originalDB
		common.UsingPostgreSQL.Store(originalPostgreSQL)
	})

	const key = "sk-synthetic-cache-set-redaction-sentinel-516"
	const tokenUUID = "018f0000-0000-7000-8000-000000000516"
	const tokenName = "synthetic-cache-set-token"
	require.NoError(t, db.Exec("INSERT INTO tokens (id, uuid, user_id, key, name, status) VALUES (?, ?, ?, ?, ?, ?)",
		516, tokenUUID, 42, key, tokenName, TokenStatusEnabled).Error)

	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	originalEnabled, originalClient := common.IsRedisEnabled(), common.RDB
	common.SetRedisEnabled(true)
	common.RDB = client
	t.Cleanup(func() {
		common.SetRedisEnabled(originalEnabled)
		common.RDB = originalClient
	})
	const failure = "ERR synthetic Redis SET failure"
	var getCalls, setCalls atomic.Int32
	redisServer.Server().SetPreHook(func(peer *server.Peer, cmd string, _ ...string) bool {
		switch strings.ToUpper(cmd) {
		case "GET":
			getCalls.Add(1)
		case "SET":
			setCalls.Add(1)
			peer.WriteError(failure)
			return true
		}
		return false
	})

	logPath := filepath.Join(t.TempDir(), "token-cache-set.log")
	captured, err := glog.New(
		glog.WithName("token-cache-set-redaction-test"),
		glog.WithLevel(glog.LevelDebug),
		glog.WithEncoding(glog.EncodingJSON),
		glog.WithOutputPaths([]string{logPath}),
		glog.WithErrorOutputPaths([]string{logPath}),
	)
	require.NoError(t, err)
	ctx := gmw.SetLogger(context.Background(), captured.With(zap.String("request_id", "synthetic-set-request")))

	token, err := CacheGetTokenByKey(ctx, key)
	require.NoError(t, err, "cache write failure must not reject the database token")
	require.Equal(t, 516, token.Id)
	require.Equal(t, key, token.Key)
	require.Equal(t, int32(1), getCalls.Load(), "normal Redis cache miss must reach SQL fallback")
	require.Equal(t, int32(1), setCalls.Load(), "real Redis SET must reach the failing server")
	require.False(t, redisServer.Exists("token:"+key))
	require.NoError(t, captured.Sync())

	output, err := os.ReadFile(logPath)
	require.NoError(t, err)
	var event map[string]any
	require.NoError(t, json.Unmarshal(output, &event))
	require.Equal(t, "Redis set token failed, continuing without cache", event["message"])
	require.Equal(t, "synthetic-set-request", event["request_id"])
	require.EqualValues(t, 516, event["token_id"])
	require.Equal(t, tokenUUID, event["token_uuid"])
	require.Equal(t, tokenName, event["token_name"])
	require.Contains(t, event["error"], failure, "retain the Redis failure diagnostic")
	require.NotContains(t, string(output), key, "credentials must never reach any encoded log field")
}
