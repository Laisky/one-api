package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	dbmodel "github.com/Laisky/one-api/model"
)

// TestTokenAuthCacheLookupErrorsLogNoCredentials exercises actual HTTP token
// authentication and captures local JSON logs for Redis and SQL lookup failures.
func TestTokenAuthCacheLookupErrorsLogNoCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, cache, database, diagnostic, level, kind string
	}{
		{"redis_get_failure_without_database", "get_failure", "nil", "ERR synthetic Redis GET failure", "ERROR", "server"},
		{"database_miss_with_cache", "miss", "open", "record not found", "WARN", "unauthorized"},
		{"database_miss_without_cache", "disabled", "open", "record not found", "WARN", "unauthorized"},
		{"database_failure_with_cache", "miss", "closed", "database is closed", "ERROR", "server"},
		{"malformed_cached_token", "corrupt", "open", "unexpected end of JSON input", "ERROR", "server"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousDB, previousRedis, previousEnabled := dbmodel.DB, common.RDB, common.IsRedisEnabled()
			previousPostgreSQL, previousPrefix := common.UsingPostgreSQL.Load(), config.TokenKeyPrefix
			dbmodel.DB = nil
			common.UsingPostgreSQL.Store(false)
			config.TokenKeyPrefix = "sk-"
			t.Cleanup(func() {
				dbmodel.DB, common.RDB = previousDB, previousRedis
				common.SetRedisEnabled(previousEnabled)
				common.UsingPostgreSQL.Store(previousPostgreSQL)
				config.TokenKeyPrefix = previousPrefix
			})

			const key = "syntheticgetredactioncredentialboundary0516"
			if tc.database != "nil" {
				db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tokens.db")), &gorm.Config{Logger: gormlogger.Discard})
				require.NoError(t, err)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
				require.NoError(t, db.AutoMigrate(&dbmodel.Token{}))
				dbmodel.DB = db
				if tc.cache == "corrupt" {
					require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, key, name, status, expired_time, remain_quota) VALUES (?, ?, ?, ?, ?, ?, ?)",
						516, 42, key, "synthetic-get-token", dbmodel.TokenStatusEnabled, -1, 100).Error)
				}
				if tc.database == "closed" {
					require.NoError(t, sqlDB.Close())
				}
			}

			redisServer := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			common.RDB = client
			common.SetRedisEnabled(tc.cache != "disabled")
			switch tc.cache {
			case "get_failure":
				redisServer.SetError(tc.diagnostic)
			case "corrupt":
				require.NoError(t, redisServer.Set("token:"+key, "{"))
			}

			logPath := filepath.Join(t.TempDir(), "auth-error.log")
			captured, err := glog.New(
				glog.WithName("token-auth-cache-redaction-test"),
				glog.WithLevel(glog.LevelInfo),
				glog.WithEncoding(glog.EncodingJSON),
				glog.WithOutputPaths([]string{logPath}),
				glog.WithErrorOutputPaths([]string{logPath}),
			)
			require.NoError(t, err)
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				gmw.SetLogger(c, captured.With(zap.String("request_id", "synthetic-get-request")))
				c.Set(helper.RequestIdKey, "synthetic-get-request")
				c.Next()
			})
			engine.Use(TokenAuth())
			dispatched := false
			engine.GET("/v1/models", func(c *gin.Context) {
				dispatched = true
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			request.Header.Set("Authorization", "Bearer sk-"+key)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)

			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.False(t, dispatched, "lookup errors must stop the protected handler")
			require.NoError(t, captured.Sync())
			output, err := os.ReadFile(logPath)
			require.NoError(t, err)
			var event map[string]any
			require.NoError(t, json.Unmarshal(output, &event))
			require.Equal(t, "server abort", event["message"])
			require.Equal(t, tc.level, event["level"])
			require.Equal(t, tc.kind, event["error_kind"])
			require.Equal(t, "synthetic-get-request", event["request_id"])
			require.EqualValues(t, http.StatusUnauthorized, event["status_code"])
			require.Contains(t, event["error"], tc.diagnostic, "retain the underlying failure diagnostic")
			require.Contains(t, event["error"], helper.MaskAPIKey(key), "retain the existing masked credential reference")
			t.Run("logs", func(t *testing.T) {
				require.NotContains(t, string(output), key, "inner errors must not expose credentials in any encoded log field")
			})
			t.Run("response", func(t *testing.T) {
				require.NotContains(t, response.Body.String(), key, "authentication error responses must also exclude credentials")
			})
		})
	}
}
