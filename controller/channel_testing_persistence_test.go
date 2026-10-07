package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// channelPersistenceRequestKey identifies a synthetic request value whose preservation is checked after completion.
type channelPersistenceRequestKey struct{}

// channelPersistenceObservation records the context and real database result of the deferred latency update.
type channelPersistenceObservation struct {
	err         error
	requestErr  error
	value       any
	deadline    time.Time
	hasDeadline bool
	carriesGin  bool
}

// TestChannelResponseTimeSurvivesHTTPCompletion verifies the real channel-test handler can persist
// probe latency after net/http cancels the completed request, while retaining its existing envelope.
// Parameters: t is the running test. Returns: none.
func TestChannelResponseTimeSurvivesHTTPCompletion(t *testing.T) {
	for _, tc := range []struct {
		name             string
		creditsFailure   bool
		databaseFailure  bool
		requestLifecycle string
	}{
		{name: "successful Responses probe"},
		{name: "HTTP200 Responses credits failure", creditsFailure: true},
		{name: "database errors remain visible", databaseFailure: true},
		{name: "explicit request cancellation", requestLifecycle: "cancel"},
		{name: "expired request deadline", requestLifecycle: "deadline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupChannelSweepTestEnvironment(t)
			core, observed := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewWithName("channel-persistence-fixture", glog.LevelDebug,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)

			upstreamSeen := make(chan map[string]any, 1)
			requestCancel := make(chan context.CancelFunc, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				decodeErr := json.NewDecoder(r.Body).Decode(&body)
				upstreamSeen <- map[string]any{"path": r.URL.Path, "body": body, "decodeError": decodeErr}
				if tc.requestLifecycle == "cancel" {
					(<-requestCancel)()
				}
				// A nonzero latency makes a missed update distinguishable from a valid zero failure latency.
				time.Sleep(25 * time.Millisecond)
				w.Header().Set("Content-Type", "application/json")
				if tc.creditsFailure {
					_, _ = io.WriteString(w, `{"id":"resp_fixture","status":"failed","error":{"message":"You have no credits remaining","type":"insufficient_quota","code":"insufficient_quota"}}`)
					return
				}
				_, _ = io.WriteString(w, `{"id":"resp_fixture","object":"response","status":"completed","model":"gpt-4.1-nano","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"PROBE_OK"}]}],"usage":{"input_tokens":7,"output_tokens":1,"total_tokens":8}}`)
			}))
			t.Cleanup(upstream.Close)
			oldClient, oldSinks := client.HTTPClient, config.TraceSinks
			client.HTTPClient, config.TraceSinks = upstream.Client(), []string{config.TraceSinkNone}
			t.Cleanup(func() { client.HTTPClient, config.TraceSinks = oldClient, oldSinks })
			baseURL := upstream.URL
			channel := &model.Channel{Name: "channel-persistence-fixture", Type: channeltype.OpenAI,
				Status: model.ChannelStatusEnabled, Key: "fixture-channel-key", BaseURL: &baseURL,
				Models: "gpt-4.1-nano", ResponseTime: 777, TestTime: 1}
			require.NoError(t, model.DB.Create(channel).Error)

			requestContext := make(chan context.Context, 1)
			observation := make(chan channelPersistenceObservation, 1)
			updateFinished := make(chan error, 1)
			// Gate before transaction acquisition, so the fixture releases no connection while waiting
			// for the handler's request to finish. The actual database update then sees the true context.
			require.NoError(t, model.DB.Callback().Update().Before("gorm:begin_transaction").Register(
				"test:channel_latency_after_request", func(tx *gorm.DB) {
					original := <-requestContext
					select {
					case <-original.Done():
					case <-time.After(5 * time.Second):
						tx.AddError(context.DeadlineExceeded)
					}
					ctx := tx.Statement.Context
					deadline, ok := ctx.Deadline()
					_, carriesGin := gmw.GetGinCtxFromStdCtx(ctx)
					observation <- channelPersistenceObservation{err: ctx.Err(), requestErr: original.Err(), value: ctx.Value(channelPersistenceRequestKey{}), deadline: deadline, hasDeadline: ok, carriesGin: carriesGin}
					gmw.GetLogger(ctx).Debug("channel persistence fixture context")
					if tc.databaseFailure {
						// Exercise a real SQL failure without corrupting the fixture's original channel table.
						tx.Statement.Table = "missing_channel_persistence_table"
					}
				}))
			require.NoError(t, model.DB.Callback().Update().After("gorm:commit_or_rollback_transaction").Register(
				"test:channel_latency_finished", func(tx *gorm.DB) { updateFinished <- tx.Error }))

			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tc.requestLifecycle == "cancel" {
					ctx, cancel := context.WithCancel(c.Request.Context())
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
					requestCancel <- cancel
				} else if tc.requestLifecycle == "deadline" {
					ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Millisecond)
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
				}
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), channelPersistenceRequestKey{}, "request-value"))
				gmw.SetLogger(c, lg.With(zap.String("request_id", "request-fixture"), zap.String("trace_id", "trace-fixture")))
				requestContext <- c.Request.Context()
				c.Next()
			})
			router.POST("/api/channel/test/:id", TestChannel)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			response, err := server.Client().Post(server.URL+"/api/channel/test/"+channel.UUID, "application/json", nil)
			require.NoError(t, err)
			body, readErr := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, readErr)
			require.Equal(t, http.StatusOK, response.StatusCode)

			var got channelPersistenceObservation
			select {
			case got = <-observation:
			case <-time.After(5 * time.Second):
				t.Fatal("latency update did not observe completed HTTP request")
			}
			var updateErr error
			select {
			case updateErr = <-updateFinished:
			case <-time.After(5 * time.Second):
				t.Fatal("latency update did not complete")
			}
			// Join both asynchronous writes before fixture globals are restored, including RED runs.
			require.Eventually(t, func() bool {
				var count int64
				return model.DB.Model(&model.Log{}).Where("channel_id = ?", channel.Id).Count(&count).Error == nil && count == 1
			}, 5*time.Second, 10*time.Millisecond)
			if updateErr != nil {
				require.Eventually(t, func() bool { return observed.FilterMessage("failed to update response time").Len() == 1 },
					5*time.Second, 10*time.Millisecond)
			}

			seen := <-upstreamSeen
			require.Equal(t, "/v1/responses", seen["path"])
			require.Nil(t, seen["decodeError"])
			request := seen["body"].(map[string]any)
			require.Equal(t, "gpt-4.1-nano", request["model"])
			require.NotEqual(t, true, request["stream"])
			require.Contains(t, request, "input")
			var envelope map[string]any
			require.NoError(t, json.Unmarshal(body, &envelope))
			require.Len(t, envelope, 4)
			require.Equal(t, !tc.creditsFailure, envelope["success"])
			require.Equal(t, "gpt-4.1-nano", envelope["modelName"])
			if tc.creditsFailure {
				require.Equal(t, "You have no credits remaining", envelope["message"])
				require.Equal(t, float64(0), envelope["time"])
			} else {
				require.Equal(t, "PROBE_OK", envelope["message"])
				require.Greater(t, envelope["time"].(float64), float64(0))
			}
			var audit model.Log
			require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).First(&audit).Error)
			require.Equal(t, channel.UUID, *audit.ChannelUUID)
			require.Equal(t, model.LogTypeTest, audit.Type)
			if tc.creditsFailure {
				require.Contains(t, audit.Content, "You have no credits remaining")
				require.Zero(t, audit.Quota)
			} else {
				require.Contains(t, audit.Content, "PROBE_OK")
			}

			if tc.requestLifecycle == "deadline" {
				require.ErrorIs(t, got.requestErr, context.DeadlineExceeded)
			} else {
				require.ErrorIs(t, got.requestErr, context.Canceled)
			}
			require.NoError(t, got.err, "finished request must not cancel its deferred latency write")
			require.Equal(t, "request-value", got.value)
			require.False(t, got.carriesGin, "background persistence must not retain a recycled Gin context")
			require.True(t, got.hasDeadline, "detached persistence must have a finite budget")
			require.WithinDuration(t, time.Now(), got.deadline, 6*time.Second)
			contextLogs := observed.FilterMessage("channel persistence fixture context").All()
			require.Len(t, contextLogs, 1)
			fields := contextLogs[0].ContextMap()
			require.Equal(t, "request-fixture", fields["request_id"])
			require.Equal(t, "trace-fixture", fields["trace_id"])
			require.Equal(t, channel.UUID, fields["channel_uuid"])
			require.Equal(t, channel.Name, fields["channel_name"])
			require.EqualValues(t, channel.Id, fields["channel_id"])
			var stored model.Channel
			require.NoError(t, model.DB.First(&stored, channel.Id).Error)
			if tc.databaseFailure {
				require.ErrorContains(t, updateErr, "missing_channel_persistence_table")
				require.Equal(t, 777, stored.ResponseTime)
				require.Equal(t, int64(1), stored.TestTime)
				errors := observed.FilterMessage("failed to update response time").All()
				require.Len(t, errors, 1)
				require.Equal(t, channel.UUID, errors[0].ContextMap()["channel_uuid"])
			} else {
				require.NoError(t, updateErr)
				require.Greater(t, stored.TestTime, int64(1))
				require.Equal(t, envelope["time"], float64(stored.ResponseTime)/1000)
				require.Empty(t, observed.FilterMessage("failed to update response time").All())
			}
		})
	}
}
