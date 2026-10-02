package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/identity"
	dbmodel "github.com/Laisky/one-api/model"
)

const (
	identityTestUserUUID    = "018f0000-0000-7000-8000-0000000000a1"
	identityTestTokenUUID   = "018f0000-0000-7000-8000-0000000000a2"
	identityTestChannelUUID = "018f0000-0000-7000-8000-0000000000a3"
)

// setupIdentityBindingTestDB creates an in-memory database with one enabled user,
// token and channel, and points the model package at it for the test's duration.
//
// Parameters:
//   - t: test handle used for assertions and cleanup.
func setupIdentityBindingTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&dbmodel.User{}, &dbmodel.Token{}, &dbmodel.Channel{}))

	userUUID := identityTestUserUUID
	require.NoError(t, db.Create(&dbmodel.User{
		Id:       175,
		UUID:     userUUID,
		Username: "alice",
		Password: "password-hash",
		Role:     dbmodel.RoleCommonUser,
		Status:   dbmodel.UserStatusEnabled,
		Group:    "default",
		Quota:    1000,
	}).Error)
	require.NoError(t, db.Create(&dbmodel.Token{
		Id:             257,
		UUID:           identityTestTokenUUID,
		UserId:         175,
		UserUUID:       &userUUID,
		Key:            "alicetoken",
		Status:         dbmodel.TokenStatusEnabled,
		Name:           "laptop-cli",
		ExpiredTime:    -1,
		RemainQuota:    1000,
		UnlimitedQuota: true,
	}).Error)

	originalDB := dbmodel.DB
	originalSQLite := common.UsingSQLite.Load()
	originalPrefix := config.TokenKeyPrefix
	originalMemoryCache := config.MemoryCacheEnabled

	dbmodel.DB = db
	common.UsingSQLite.Store(true)
	config.TokenKeyPrefix = "sk-"
	config.MemoryCacheEnabled = false

	t.Cleanup(func() {
		dbmodel.DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
		config.TokenKeyPrefix = originalPrefix
		config.MemoryCacheEnabled = originalMemoryCache
	})
}

// observedLoggerMiddleware installs a recording logger the way
// gmw.NewLoggerMiddleware installs the real request logger.
func observedLoggerMiddleware(t *testing.T) (gin.HandlerFunc, *observer.ObservedLogs) {
	t.Helper()

	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)

	return func(c *gin.Context) {
		gmw.SetLogger(c, lg)
		c.Next()
	}, logs
}

// countKey counts occurrences of a field key on a log entry.
func countKey(fields []zapcore.Field, key string) int {
	n := 0
	for _, f := range fields {
		if f.Key == key {
			n++
		}
	}
	return n
}

// flatten renders a log entry's fields as key -> value.
func flatten(fields []zapcore.Field) map[string]any {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	return enc.Fields
}

// TestTokenAuth_BindsFullIdentityOntoRequestLogger is the acceptance test for the
// operator complaint: a log line emitted anywhere inside a request must carry the
// uuid and name of the user and token, not just their integer ids — and each field
// exactly once.
func TestTokenAuth_BindsFullIdentityOntoRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupIdentityBindingTestDB(t)
	inject, logs := observedLoggerMiddleware(t)

	engine := gin.New()
	engine.Use(inject, RequestId(), TokenAuth())
	engine.GET("/v1/models", func(c *gin.Context) {
		// A handler that says nothing about identity at all.
		gmw.GetLogger(c).Warn("handler log line")
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(context.Background())
	req.Header.Set("Authorization", "Bearer sk-alicetoken")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)

	entries := logs.FilterMessage("handler log line").All()
	require.Len(t, entries, 1)

	got := flatten(entries[0].Context)
	require.Equal(t, int64(175), got["user_id"])
	require.Equal(t, identityTestUserUUID, got["user_uuid"])
	require.Equal(t, "alice", got["username"])
	require.Equal(t, int64(257), got["token_id"])
	require.Equal(t, identityTestTokenUUID, got["token_uuid"])
	require.Equal(t, "laptop-cli", got["token_name"])
	require.NotEmpty(t, got["request_id"])

	for _, key := range []string{"user_id", "user_uuid", "username", "token_id", "token_uuid", "token_name"} {
		require.Equalf(t, 1, countKey(entries[0].Context, key), "field %s must appear exactly once", key)
	}
	// The token owner's email must never reach the logs.
	require.NotContains(t, got, "email")
}

// TestTokenAuth_AbortCarriesTokenIdentityBeforeUserLoad covers the early rejection
// paths (subnet, expiry, model restriction) that fire before the owner row is read.
func TestTokenAuth_AbortCarriesTokenIdentityBeforeUserLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupIdentityBindingTestDB(t)

	subnet := "10.0.0.0/8"
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Token{}).Where("id = ?", 257).
		Update("subnet", &subnet).Error)

	inject, logs := observedLoggerMiddleware(t)
	engine := gin.New()
	engine.Use(inject, RequestId(), TokenAuth())
	engine.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(context.Background())
	req.Header.Set("Authorization", "Bearer sk-alicetoken")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusForbidden, recorder.Code)

	entries := logs.FilterMessage("server abort").All()
	require.Len(t, entries, 1)
	// 4xx must stay at WARN so it does not page anyone.
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)

	got := flatten(entries[0].Context)
	require.Equal(t, int64(257), got["token_id"])
	require.Equal(t, identityTestTokenUUID, got["token_uuid"])
	require.Equal(t, "laptop-cli", got["token_name"])
	require.Equal(t, int64(175), got["user_id"])
	require.Equal(t, identityTestUserUUID, got["user_uuid"])
}

// TestAbortWithError_SurfacesIdentityTaggedOntoTheError proves the mechanism that
// fixes the reported log line: the quota error is built deep in model/, where no
// gin context exists, and its identity still reaches the log record.
func TestAbortWithError_SurfacesIdentityTaggedOntoTheError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inject, logs := observedLoggerMiddleware(t)

	const message = "insufficient user quota: required=50, available=0, userId=175, tokenId=257"

	engine := gin.New()
	engine.Use(inject, RequestId())
	engine.GET("/relay", func(c *gin.Context) {
		err := identity.Tag(
			errors.Errorf("insufficient user quota: required=%d, available=%d, userId=%d, tokenId=%d",
				50, 0, 175, 257),
			identity.NewUserRef(175, identityTestUserUUID, "alice"),
			identity.NewTokenRef(257, identityTestTokenUUID, "laptop-cli"))
		AbortWithError(c, http.StatusForbidden, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/relay", nil).WithContext(context.Background())
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusForbidden, recorder.Code)

	entries := logs.FilterMessage("server abort").All()
	require.Len(t, entries, 1)

	got := flatten(entries[0].Context)
	require.Equal(t, int64(175), got["user_id"])
	require.Equal(t, identityTestUserUUID, got["user_uuid"])
	require.Equal(t, "alice", got["username"])
	require.Equal(t, "laptop-cli", got["token_name"])

	// The message text itself must be untouched — clients and classifiers read it.
	require.Contains(t, got["error"], message)
	require.Contains(t, recorder.Body.String(), message)
}
