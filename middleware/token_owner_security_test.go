package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/blacklist"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
)

// securityOwnerDB installs a database and optional shared cache for account
// authorization fixtures. It restores global state when the test completes.
func securityOwnerDB(t *testing.T, cached bool) *gorm.DB {
	t.Helper()
	db := setupTokenAuthChannelSuffixTestDB(t)
	oldDB, oldRDB, oldRedis := dbmodel.DB, common.RDB, common.IsRedisEnabled()
	oldPrefix, oldSQLite, oldBan := config.TokenKeyPrefix, common.UsingSQLite.Load(), blacklist.IsUserBanned(1)
	dbmodel.DB = db
	common.SetRedisEnabled(cached)
	common.UsingSQLite.Store(true)
	config.TokenKeyPrefix = "sk-"
	blacklist.UnbanUser(1)
	t.Cleanup(func() {
		dbmodel.DB, common.RDB = oldDB, oldRDB
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.TokenKeyPrefix = oldPrefix
		if oldBan {
			blacklist.BanUser(1)
		} else {
			blacklist.UnbanUser(1)
		}
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	if cached {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		common.RDB = client
		t.Cleanup(func() { require.NoError(t, client.Close()) })
	}
	return db
}

// securityOwnerRequest runs the production authentication middleware and records
// whether the protected handler was reached, without contacting a provider.
func securityOwnerRequest(t *testing.T, method, path, header string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.Use(TokenAuth())
	r.Handle(method, path, func(c *gin.Context) {
		require.Equal(t, 1, c.GetInt(ctxkey.Id))
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(method, path, strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"test"}]}`))
	req.Header.Set("Content-Type", "application/json")
	value := "sk-admintoken"
	if header == "Authorization" {
		value = "Bearer " + value
	}
	req.Header.Set(header, value)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestSecurityTokenOwnerStates verifies enabled, disabled, deleted and unknown
// durable statuses across inference, MCP and other token-authenticated routes.
func TestSecurityTokenOwnerStates(t *testing.T) {
	db := securityOwnerDB(t, false)
	for _, status := range []int{dbmodel.UserStatusEnabled, dbmodel.UserStatusDisabled, dbmodel.UserStatusDeleted, 0, 99} {
		for _, route := range []struct{ method, path string }{
			{"GET", "/v1/models"}, {"POST", "/v1/chat/completions"},
			{"POST", "/v1/messages"}, {"POST", "/mcp"},
			{"GET", "/api/user/get-by-token"}, {"GET", "/v1/async/videos/owned-task"},
		} {
			for _, header := range []string{"Authorization", "X-Api-Key", "Api-Key"} {
				t.Run(fmt.Sprintf("%d-%s-%s", status, route.path, header), func(t *testing.T) {
					require.NoError(t, db.Model(&dbmodel.User{}).Where("id = ?", 1).Update("status", status).Error)
					w := securityOwnerRequest(t, route.method, route.path, header)
					want := http.StatusForbidden
					if status == dbmodel.UserStatusEnabled {
						want = http.StatusNoContent
					}
					require.Equal(t, want, w.Code, w.Body.String())
				})
			}
		}
	}
}

// TestSecurityTokenOwnerStaleCache verifies that a stale enabled user object
// cannot authorize work after another replica commits a status change.
func TestSecurityTokenOwnerStaleCache(t *testing.T) {
	db := securityOwnerDB(t, true)
	user, err := dbmodel.CacheGetUserById(context.Background(), 1)
	require.NoError(t, err)
	payload, err := json.Marshal(user)
	require.NoError(t, err)
	require.NoError(t, common.RedisSet(context.Background(), "user_obj:1", string(payload), time.Hour))
	for _, status := range []int{dbmodel.UserStatusDisabled, dbmodel.UserStatusDeleted, 99} {
		require.NoError(t, db.Model(&dbmodel.User{}).Where("id = ?", 1).Update("status", status).Error)
		blacklist.UnbanUser(1) // A second process does not share the local map.
		w := securityOwnerRequest(t, "GET", "/v1/models", "Authorization")
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	}
}

// TestSecurityTokenOwnerDeleteRestart verifies a persisted self/admin deletion
// denies retained tokens even after the process-local blacklist is lost.
func TestSecurityTokenOwnerDeleteRestart(t *testing.T) {
	db := securityOwnerDB(t, false)
	require.NoError(t, dbmodel.DeleteUserById(1))
	blacklist.UnbanUser(1)
	require.Equal(t, http.StatusForbidden, securityOwnerRequest(t, "POST", "/mcp", "Authorization").Code)
	require.NoError(t, db.Unscoped().Delete(&dbmodel.User{}, 1).Error)
	require.Equal(t, http.StatusForbidden, securityOwnerRequest(t, "GET", "/v1/models", "Authorization").Code)
}
