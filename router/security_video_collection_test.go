package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityLegacyVideoCollectionOwnership traverses real authentication, binding, distribution and provider I/O.
func TestSecurityLegacyVideoCollectionOwnership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLOG, oldClient := model.DB, model.LOG_DB, client.HTTPClient
	oldRedis, oldSQLite, oldMemory, oldRate := common.IsRedisEnabled(), common.UsingSQLite.Load(), config.MemoryCacheEnabled, config.RateLimitDisabled
	model.DB, model.LOG_DB = db, db
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	config.MemoryCacheEnabled = false
	config.RateLimitDisabled = true
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, graceful.Drain(ctx))
		model.DB, model.LOG_DB, client.HTTPClient = oldDB, oldLOG, oldClient
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.MemoryCacheEnabled, config.RateLimitDisabled = oldMemory, oldRate
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.AsyncTaskBinding{}, &model.Trace{}))
	var collections, items atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.TrimSuffix(r.URL.Path, "/") == "/v1/videos" {
			collections.Add(1)
			_, _ = io.WriteString(w, `{"data":[{"id":"foreign-provider-task","status":"completed"}]}`)
			return
		}
		items.Add(1)
		_, _ = io.WriteString(w, `{"id":"owned-task","status":"completed"}`)
	}))
	defer upstream.Close()
	client.HTTPClient = upstream.Client()
	channel := &model.Channel{Id: 8820, UUID: uuid.NewString(), Type: channeltype.OpenAI, Status: model.ChannelStatusEnabled, Name: "security-video", Models: "sora-2", Group: "default", BaseURL: &upstream.URL, Key: "synthetic-provider-key"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities())
	var keys []string
	for i := 0; i < 2; i++ {
		user := &model.User{Id: 8800 + i, UUID: uuid.NewString(), Username: []string{"video-owner-a", "video-owner-b"}[i], AccessToken: uuid.NewString(), AffCode: uuid.NewString(), Status: model.UserStatusEnabled, Quota: 10000000, Group: "default"}
		require.NoError(t, db.Create(user).Error)
		key := strings.ReplaceAll(uuid.NewString(), "-", "")
		keys = append(keys, key)
		require.NoError(t, db.Create(&model.Token{Id: 8810 + i, UUID: uuid.NewString(), UserId: user.Id, Key: key, Status: model.TokenStatusEnabled, RemainQuota: 10000000, ExpiredTime: -1}).Error)
	}
	now := time.Now().UnixMilli()
	require.NoError(t, db.Create(&model.AsyncTaskBinding{TaskID: "owned-task", TaskType: "video", UserID: 8800, TokenID: 8810, ChannelID: channel.Id, ChannelType: channel.Type, OriginModel: "sora-2", ActualModel: "sora-2", CreatedAt: now, UpdatedAt: now, LastAccessedAt: now}).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { gmw.SetLogger(c, logger.Logger); c.Next() })
	SetRelayRouter(engine)
	send := func(key, method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer sk-"+key)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	for _, key := range keys {
		for _, query := range []string{"?model=sora-2", "", "?model=sora-2&after=foreign-provider-task&limit=100"} {
			w := send(key, http.MethodGet, "/v1/videos"+query)
			if w.Code == http.StatusOK && collections.Load() > 0 && strings.Contains(w.Body.String(), "foreign-provider-task") {
				t.Log("REPRODUCED_473_FOREIGN_COLLECTION_THROUGH_REAL_ROUTER")
			}
			require.Equal(t, http.StatusNotImplemented, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "foreign-provider-task")
		}
	}
	require.Zero(t, collections.Load(), "provider account collection must never be requested")
	for _, path := range []string{"/v1/videos/owned-task", "/v1/videos/owned-task/content", "/v1/videos/unbound"} {
		w := send(keys[1], http.MethodGet, path)
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	}
	require.Zero(t, items.Load())
	w := send(keys[0], http.MethodGet, "/v1/videos/owned-task")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.EqualValues(t, 1, items.Load())
	w = send(keys[1], http.MethodDelete, "/v1/videos/owned-task")
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.EqualValues(t, 1, items.Load())
}
