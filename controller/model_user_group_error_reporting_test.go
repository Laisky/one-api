package controller

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

// TestAvailableModelsByTokenReportsGroupCacheFailure verifies a failed cache fill retains the existing response and a stackless diagnostic.
func TestAvailableModelsByTokenReportsGroupCacheFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, db.Create(&model.User{Id: 123, Username: "fixture", Password: "password", Group: "vip"}).Error)
	token := model.Token{Id: 456, UserId: 123, Name: "fixture", Status: model.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
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
	var encoded bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&encoded), zapcore.DebugLevel)
	lg, err := glog.NewWithName("test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/available_models", nil)
	gmw.SetLogger(c, lg)
	c.Set(ctxkey.Id, 123)
	c.Set(ctxkey.TokenId, 456)
	c.Set(ctxkey.AvailableModels, "model-a")
	GetAvailableModelsByToken(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Available []string `json:"available"`
			Enabled   bool     `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.False(t, payload.Success)
	require.Contains(t, payload.Message, "synthetic set failure")
	require.Nil(t, payload.Data.Available)
	require.False(t, payload.Data.Enabled)
	var event map[string]any
	require.NoError(t, json.NewDecoder(&encoded).Decode(&event))
	require.Equal(t, "warn", event["level"])
	require.Equal(t, "failed to load user group for available models", event["msg"])
	require.Contains(t, event["error"], "synthetic set failure")
	require.NotContains(t, event, "errorVerbose")
	require.NotContains(t, event, "stacktrace")
}
