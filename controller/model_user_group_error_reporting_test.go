package controller

import (
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

// TestModelDiscoverySurvivesCacheFillFailure verifies that model discovery
// serves the authoritative database answer when Redis cannot store the cache
// entry. A failed cache fill must not turn a valid request into an HTTP 400 or
// expose Redis diagnostics to the client; it is reported once inside the cache
// layer as a stackless WARN.
func TestModelDiscoverySurvivesCacheFillFailure(t *testing.T) {
	for _, tc := range []struct {
		name, path, group string
		handler           gin.HandlerFunc
	}{
		{name: "available models by token", path: "/api/available_models", group: "cache-fill-token", handler: GetAvailableModelsByToken},
		{name: "list models", path: "/v1/models", group: "cache-fill-list", handler: ListModels},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{Logger: gormlogger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}))
			require.NoError(t, db.Create(&model.User{Id: 123, Username: "fixture", Password: "password", Group: tc.group, Status: model.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 456, UserId: 123, Name: "fixture", Status: model.TokenStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Channel{Id: 789, Name: "fixture-channel", Type: 1, Status: model.ChannelStatusEnabled, Group: tc.group, Models: "model-a"}).Error)
			require.NoError(t, db.Create(&model.Ability{Group: tc.group, Model: "model-a", ChannelId: 789, Enabled: true}).Error)
			oldDB, oldPG := model.DB, common.UsingPostgreSQL.Load()
			model.DB = db
			common.UsingPostgreSQL.Store(false)
			t.Cleanup(func() { model.DB = oldDB; common.UsingPostgreSQL.Store(oldPG) })
			serverInstance := miniredis.RunT(t)
			serverInstance.Server().SetPreHook(func(peer *server.Peer, cmd string, _ ...string) bool {
				if strings.EqualFold(cmd, "SET") {
					peer.WriteError("ERR synthetic set failure")
					return true
				}
				return false
			})
			client := redis.NewClient(&redis.Options{Addr: serverInstance.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			oldClient, oldEnabled := common.RDB, common.IsRedisEnabled()
			common.RDB = client
			common.SetRedisEnabled(true)
			t.Cleanup(func() { common.RDB = oldClient; common.SetRedisEnabled(oldEnabled) })
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewWithName("test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			gmw.SetLogger(c, lg)
			c.Set(ctxkey.Id, 123)
			c.Set(ctxkey.TokenId, 456)
			c.Set(ctxkey.AvailableModels, "model-a")

			tc.handler(c)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "synthetic set failure", "Redis diagnostics must not reach the client")
			require.Contains(t, rec.Body.String(), "model-a")
			if tc.path == "/api/available_models" {
				var payload struct {
					Success bool `json:"success"`
					Data    struct {
						Available []string `json:"available"`
						Enabled   bool     `json:"enabled"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
				require.True(t, payload.Success)
				require.Equal(t, []string{"model-a"}, payload.Data.Available)
				require.True(t, payload.Data.Enabled)
			}
			require.Empty(t, logs.FilterMessage("failed to load user group for available models").All())
			require.Len(t, logs.FilterMessage("Redis set user group failed, continuing without cache").All(), 1)
			for _, entry := range logs.FilterLevelExact(zapcore.WarnLevel).All() {
				require.Contains(t, entry.Message, "continuing without cache", "only cache-layer degradation is reported")
				require.NotContains(t, entry.ContextMap(), "errorVerbose", "a WARN must not export a stack")
			}
			require.Empty(t, logs.FilterLevelExact(zapcore.ErrorLevel).All())
		})
	}
}
