package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	gmw "github.com/Laisky/gin-middlewares/v7"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
)

// routingModelMappedUpstreamID is what AWS Bedrock and Vertex adaptors write into
// ctxkey.RequestModel during ConvertRequest: the channel-mapped upstream model.
const routingModelMappedUpstreamID = "mapped-upstream-model-id"

// routingModelAbility reads the persisted ability of channelID for the routed
// model, failing the test if it does not exist.
func routingModelAbility(t *testing.T, channelID int) dbmodel.Ability {
	t.Helper()
	var row dbmodel.Ability
	require.NoError(t, dbmodel.DB.Where("channel_id = ? AND model = ?", channelID, retryOrderModel).Take(&row).Error)
	return row
}

// TestRelayCooldownUsesRoutedModelWhenAdaptorRewritesRequestModel drives the
// real Relay retry loop and the real channel error processing with an adaptor
// that, like the AWS Bedrock and Vertex adaptors with a channel model mapping,
// overwrites ctxkey.RequestModel with the mapped upstream id while converting the
// request. Routing, retry selection, the adaptive cooldown of the failed channel
// and the success reset of the winning channel must all keep using the routed
// (requested) model, whose abilities exist, not the mapped upstream id.
func TestRelayCooldownUsesRoutedModelWhenAdaptorRewritesRequestModel(t *testing.T) {
	for _, routing := range routingPaths {
		t.Run(routing.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			for _, name := range []string{"CHANNEL_SUSPEND_SECONDS_FOR_429", "CHANNEL_SUSPEND_SECONDS_FOR_5XX"} {
				t.Setenv(name, "") // registers restoration of the original value
				require.NoError(t, os.Unsetenv(name))
			}
			primary := retryOrderChannel(9301, "mapped-primary", 10)
			backup := retryOrderChannel(9302, "mapped-backup", 0)
			setupRetryOrderDB(t, []*dbmodel.Channel{primary, backup})
			previousRetries, previousMemory := config.RetryTimes, config.MemoryCacheEnabled
			config.RetryTimes, config.MemoryCacheEnabled = 1, routing.memoryCache
			t.Cleanup(func() { config.RetryTimes, config.MemoryCacheEnabled = previousRetries, previousMemory })
			if routing.memoryCache {
				dbmodel.InitChannelCache()
			}
			// The backup carries an expired streak from an earlier episode; its
			// genuine success must reset it under the routed model's ability.
			earlier := time.Now().UTC().Add(-time.Hour)
			_, err := dbmodel.RecordAbilityCooldownFailure(context.Background(), retryOrderGroup, retryOrderModel, backup.Id, earlier, earlier)
			require.NoError(t, err)
			require.Equal(t, 1, routingModelAbility(t, backup.Id).CooldownFailures)

			var order []int
			relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
				id := c.GetInt(ctxkey.ChannelId)
				order = append(order, id)
				c.Set(ctxkey.RequestModel, routingModelMappedUpstreamID) // what the adaptor does
				c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
				if id == primary.Id {
					return &model.ErrorWithStatusCode{StatusCode: http.StatusTooManyRequests, Error: model.Error{
						Message: "rate limit exceeded", Type: model.ErrorTypeRateLimit, Code: "rate_limit_exceeded", FromUpstream: true,
					}}
				}
				c.Status(http.StatusOK)
				_, _ = c.Writer.Write([]byte(`{"ok":true}`))
				return nil
			}
			t.Cleanup(func() { relayHelperForTest = nil })

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, logger.Logger)
			body, err := json.Marshal(map[string]any{"model": retryOrderModel, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
			require.NoError(t, err)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(ctxkey.Id, 1)
			c.Set(ctxkey.TokenId, 2)
			c.Set(ctxkey.Group, retryOrderGroup)
			c.Set(ctxkey.RequestModel, retryOrderModel)
			middleware.SetupContextForSelectedChannel(c, primary, retryOrderModel)

			Relay(c)
			drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, graceful.Drain(drainCtx))

			require.Equal(t, []int{primary.Id, backup.Id}, order, "the retry must select by the routed model")
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			failed := routingModelAbility(t, primary.Id)
			require.Equal(t, 1, failed.CooldownFailures, "the provider 429 must cool the routed ability")
			require.NotNil(t, failed.AdaptiveSuspendUntil)
			require.Zero(t, routingModelAbility(t, backup.Id).CooldownFailures, "the winning attempt must reset the routed ability")
		})
	}
}

