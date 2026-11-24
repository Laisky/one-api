package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestGetAvailableModelsByTokenLogsGroupLookupErrorStructurally verifies the
// user-group lookup failure is logged once with a structured zap error field
// and the user id, instead of a flattened error string.
func TestGetAvailableModelsByTokenLogsGroupLookupErrorStructurally(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tokens.db")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	// The users table is deliberately absent so the group lookup fails in the DB.
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	require.NoError(t, db.Create(&model.Token{Id: 456, UserId: 123, Name: "fixture", Status: model.TokenStatusEnabled}).Error)
	oldDB, oldPG := model.DB, common.UsingPostgreSQL.Load()
	model.DB = db
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() { model.DB = oldDB; common.UsingPostgreSQL.Store(oldPG) })
	oldEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldEnabled) })

	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/available_models", nil)
	gmw.SetLogger(c, lg)
	c.Set(ctxkey.Id, 123)
	c.Set(ctxkey.TokenId, 456)
	c.Set(ctxkey.AvailableModels, "model-a")

	GetAvailableModelsByToken(c)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	entries := logs.FilterMessage("failed to load user group for available models").All()
	require.Len(t, entries, 1)
	var errorField, userField *zapcore.Field
	for i := range entries[0].Context {
		field := &entries[0].Context[i]
		switch field.Key {
		case "error":
			errorField = field
		case "user_id":
			userField = field
		}
	}
	require.NotNil(t, userField)
	require.Equal(t, int64(123), userField.Integer)
	require.NotNil(t, errorField)
	require.Equal(t, zapcore.ErrorType, errorField.Type, "the error must be logged with zap.Error, not as a string")
	loggedErr, ok := errorField.Interface.(error)
	require.True(t, ok)
	require.Contains(t, loggedErr.Error(), "users")
}
