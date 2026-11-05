package model

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
)

// TestCacheUserResultsReturnSetFailures verifies fallback values and Redis SET
// errors reach callers without being logged inside the cache functions.
func TestCacheUserResultsReturnSetFailures(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Create(&User{Id: 123, Username: "test-cache", Password: "test-password", Group: "vip", Status: UserStatusEnabled}).Error)
	oldDB, oldPG := DB, common.UsingPostgreSQL.Load()
	DB = db
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() { DB = oldDB; common.UsingPostgreSQL.Store(oldPG) })
	redisServer := miniredis.RunT(t)
	redisServer.Server().SetPreHook(func(peer *server.Peer, cmd string, _ ...string) bool {
		if strings.EqualFold(cmd, "SET") {
			peer.WriteError("ERR synthetic set failure")
			return true
		}
		return false
	})
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	oldClient, oldEnabled := common.RDB, common.IsRedisEnabled()
	common.RDB = client
	common.SetRedisEnabled(true)
	t.Cleanup(func() { common.RDB = oldClient; common.SetRedisEnabled(oldEnabled) })
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("cache-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	ctx := gmw.SetLogger(context.Background(), lg)
	group, err := CacheGetUserGroup(ctx, 123)
	require.Equal(t, "vip", group)
	require.ErrorContains(t, err, "synthetic set failure")
	enabled, err := CacheIsUserEnabled(ctx, 123)
	require.True(t, enabled)
	require.ErrorContains(t, err, "synthetic set failure")
	require.Empty(t, logs.FilterLevelExact(zapcore.WarnLevel).All())
}
