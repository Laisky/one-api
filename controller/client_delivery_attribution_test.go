package controller

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/baidu"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// failedClientDelivery simulates a client write failure after headers were committed.
type failedClientDelivery struct {
	gin.ResponseWriter
	cause error
}

// Write commits headers and returns the configured client delivery error.
func (w failedClientDelivery) Write([]byte) (int, error) { w.WriteHeaderNow(); return 0, w.cause }

// TestClientDeliveryFailureAttribution checks the actual adapter error, committed response, and health classification.
func TestClientDeliveryFailureAttribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	cause := stderrors.New("delivery failed")
	c.Writer = failedClientDelivery{c.Writer, cause}
	failure, usage := baidu.Handler(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"id","result":"hello","usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`))})
	require.NotNil(t, failure)
	require.ErrorIs(t, failure.RawError, cause)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.TotalTokens)
	require.True(t, c.Writer.Written())
	require.False(t, rcontroller.BillingAllowsRetry(c))
	require.False(t, writeRelayFinalError(c, failure))
	require.False(t, countsAgainstChannelHealth(failure))
	require.Equal(t, errkind.Server, relayFailureKind(failure))
}

// TestClientDeliveryCodesRemainLocal verifies delivery codes and distinguishes provider-origin failures.
func TestClientDeliveryCodesRemainLocal(t *testing.T) {
	for _, code := range []string{"write_response_body_failed", "write_embedding_response", "write_embedding_response_body_failed", "write_response_failed", "write_stream_failed", "write_mcp_response_failed", "write_converted_response_failed", "write_tool_search_response_failed"} {
		for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway} {
			failure := openai.ErrorWrapper(stderrors.New("write failure"), code, status)
			require.Equal(t, errkind.Server, relayFailureKind(failure), code)
			require.False(t, countsAgainstChannelHealth(failure), code)
		}
	}
	provider := &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{Code: "write_response_body_failed", Type: "provider_error"}}
	require.Equal(t, errkind.Upstream, relayFailureKind(provider))
	require.True(t, countsAgainstChannelHealth(provider))
	readFailure := openai.ErrorWrapper(stderrors.New("read failure"), "read_response_body_failed", http.StatusBadGateway)
	require.Equal(t, errkind.Upstream, relayFailureKind(readFailure))
	marked := openai.ErrorWrapper(errkind.Mark(stderrors.New("marked"), errkind.InvalidRequest), "write_response_body_failed", http.StatusInternalServerError)
	require.Equal(t, errkind.InvalidRequest, relayFailureKind(marked))
}

// TestRelayClientDeliveryFailureDoesNotReplay exercises the actual retry loop after a provider receipt and committed failed write.
func TestRelayClientDeliveryFailureDoesNotReplay(t *testing.T) {
	initial := retryOrderChannel(1, "first", 10)
	second := retryOrderChannel(2, "second", 5)
	setupRetryOrderDB(t, []*dbmodel.Channel{initial, second})
	previousRetries := config.RetryTimes
	config.RetryTimes = 3
	t.Cleanup(func() { config.RetryTimes = previousRetries })
	processChannelRelayErrorForTest = func(context.Context, processChannelRelayErrorParams) {}
	t.Cleanup(func() { processChannelRelayErrorForTest = nil })
	calls := 0
	relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
		calls++
		failure, usage := baidu.Handler(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"id","result":"hello","usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`))})
		require.NotNil(t, usage)
		require.Equal(t, 10, usage.TotalTokens)
		return failure
	}
	t.Cleanup(func() { relayHelperForTest = nil })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	c.Writer = failedClientDelivery{c.Writer, stderrors.New("client write failed")}
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 2)
	c.Set(ctxkey.Group, retryOrderGroup)
	c.Set(ctxkey.RequestModel, retryOrderModel)
	middleware.SetupContextForSelectedChannel(c, initial, retryOrderModel)
	Relay(c)
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(drain))
	require.Equal(t, 1, calls)
	require.True(t, c.Writer.Written())
}
