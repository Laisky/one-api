package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/channeltype"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

// disconnectBillingRouter builds the shipped router and real auth/distributor
// with a wallet that can pay exactly one quoted task. No accounting is mocked.
func disconnectBillingRouter(t *testing.T, base string) (*gin.Engine, string, int, int, <-chan error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
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
		model.DB, model.LOG_DB, client.HTTPClient = oldDB, oldLOG, oldClient
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.MemoryCacheEnabled = oldMemory
		config.RateLimitDisabled = oldRate
		config.AsyncVideoWaitSeconds = oldWait
		config.SetLogConsumeEnabled(oldLogging)
		require.NoError(t, conn.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.AsyncTask{}, &model.AsyncTaskBinding{}, &model.AsyncTaskBindingRetry{}, &model.AsyncTaskLogReceipt{}, &model.UserRequestCost{}, &model.QuotaRefund{}, &model.Log{}, &model.Trace{}))
	user := &model.User{Id: 919191, UUID: uuid.NewString(), Username: "disconnect-owner", Status: model.UserStatusEnabled, Quota: 200000, Group: "default"}
	key := strings.ReplaceAll(uuid.NewString(), "-", "")
	token := &model.Token{Id: 919192, UUID: uuid.NewString(), UserId: user.Id, Key: key, Status: model.TokenStatusEnabled, RemainQuota: 200000, ExpiredTime: -1}
	channel := &model.Channel{Id: 919193, UUID: uuid.NewString(), Type: channeltype.MuAPI, Status: model.ChannelStatusEnabled, Name: "disconnect-native", Models: "veo3-fast", Group: "default", BaseURL: &base, Key: "fixture-only"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities())
	ended := make(chan error, 4)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Next()
		if c.Request.Method == http.MethodPost {
			ended <- c.Request.Context().Err()
		}
	})
	engine.Use(middleware.RequestId())
	SetRelayRouter(engine)
	return engine, key, user.Id, token.Id, ended
}

