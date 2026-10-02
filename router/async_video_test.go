package router

import (
	"context"
	"encoding/json"
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
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/channeltype"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

// TestAsyncVideoShippedRouterCompatibility exercises the actual production router,
// TokenAuth, distributor, relay dispatch, database workers and final JSON response.
// Native MuAPI and a genuinely synchronous compatible provider use the same
// /v1/videos/generations client endpoint without changing /v1/videos semantics.
func TestAsyncVideoShippedRouterCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLOG, oldClient := model.DB, model.LOG_DB, client.HTTPClient
	oldRedis, oldSQLite, oldMemory, oldRate, oldWait, oldLogging := common.IsRedisEnabled(), common.UsingSQLite.Load(), config.MemoryCacheEnabled, config.RateLimitDisabled, config.AsyncVideoWaitSeconds, config.IsLogConsumeEnabled()
	model.DB, model.LOG_DB = db, db
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	config.MemoryCacheEnabled = false
	config.RateLimitDisabled = true
	config.AsyncVideoWaitSeconds = 10
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() {
		drain, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, graceful.Drain(drain))
		model.DB, model.LOG_DB, client.HTTPClient = oldDB, oldLOG, oldClient
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.MemoryCacheEnabled = oldMemory
		config.RateLimitDisabled = oldRate
		config.AsyncVideoWaitSeconds = oldWait
		config.SetLogConsumeEnabled(oldLogging)
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.AsyncTask{}, &model.AsyncTaskBinding{}, &model.AsyncTaskBindingRetry{}, &model.AsyncTaskLogReceipt{}, &model.UserRequestCost{}, &model.QuotaRefund{}, &model.Log{}, &model.Trace{}))
	var nativeCreates, syncCreates atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/estimate-cost"):
			_, _ = io.WriteString(w, `{"cost":0.40,"currency":"USD"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/veo3-fast":
			nativeCreates.Add(1)
			_, _ = io.WriteString(w, `{"request_id":"router-job","status":"processing"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/predictions/router-job/result":
			_, _ = io.WriteString(w, `{"id":"router-job","status":"completed","outputs":["https://media.example/native.mp4"]}`)
		case r.Method == http.MethodPost && (r.URL.Path == "/v1/videos" || r.URL.Path == "/v1/videos/generations"):
			syncCreates.Add(1)
			_, _ = io.WriteString(w, `{"created":123,"data":[{"url":"https://media.example/synchronous.mp4"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client.HTTPClient = upstream.Client()
	user := &model.User{Id: 818181, UUID: uuid.NewString(), Username: "async-router", Status: model.UserStatusEnabled, Quota: 10000000, Group: "default"}
	require.NoError(t, db.Create(user).Error)
	key := strings.ReplaceAll(uuid.NewString(), "-", "")
	token := &model.Token{Id: 818182, UUID: uuid.NewString(), UserId: user.Id, Key: key, Status: model.TokenStatusEnabled, RemainQuota: 10000000, ExpiredTime: -1}
	require.NoError(t, db.Create(token).Error)
	priorityHigh, priorityLow := int64(100), int64(1)
	native := &model.Channel{Id: 818183, UUID: uuid.NewString(), Type: channeltype.MuAPI, Status: model.ChannelStatusEnabled, Name: "native", Models: "veo3-fast", Group: "default", Priority: &priorityHigh, BaseURL: &upstream.URL, Key: "fixture-provider-key"}
	synchronous := &model.Channel{Id: 818184, UUID: uuid.NewString(), Type: channeltype.OpenAICompatible, Status: model.ChannelStatusEnabled, Name: "sync", Models: "veo3-fast,sync-video", Group: "default", Priority: &priorityLow, BaseURL: &upstream.URL, Key: "fixture-provider-key"}
	require.NoError(t, synchronous.SetModelPriceConfigs(map[string]model.ModelConfigLocal{
		"veo3-fast":  {PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 400}},
		"sync-video": {PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 400}},
	}))
	for _, channel := range []*model.Channel{native, synchronous} {
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, channel.AddAbilities())
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { gmw.SetLogger(c, logger.Logger); c.Next() })
	SetRelayRouter(engine)
	request := func(method, path, body, idempotency string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer sk-"+key)
		req.Header.Set("Content-Type", "application/json")
		if idempotency != "" {
			req.Header.Set("Idempotency-Key", idempotency)
		}
		writer := httptest.NewRecorder()
		engine.ServeHTTP(writer, req)
		return writer
	}
	legacy := request(http.MethodPost, "/v1/videos", `{"model":"veo3-fast","duration":5}`, "")
	require.Equal(t, 200, legacy.Code, legacy.Body.String())
	require.Contains(t, legacy.Body.String(), "synchronous.mp4")
	require.Zero(t, nativeCreates.Load(), "native high-priority channel must be excluded on the legacy route")
	directSync := request(http.MethodPost, "/v1/videos/generations", `{"model":"sync-video","duration":5}`, "")
	require.Equal(t, 200, directSync.Code, directSync.Body.String())
	require.EqualValues(t, 2, syncCreates.Load())
	workerCtx, stop := context.WithCancel(context.Background())
	asyncvideo.StartWorkers(workerCtx, relaycontroller.ResolveAsyncVideoProvider, 2)
	t.Cleanup(func() {
		stop()
		deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, model.WaitForBackgroundWorkers(deadline))
	})
	bridged := request(http.MethodPost, "/v1/videos/generations", `{"model":"veo3-fast","duration":5}`, "router-synchronous-bridge")
	require.Equal(t, 200, bridged.Code, bridged.Body.String())
	require.Contains(t, bridged.Body.String(), "native.mp4")
	require.EqualValues(t, 1, nativeCreates.Load())
	taskID := bridged.Header().Get("X-Async-Task-Id")
	require.NotEmpty(t, taskID)
	// The original key may be spent and the original channel disabled. Retrieval
	// and reattachment must precede both positive-balance and channel selection.
	require.NoError(t, db.Model(token).Updates(map[string]any{"remain_quota": 0, "status": model.TokenStatusExhausted}).Error)
	require.NoError(t, db.Model(user).Update("quota", 0).Error)
	require.NoError(t, db.Model(native).Update("status", model.ChannelStatusManuallyDisabled).Error)
	fetched := request(http.MethodGet, "/v1/async/videos/"+taskID, "", "")
	require.Equal(t, 200, fetched.Code, fetched.Body.String())
	var state map[string]any
	require.NoError(t, json.Unmarshal(fetched.Body.Bytes(), &state))
	require.Equal(t, "completed", state["status"])
	replay := request(http.MethodPost, "/v1/videos/generations", `{"duration":5,"model":"veo3-fast"}`, "router-synchronous-bridge")
	require.Equal(t, 200, replay.Code, replay.Body.String())
	require.EqualValues(t, 1, nativeCreates.Load())
	rejected := request(http.MethodPost, "/v1/async/videos", `{"duration":5,"model":"veo3-fast"}`, "new-spent-request")
	require.Equal(t, 403, rejected.Code, rejected.Body.String())
	require.EqualValues(t, 1, nativeCreates.Load())
	require.NoError(t, db.Model(token).Update("status", model.TokenStatusDisabled).Error)
	disabled := request(http.MethodGet, "/v1/async/videos/"+taskID, "", "")
	require.Equal(t, 401, disabled.Code, disabled.Body.String())
}
