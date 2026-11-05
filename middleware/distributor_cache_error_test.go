package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestDistributorGroupFailureReporting exercises cache-fill and database failure
// while retaining the selected group and the existing subsequent routing outcome.
// A failed cache fill is degradation inside the cache layer: the distributor
// receives the authoritative group without an error and reports nothing, while
// a failed database read is still reported once by the distributor.
func TestDistributorGroupFailureReporting(t *testing.T) {
	for _, tc := range []struct {
		name, group, errorText string
		databaseFailure        bool
	}{{"redis set failure", "vip", "synthetic set failure", false}, {"database failure", "", "no such table", true}} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{Logger: gormlogger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			if !tc.databaseFailure {
				require.NoError(t, db.AutoMigrate(&model.User{}))
				require.NoError(t, db.Create(&model.User{Id: 123, Username: "test-cache", Password: "test-password", Group: "vip"}).Error)
			}
			oldDB, oldPG := model.DB, common.UsingPostgreSQL.Load()
			model.DB = db
			common.UsingPostgreSQL.Store(false)
			t.Cleanup(func() { model.DB = oldDB; common.UsingPostgreSQL.Store(oldPG) })
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
			var encoded bytes.Buffer
			jsonCore := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&encoded), zapcore.DebugLevel)
			lg, err := glog.NewWithName("distributor-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return zapcore.NewTee(core, jsonCore) }))
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set(ctxkey.Id, 123)
			c.Set(ctxkey.SpecificChannelId, -1)
			gmw.SetLogger(c, lg)
			Distribute()(c)
			require.Equal(t, tc.group, c.GetString(ctxkey.Group))
			require.True(t, c.IsAborted())
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			cacheWarnings := logs.FilterMessage("Redis set user group failed, continuing without cache").All()
			entries := logs.FilterMessage("failed to load user group, continuing with available group").All()
			if !tc.databaseFailure {
				require.Empty(t, entries, "a cache fill failure is not a group lookup failure")
				require.Len(t, cacheWarnings, 1, "the cache layer reports its own degradation once")
				require.Contains(t, cacheWarnings[0].ContextMap()["error"], tc.errorText)
				require.NotContains(t, cacheWarnings[0].ContextMap(), "errorVerbose")
				return
			}
			require.Empty(t, cacheWarnings, "no cache fill is attempted without an authoritative value")
			require.Len(t, entries, 1)
			require.EqualValues(t, 123, entries[0].ContextMap()["user_id"])
			require.Contains(t, entries[0].ContextMap()["error"], "user")
			var warning map[string]any
			decoder := json.NewDecoder(&encoded)
			for decoder.More() {
				var event map[string]any
				require.NoError(t, decoder.Decode(&event))
				if event["msg"] == "failed to load user group, continuing with available group" {
					warning = event
				}
			}
			require.NotNil(t, warning)
			require.Equal(t, "warn", warning["level"])
			require.EqualValues(t, 123, warning["user_id"])
			require.Contains(t, warning["error"], tc.errorText)
			require.NotContains(t, warning, "errorVerbose", "warning diagnostics must not export a stack")
			require.NotContains(t, warning, "stacktrace")
		})
	}
}