// TestRelayCooldownIgnoresReadOnlyOutcomes drives Relay with read-only provider
// calls (a GET video status poll). A successful poll proves only that the
// provider answers reads, not that generation recovered, so it must not reset
// the adaptive streak built by failed creations; a failed poll keeps the legacy
// fixed suspension instead of climbing the adaptive ladder. A POST success is
// the control that must reset the streak.
func TestRelayCooldownIgnoresReadOnlyOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		fail       bool
		wantStreak int
		wantFixed  bool
	}{
		{name: "get_success_keeps_streak", method: http.MethodGet, wantStreak: 1},
		{name: "get_rate_limit_is_legacy", method: http.MethodGet, fail: true, wantStreak: 1, wantFixed: true},
		{name: "post_success_resets_control", method: http.MethodPost, wantStreak: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			for _, name := range []string{"CHANNEL_SUSPEND_SECONDS_FOR_429", "CHANNEL_SUSPEND_SECONDS_FOR_5XX"} {
				t.Setenv(name, "")
				require.NoError(t, os.Unsetenv(name))
			}
			only := retryOrderChannel(9311, "read-only", 0)
			setupRetryOrderDB(t, []*dbmodel.Channel{only})
			previousRetries := config.RetryTimes
			config.RetryTimes = 0
			t.Cleanup(func() { config.RetryTimes = previousRetries })
			earlier := time.Now().UTC().Add(-time.Hour)
			_, err := dbmodel.RecordAbilityCooldownFailure(context.Background(), retryOrderGroup, retryOrderModel, only.Id, earlier, earlier)
			require.NoError(t, err)

			relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
				c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
				if tc.fail {
					return &model.ErrorWithStatusCode{StatusCode: http.StatusTooManyRequests, Error: model.Error{
						Message: "rate limit exceeded", Type: model.ErrorTypeRateLimit, Code: "rate_limit_exceeded", FromUpstream: true,
					}}
				}
				c.Status(http.StatusOK)
				_, _ = c.Writer.Write([]byte(`{"status":"in_progress"}`))
				return nil
			}
			t.Cleanup(func() { relayHelperForTest = nil })

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, logger.Logger)
			path, body := "/v1/videos/video_fixture", []byte(nil)
			if tc.method == http.MethodPost {
				path = "/v1/chat/completions"
				body, err = json.Marshal(map[string]any{"model": retryOrderModel, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
				require.NoError(t, err)
			}
			c.Request = httptest.NewRequest(tc.method, path, bytes.NewReader(body))
			c.Set(ctxkey.Id, 1)
			c.Set(ctxkey.TokenId, 2)
			c.Set(ctxkey.Group, retryOrderGroup)
			c.Set(ctxkey.RequestModel, retryOrderModel)
			middleware.SetupContextForSelectedChannel(c, only, retryOrderModel)

			Relay(c)
			drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, graceful.Drain(drainCtx))

			row := routingModelAbility(t, only.Id)
			require.Equal(t, tc.wantStreak, row.CooldownFailures)
			require.Equal(t, tc.wantFixed, row.SuspendUntil != nil, "read-only failures keep the legacy fixed suspension")
		})
	}
}

// TestRelayCooldownIgnoresStaleUpstreamSnapshot models a replica whose routing
// cache still holds a channel snapshot from before an administrator rotated the
// channel's key. Its attempts are dispatched after the change but against the old
// upstream, so their failures must not rebuild a cooldown streak for the new
// upstream (which would sideline the repaired channel on every replica). An
// attempt routed with the current channel snapshot is the control that must
// still advance the ladder.
func TestRelayCooldownIgnoresStaleUpstreamSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stale      bool
		wantStreak int
	}{
		{name: "stale_snapshot", stale: true, wantStreak: 0},
		{name: "current_snapshot_control", stale: false, wantStreak: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			for _, name := range []string{"CHANNEL_SUSPEND_SECONDS_FOR_429", "CHANNEL_SUSPEND_SECONDS_FOR_5XX"} {
				t.Setenv(name, "")
				require.NoError(t, os.Unsetenv(name))
			}
			only := retryOrderChannel(9321, "rotated", 0)
			only.Key = "old-fixture-key"
			setupRetryOrderDB(t, []*dbmodel.Channel{only})
			previousRetries := config.RetryTimes
			config.RetryTimes = 0
			t.Cleanup(func() { config.RetryTimes = previousRetries })

			stale, err := dbmodel.GetChannelById(only.Id, true)
			require.NoError(t, err)
			rotated, err := dbmodel.GetChannelById(only.Id, true)
			require.NoError(t, err)
			rotated.Key = "new-fixture-key"
			rotated.TestingModel = nil
			require.NoError(t, rotated.Update())
			routed := stale
			if !tc.stale {
				routed, err = dbmodel.GetChannelById(only.Id, true)
				require.NoError(t, err)
			}
			time.Sleep(time.Millisecond) // dispatch strictly after the change on a coarse clock

			relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
				c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
				return &model.ErrorWithStatusCode{StatusCode: http.StatusTooManyRequests, Error: model.Error{
					Message: "rate limit exceeded", Type: model.ErrorTypeRateLimit, Code: "rate_limit_exceeded", FromUpstream: true,
				}}
			}
			t.Cleanup(func() { relayHelperForTest = nil })
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, logger.Logger)
			body, err := json.Marshal(map[string]any{"model": retryOrderModel, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
			require.NoError(t, err)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Set(ctxkey.Id, 1)
			c.Set(ctxkey.TokenId, 2)
			c.Set(ctxkey.Group, retryOrderGroup)
			c.Set(ctxkey.RequestModel, retryOrderModel)
			middleware.SetupContextForSelectedChannel(c, routed, retryOrderModel)

			Relay(c)
			drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, graceful.Drain(drainCtx))
			require.Equal(t, tc.wantStreak, routingModelAbility(t, only.Id).CooldownFailures)
		})
	}
}
