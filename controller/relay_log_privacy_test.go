package controller

import (
	"context"
	"fmt"
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

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/relay/model"
)

// TestRelayRawBodyPreviewRemainsPrivate exercises the real debug decision path:
// an upstream parser's existing raw-body marker must protect its short preview
// while retaining the response and useful status/error-code diagnostics.
func TestRelayRawBodyPreviewRemainsPrivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDebug, oldRetry := config.DebugEnabled, config.RetryTimes
	oldHelper, oldProcess := relayHelperForTest, processChannelRelayErrorForTest
	config.DebugEnabled, config.RetryTimes = true, 3
	t.Cleanup(func() {
		config.DebugEnabled, config.RetryTimes = oldDebug, oldRetry
		relayHelperForTest, processChannelRelayErrorForTest = oldHelper, oldProcess
	})
	processChannelRelayErrorForTest = func(context.Context, processChannelRelayErrorParams) {}
	const canary = "sentinel-private-upstream-body"
	relayHelperForTest = func(*gin.Context, int) *model.ErrorWithStatusCode {
		return &model.ErrorWithStatusCode{StatusCode: http.StatusBadRequest, Error: model.Error{
			Message: canary, MessageContainsRawBody: true, Type: model.ErrorType("invalid_request_error"), Code: "invalid_request",
		}}
	}
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.New(glog.WithName("raw-body-preview"), glog.WithLevel(glog.LevelDebug),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	gmw.SetLogger(c, lg)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", http.NoBody)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 73)
	c.Set(ctxkey.ChannelId, 5)
	c.Set(ctxkey.ChannelName, "openai")
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.RequestModel, "gpt-transcribe")
	c.Set(ctxkey.RequestId, "synthetic-private-preview")
	Relay(c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), canary, "caller response remains unchanged")
	decisions := observed.FilterMessage("non-retry relay decision details").All()
	require.Len(t, decisions, 1)
	require.EqualValues(t, 400, decisions[0].ContextMap()["status_code"])
	require.Equal(t, "invalid_request", decisions[0].ContextMap()["error_code"])
	require.Equal(t, "[REDACTED]", decisions[0].ContextMap()["error_message_preview"])
	for _, entry := range observed.All() {
		require.NotContains(t, entry.Message, canary)
		for _, value := range entry.ContextMap() {
			require.NotContains(t, fmt.Sprint(value), canary)
		}
	}
}
