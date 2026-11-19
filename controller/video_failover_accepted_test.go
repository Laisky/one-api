package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/model"
)

// videoAttemptHook scripts one paid video-creation attempt with access to the
// request context, so a scenario can record provider acceptance markers before
// returning the attempt's failure. A nil return means the attempt succeeded.
type videoAttemptHook func(c *gin.Context, channelID int) *model.ErrorWithStatusCode

// runVideoFailoverWithHook drives the real controller.Relay retry loop for a
// paid video POST whose attempts are scripted by hook. Parameters: t is the test
// handle, initial the distributor-selected channel, requestPath the video
// creation route, memoryCache the routing implementation, retryTimes the retry
// budget and hook the per-attempt script. It returns the attempted channel ids
// in order and the HTTP status written to the client.
func runVideoFailoverWithHook(t *testing.T, initial *dbmodel.Channel, requestPath string, memoryCache bool, retryTimes int, hook videoAttemptHook) ([]int, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalRetryTimes, originalMemoryCache := config.RetryTimes, config.MemoryCacheEnabled
	config.RetryTimes, config.MemoryCacheEnabled = retryTimes, memoryCache
	t.Cleanup(func() { config.RetryTimes, config.MemoryCacheEnabled = originalRetryTimes, originalMemoryCache })

	processChannelRelayErrorForTest = func(context.Context, processChannelRelayErrorParams) {}
	t.Cleanup(func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		processChannelRelayErrorForTest = nil
	})

	var order []int
	relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
		id := c.GetInt(ctxkey.ChannelId)
		order = append(order, id)
		// Every scripted attempt represents a dispatched paid creation.
		c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
		return hook(c, id)
	}
	t.Cleanup(func() { relayHelperForTest = nil })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(map[string]any{"model": retryOrderModel, "prompt": "hi", "duration": 5})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, requestPath, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 2)
	c.Set(ctxkey.Group, retryOrderGroup)
	c.Set(ctxkey.RequestModel, retryOrderModel)
	middleware.SetupContextForSelectedChannel(c, initial, retryOrderModel)

	Relay(c)

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(drainCtx))
	return order, recorder.Code
}

// TestVideoFailover_AcceptedJobNeverReplays pins the acceptance boundary: once
// the provider accepted the paid job (AsyncVideoAcceptedKey) or one-api bound a
// durable task (DurableTaskKey), even a provider-attributed 429 that follows
// must not create a second job on another channel.
func TestVideoFailover_AcceptedJobNeverReplays(t *testing.T) {
	markers := []struct {
		name string
		mark func(c *gin.Context)
	}{
		{name: "async_video_accepted", mark: func(c *gin.Context) { c.Set(adaptor.AsyncVideoAcceptedKey, true) }},
		{name: "durable_task_bound", mark: func(c *gin.Context) { c.Set(asyncvideo.DurableTaskKey, "task-primary") }},
	}
	for _, route := range videoFailoverRoutes {
		for _, marker := range markers {
			for _, path := range routingPaths {
				name := strings.ReplaceAll(route.path, "/", "_") + "/" + marker.name + "/" + path.name
				t.Run(name, func(t *testing.T) {
					pool := videoFailoverPool(route.channelType)
					setupRetryOrderDB(t, pool)

					order, status := runVideoFailoverWithHook(t, pool[0], route.path, path.memoryCache, videoFailoverRetryTime,
						func(c *gin.Context, channelID int) *model.ErrorWithStatusCode {
							if channelID != videoFailoverPrimary {
								return nil
							}
							marker.mark(c)
							return issue625QuotaRejection(channelID)
						})

					require.Equal(t, []int{videoFailoverPrimary}, order,
						"an accepted job must never be replayed, whatever error follows")
					require.NotEqual(t, http.StatusOK, status)
				})
			}
		}
	}
}

// TestVideoFailover_RejectionWithoutForwardingStillRetries is the positive
// control for the unchanged pre-dispatch path: when the creation never left
// one-api, any retryable failure may use the backup channel.
func TestVideoFailover_RejectionWithoutForwardingStillRetries(t *testing.T) {
	for _, route := range videoFailoverRoutes {
		for _, path := range routingPaths {
			t.Run(strings.ReplaceAll(route.path, "/", "_")+"/"+path.name, func(t *testing.T) {
				pool := videoFailoverPool(route.channelType)
				setupRetryOrderDB(t, pool)

				order, status := runVideoFailoverWithHook(t, pool[0], route.path, path.memoryCache, videoFailoverRetryTime,
					func(c *gin.Context, channelID int) *model.ErrorWithStatusCode {
						if channelID != videoFailoverPrimary {
							return nil
						}
						c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
						return serverError(channelID)
					})

				require.Equal(t, []int{videoFailoverPrimary, videoFailoverBackup}, order)
				require.Equal(t, http.StatusOK, status)
			})
		}
	}
}
