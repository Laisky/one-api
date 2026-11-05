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

// cacheFillFailureFixture seeds an authoritative SQLite database and a Redis
// server whose every SET fails, and returns a context carrying an observed
// logger. Reads still miss, so every cache-aside function falls back to SQL.
func cacheFillFailureFixture(t *testing.T) (context.Context, *observer.ObservedLogs) {
	t.Helper()
	ctx, logs, _ := cacheAsideFixture(t, true)
	return ctx, logs
}

// cacheAsideFixture seeds the authoritative SQLite database and a real Redis
// server. When failSet is true every Redis SET fails. It returns a context
// carrying an observed logger and the Redis server for seeding cache entries.
func cacheAsideFixture(t *testing.T, failSet bool) (context.Context, *observer.ObservedLogs, *miniredis.Miniredis) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Ability{}))
	require.NoError(t, db.Create(&User{Id: 123, Username: "test-cache", Password: "test-password", Group: "vip", Status: UserStatusEnabled, Quota: 5000}).Error)
	require.NoError(t, db.Create(&Token{Id: 456, UserId: 123, Name: "cache-token", Key: "cachefillfailurekey", Status: TokenStatusEnabled}).Error)
	require.NoError(t, db.Create(&Channel{Id: 789, Name: "cache-channel", Type: 1, Status: ChannelStatusEnabled, Group: "vip", Models: "model-a"}).Error)
	require.NoError(t, db.Create(&Ability{Group: "vip", Model: "model-a", ChannelId: 789, Enabled: true}).Error)
	// GetGroupModelsV2 memoizes per group in-process, so V2 uses its own group.
	require.NoError(t, db.Create(&Channel{Id: 790, Name: "cache-channel-v2", Type: 1, Status: ChannelStatusEnabled, Group: "cache-v2", Models: "model-b"}).Error)
	require.NoError(t, db.Create(&Ability{Group: "cache-v2", Model: "model-b", ChannelId: 790, Enabled: true}).Error)
	oldDB, oldPG := DB, common.UsingPostgreSQL.Load()
	DB = db
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() { DB = oldDB; common.UsingPostgreSQL.Store(oldPG) })
	redisServer := miniredis.RunT(t)
	redisServer.Server().SetPreHook(func(peer *server.Peer, cmd string, _ ...string) bool {
		if failSet && strings.EqualFold(cmd, "SET") {
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
	return gmw.SetLogger(context.Background(), lg), logs, redisServer
}

// TestCacheFillFailureReturnsAuthoritativeValue verifies the cache-aside
// contract for every Redis-backed read: after the authoritative database read
// succeeds, a failed cache fill is reported once as a stackless WARN inside the
// cache layer and never fails the caller, which receives the database value.
func TestCacheFillFailureReturnsAuthoritativeValue(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		read          func(context.Context) (any, error)
		want          any
	}{
		{"user group", "Redis set user group failed, continuing without cache",
			func(ctx context.Context) (any, error) { return CacheGetUserGroup(ctx, 123) }, "vip"},
		{"user enabled", "Redis set user enabled failed, continuing without cache",
			func(ctx context.Context) (any, error) { return CacheIsUserEnabled(ctx, 123) }, true},
		{"user quota", "Redis set user quota failed, continuing without cache",
			func(ctx context.Context) (any, error) { return CacheGetUserQuota(ctx, 123) }, int64(5000)},
		{"username", "Redis set username failed, continuing without cache",
			func(ctx context.Context) (any, error) { return CacheGetUsername(ctx, 123), nil }, "test-cache"},
		{"user object", "Redis set user object failed, continuing without cache",
			func(ctx context.Context) (any, error) {
				user, err := CacheGetUserById(ctx, 123)
				if user == nil {
					return nil, err
				}
				return user.Group, err
			}, "vip"},
		{"token", "Redis set token failed, continuing without cache",
			func(ctx context.Context) (any, error) {
				token, err := CacheGetTokenByKey(ctx, "cachefillfailurekey")
				if token == nil {
					return nil, err
				}
				return token.Id, err
			}, 456},
		{"group models", "Redis set group models failed, continuing without cache",
			func(ctx context.Context) (any, error) { return CacheGetGroupModels(ctx, "vip") }, []string{"model-a"}},
		{"group models v2", "Redis set group models failed, continuing without cache",
			func(ctx context.Context) (any, error) {
				abilities, err := CacheGetGroupModelsV2(ctx, "cache-v2")
				names := make([]string, 0, len(abilities))
				for _, ability := range abilities {
					names = append(names, ability.Model)
				}
				return names, err
			}, []string{"model-b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, logs := cacheFillFailureFixture(t)
			got, err := tc.read(ctx)
			require.NoError(t, err, "a cache fill failure must not fail an authoritative read")
			require.Equal(t, tc.want, got)
			warnings := logs.FilterLevelExact(zapcore.WarnLevel).All()
			require.Len(t, warnings, 1, "the fill failure is reported exactly once, in the cache layer")
			require.Equal(t, tc.message, warnings[0].Message)
			fields := warnings[0].ContextMap()
			require.Contains(t, fields["error"], "synthetic set failure")
			require.NotContains(t, fields, "errorVerbose", "a WARN must not export a stack")
			require.Empty(t, logs.FilterLevelExact(zapcore.ErrorLevel).All())
			for _, entry := range logs.All() {
				require.NotContains(t, entry.Message, "cachefillfailurekey", "the token credential must never be logged")
				for _, value := range entry.ContextMap() {
					if text, ok := value.(string); ok {
						require.NotContains(t, text, "cachefillfailurekey", "the token credential must never be logged")
					}
				}
			}
		})
	}
}

// TestCacheCorruptEntryFallsBackToDatabase verifies that an unreadable cache
// entry is treated as a miss: the caller receives the authoritative database
// value instead of an error or a fabricated zero balance, the corruption is
// reported once as a stackless WARN, and the entry is repaired.
func TestCacheCorruptEntryFallsBackToDatabase(t *testing.T) {
	for _, tc := range []struct {
		name, key, message string
		read               func(context.Context) (any, error)
		want               any
	}{
		{"user quota", "user_quota:123", "Redis cached user quota corrupted, falling back to database",
			func(ctx context.Context) (any, error) { return CacheGetUserQuota(ctx, 123) }, int64(5000)},
		{"token", "token:cachefillfailurekey", "Redis cached token corrupted, falling back to database",
			func(ctx context.Context) (any, error) {
				token, err := CacheGetTokenByKey(ctx, "cachefillfailurekey")
				if token == nil {
					return nil, err
				}
				return token.Id, err
			}, 456},
		{"user object", "user_obj:123", "Redis cached user object corrupted, falling back to database",
			func(ctx context.Context) (any, error) {
				user, err := CacheGetUserById(ctx, 123)
				if user == nil {
					return nil, err
				}
				return user.Group, err
			}, "vip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, logs, redisServer := cacheAsideFixture(t, false)
			require.NoError(t, redisServer.Set(tc.key, "{corrupt"))
			got, err := tc.read(ctx)
			require.NoError(t, err, "a corrupt cache entry must not fail an authoritative read")
			require.Equal(t, tc.want, got)
			warnings := logs.FilterLevelExact(zapcore.WarnLevel).All()
			require.Len(t, warnings, 1)
			require.Equal(t, tc.message, warnings[0].Message)
			require.NotContains(t, warnings[0].ContextMap(), "errorVerbose", "a WARN must not export a stack")
			repaired, err := redisServer.Get(tc.key)
			require.NoError(t, err)
			require.NotEqual(t, "{corrupt", repaired, "the database value replaces the corrupt entry")
			for _, entry := range logs.All() {
				for _, value := range entry.ContextMap() {
					if text, ok := value.(string); ok {
						require.NotContains(t, text, "cachefillfailurekey", "the token credential must never be logged")
					}
				}
			}
		})
	}
}