// TestAsyncBillingRealSocketDisconnect covers disconnect before dispatch, after
// acceptance, during an active provider GET, and a real HTTP 504. Closing the
// client's TCP connection must end only its waiter, never refund paid work or
// cancel the independent worker. The higher final cost is collected as debt.
func TestAsyncBillingRealSocketDisconnect(t *testing.T) {
	for _, stage := range []string{"before_dispatch", "accepted", "polling", "wait_timeout"} {
		t.Run(stage, func(t *testing.T) {
			var creates, polls atomic.Int32
			pollStarted, releasePoll := make(chan struct{}, 1), make(chan struct{})
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/estimate-cost"):
					_, _ = io.WriteString(w, `{"cost":0.4,"currency":"USD"}`)
				case r.Method == http.MethodPost:
					creates.Add(1)
					_, _ = io.WriteString(w, `{"request_id":"disconnect-job"}`)
				case r.Method == http.MethodGet:
					polls.Add(1)
					if stage == "polling" {
						pollStarted <- struct{}{}
						select {
						case <-releasePoll:
						case <-r.Context().Done():
							return
						}
					}
					_, _ = io.WriteString(w, `{"id":"disconnect-job","status":"completed","outputs":["https://media.example/disconnected.mp4"],"cost":{"amount_usd":0.8}}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer provider.Close()
			engine, key, userID, tokenID, ended := disconnectBillingRouter(t, provider.URL)
			client.HTTPClient = provider.Client()
			if stage == "wait_timeout" {
				config.AsyncVideoWaitSeconds = 1
			}
			gateway := httptest.NewServer(engine)
			defer gateway.Close()
			const body = `{"model":"veo3-fast","duration":5,"prompt":"socket fixture"}`
			var socket net.Conn
			if stage == "wait_timeout" {
				request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/videos/generations", strings.NewReader(body))
				require.NoError(t, err)
				request.Header.Set("Authorization", "Bearer sk-"+key)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", "socket-job")
				response, err := gateway.Client().Do(request)
				require.NoError(t, err)
				data, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Equal(t, http.StatusGatewayTimeout, response.StatusCode, string(data))
				require.Contains(t, string(data), "async_video_wait_timeout")
			} else {
				var err error
				socket, err = net.DialTimeout("tcp", strings.TrimPrefix(gateway.URL, "http://"), 2*time.Second)
				require.NoError(t, err)
				t.Cleanup(func() { _ = socket.Close() })
				_, err = fmt.Fprintf(socket, "POST /v1/videos/generations HTTP/1.1\r\nHost: fixture\r\nAuthorization: Bearer sk-%s\r\nContent-Type: application/json\r\nIdempotency-Key: socket-job\r\nContent-Length: %d\r\n\r\n%s", key, len(body), body)
				require.NoError(t, err)
			}
			var task model.AsyncTask
			require.Eventually(t, func() bool { return model.DB.Where("user_id = ?", userID).Take(&task).Error == nil }, 3*time.Second, 10*time.Millisecond)
			step := func() error {
				if err := model.DB.Model(&model.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error; err != nil {
					return err
				}
				_, err := asyncvideo.ProcessOne(context.Background(), relaycontroller.ResolveAsyncVideoProvider, time.Now())
				return err
			}
			if stage == "accepted" || stage == "polling" {
				require.NoError(t, step())
			}
			result := make(chan error, 1)
			if stage == "polling" {
				go func() { result <- step() }()
				select {
				case <-pollStarted:
				case <-time.After(3 * time.Second):
					t.Fatal("provider poll did not start")
				}
			}
			if socket != nil {
				require.NoError(t, socket.Close())
			}
			select {
			case err := <-ended:
				if stage != "wait_timeout" {
					require.True(t, errors.Is(err, context.Canceled), "real TCP close must cancel the HTTP waiter")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP waiter did not end")
			}
			var user model.User
			require.NoError(t, model.DB.Where("id = ?", userID).Take(&user).Error)
			require.Zero(t, user.Quota, "disconnect/timeout must not restore reserved funds")
			require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
			if stage == "polling" {
				close(releasePoll)
				require.NoError(t, <-result)
			} else {
				if stage != "accepted" {
					require.NoError(t, step())
				}
				require.NoError(t, step())
			}
			require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
			require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
			require.NoError(t, model.DB.Where("id = ?", task.ID).Take(&task).Error)
			require.NoError(t, model.DB.Where("id = ?", userID).Take(&user).Error)
			var token model.Token
			require.NoError(t, model.DB.Where("id = ?", tokenID).Take(&token).Error)
			require.Equal(t, model.AsyncTaskCompleted, task.State)
			require.Equal(t, model.AsyncBillingSettled, task.BillingState)
			require.EqualValues(t, -200000, user.Quota)
			require.EqualValues(t, 400000, user.UsedQuota)
			require.EqualValues(t, 1, user.RequestCount)
			require.EqualValues(t, -200000, token.RemainQuota)
			require.EqualValues(t, 400000, token.UsedQuota)
			require.EqualValues(t, 1, creates.Load())
			require.EqualValues(t, 1, polls.Load())
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, 400000, logs[0].Quota)
			var cost model.UserRequestCost
			require.NoError(t, model.DB.Where("request_id = ?", task.RequestID).Take(&cost).Error)
			require.EqualValues(t, 400000, cost.Quota)
			// A spent/debtor token may retrieve the already-paid result, but this cannot
			// create another upstream generation or waive the persisted supplement.
			request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/videos/generations", strings.NewReader(body))
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer sk-"+key)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "socket-job")
			response, err := gateway.Client().Do(request)
			require.NoError(t, err)
			data, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode, string(data))
			require.Contains(t, string(data), "disconnected.mp4")
			require.EqualValues(t, 1, creates.Load())
		})
	}
}
