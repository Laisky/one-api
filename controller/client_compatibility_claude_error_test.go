package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const clientCompatibilityErrorModel = "claude-client-error-fixture"
const clientCompatibilitySuccessBody = `{"id":"msg_success","type":"message","role":"assistant","model":"claude-client-error-fixture","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`

// clientCompatibilityErrorAttempt describes one synthetic provider response.
// Parameters: none. Returns: this data-only fixture carries status and wire JSON.
type clientCompatibilityErrorAttempt struct {
	status          int
	body, requestID string
	channelType     int
	local           bool
	mismatch        bool
}

// clientCompatibilityRunFinalClaudeError drives the real outer retry decision
// and native HTTP helper using isolated SQLite and synthetic provider responses.
// Parameters: t owns cleanup and attempts defines the initial/fallback channels.
// Returns: downstream HTTP bytes and the physical provider dispatch count.
func clientCompatibilityRunFinalClaudeError(t *testing.T, attempts []clientCompatibilityErrorAttempt) (*httptest.ResponseRecorder, int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := int(calls.Add(1)) - 1
		if index >= len(attempts) {
			http.Error(w, "unexpected additional attempt", 500)
			return
		}
		a := attempts[index]
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", a.requestID)
		w.Header().Set("retry-after", fmt.Sprint(index+2))
		w.Header().Set("anthropic-ratelimit-requests-remaining", fmt.Sprint(index+8))
		w.Header().Set("Set-Cookie", "provider=synthetic")
		w.Header().Set("Authorization", "Bearer synthetic-provider-response")
		w.Header().Set("X-Upstream-Private", "synthetic-private-header")
		w.WriteHeader(a.status)
		_, _ = io.WriteString(w, a.body)
	}))
	t.Cleanup(server.Close)
	oldClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = oldClient })
	channels := make([]*dbmodel.Channel, 0, len(attempts))
	for i, a := range attempts {
		priority := int64(10 - i)
		kind := a.channelType
		if kind == 0 {
			kind = channeltype.Anthropic
		}
		base := server.URL
		ch := &dbmodel.Channel{Id: 8100 + i, Name: fmt.Sprintf("native-error-%d", i), Type: kind, Status: dbmodel.ChannelStatusEnabled, Models: clientCompatibilityErrorModel, Group: "default", Key: "synthetic-provider-key", BaseURL: &base, Priority: &priority}
		require.NoError(t, ch.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{clientCompatibilityErrorModel: {Ratio: 1, CompletionRatio: 1}}))
		channels = append(channels, ch)
	}
	setupRetryOrderDB(t, channels)
	require.NoError(t, dbmodel.DB.AutoMigrate(&dbmodel.User{}, &dbmodel.Token{}, &dbmodel.UserRequestCost{}, &dbmodel.QuotaRefund{}, &dbmodel.Log{}, &dbmodel.Trace{}, &dbmodel.MCPServer{}, &dbmodel.MCPTool{}))
	oldLog := dbmodel.LOG_DB
	dbmodel.LOG_DB = dbmodel.DB
	t.Cleanup(func() { dbmodel.LOG_DB = oldLog })
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldBatch, oldMemory, oldRetries := config.BatchUpdateEnabled, config.MemoryCacheEnabled, config.RetryTimes
	config.BatchUpdateEnabled = false
	config.MemoryCacheEnabled = false
	config.RetryTimes = len(attempts) - 1
	oldLogEnabled := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() {
		config.BatchUpdateEnabled = oldBatch
		config.MemoryCacheEnabled = oldMemory
		config.RetryTimes = oldRetries
		config.SetLogConsumeEnabled(oldLogEnabled)
	})
	require.NoError(t, dbmodel.DB.Create(&dbmodel.User{Id: 1, Username: "compatibility", Status: dbmodel.UserStatusEnabled, Quota: 100000, Group: "default"}).Error)
	require.NoError(t, dbmodel.DB.Create(&dbmodel.Token{Id: 2, UserId: 1, Key: "synthetic-gateway-key", Status: dbmodel.TokenStatusEnabled, RemainQuota: 100000, ExpiredTime: -1}).Error)
	oldProcess := processChannelRelayErrorForTest
	processChannelRelayErrorForTest = func(context.Context, processChannelRelayErrorParams) {}
	t.Cleanup(func() { processChannelRelayErrorForTest = oldProcess })
	oldHelper := relayHelperForTest
	relayHelperForTest = func(c *gin.Context, _ int) *relaymodel.ErrorWithStatusCode {
		index := c.GetInt(ctxkey.ChannelId) - 8100
		if attempts[index].mismatch {
			return &relaymodel.ErrorWithStatusCode{StatusCode: 400, Error: relaymodel.Error{Type: relaymodel.ErrorTypeOneAPI, Code: "unsupported_model_transport", RawError: adaptor.ErrModelRequiresLiveTransport}}
		}
		if attempts[index].local {
			return &relaymodel.ErrorWithStatusCode{StatusCode: 400, Error: relaymodel.Error{Type: relaymodel.ErrorTypeOneAPI, Code: "fixture_local_error", Message: "local rejection"}}
		}
		return rcontroller.RelayClaudeMessagesHelper(c)
	}
	t.Cleanup(func() { relayHelperForTest = oldHelper })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"`+clientCompatibilityErrorModel+`","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-api-key", "synthetic-gateway-key")
	for key, value := range map[string]any{ctxkey.Id: 1, ctxkey.TokenId: 2, ctxkey.TokenName: "fixture", ctxkey.Group: "default", ctxkey.RequestId: "req_native_final_compatibility", ctxkey.TokenQuota: int64(100000), ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 1, Quota: 100000}, ctxkey.Username: "compatibility", ctxkey.RequestModel: clientCompatibilityErrorModel} {
		c.Set(key, value)
	}
	middleware.SetupContextForSelectedChannel(c, channels[0], clientCompatibilityErrorModel)
	Relay(c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
	return w, calls.Load()
}

// TestClientCompatibilityClaudeFinalNativeError preserves the provider's native
// envelope only when the outer relay selects that error as the final outcome.
// Parameters: t owns 400/429/529 loopbacks. Returns: nothing; native bodies and
// selected headers survive without forwarding cookies or authorization headers.
func TestClientCompatibilityClaudeFinalNativeError(t *testing.T) {
	for _, status := range []int{400, 429, 529} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := fmt.Sprintf(`{"type":"error","error":{"type":"fixture_error","message":"native failure %d","future_number":9007199254740993},"request_id":"req_native_body","future_root":true}`, status)
			w, calls := clientCompatibilityRunFinalClaudeError(t, []clientCompatibilityErrorAttempt{{status: status, body: body, requestID: "req_native_header"}})
			require.EqualValues(t, 1, calls)
			require.Equal(t, status, w.Code)
			require.Empty(t, w.Header().Get("Set-Cookie"))
			require.Empty(t, w.Header().Get("Authorization"))
			require.Empty(t, w.Header().Get("X-Upstream-Private"))
			require.Equal(t, body, w.Body.String(), "final native error must retain its exact envelope and opaque fields")
			require.Equal(t, "req_native_header", w.Header().Get("request-id"))
			require.Equal(t, "2", w.Header().Get("retry-after"))
			require.Equal(t, "8", w.Header().Get("anthropic-ratelimit-requests-remaining"))
		})
	}
}

// TestClientCompatibilityClaudeRetryDiscardsStaleNativeError proves native error
// capture cannot commit output early or contaminate later attempts. Parameters:
// t owns retry fixtures. Returns: nothing; success and generic local/converted
// errors retain their own wire semantics and never inherit old native headers.
func TestClientCompatibilityClaudeRetryDiscardsStaleNativeError(t *testing.T) {
	first := clientCompatibilityErrorAttempt{status: 429, body: `{"type":"error","error":{"type":"rate_limit_error","message":"first error"},"request_id":"req_first_body"}`, requestID: "req_first_header"}
	cases := []struct {
		name          string
		last          clientCompatibilityErrorAttempt
		providerCalls int32
	}{
		{"success", clientCompatibilityErrorAttempt{status: 200, body: clientCompatibilitySuccessBody, requestID: "req_success_header"}, 2},
		{"new_native_error", clientCompatibilityErrorAttempt{status: 529, body: `{"type":"error","error":{"type":"overloaded_error","message":"second native"},"request_id":"req_second_body"}`, requestID: "req_second_header"}, 2},
		{"new_local_error", clientCompatibilityErrorAttempt{local: true}, 1},
		{"transport_mismatch_exhaustion", clientCompatibilityErrorAttempt{mismatch: true}, 1},
		{"new_converted_error", clientCompatibilityErrorAttempt{status: 400, body: `{"error":{"type":"invalid_request_error","message":"converted rejection","code":"converted_fixture"}}`, requestID: "req_converted_header", channelType: channeltype.OpenAI}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, calls := clientCompatibilityRunFinalClaudeError(t, []clientCompatibilityErrorAttempt{first, tc.last})
			require.Equal(t, tc.providerCalls, calls, "native error must not prevent eligible retries")
			if tc.name != "transport_mismatch_exhaustion" {
				require.NotContains(t, w.Body.String(), "first error")
				require.NotContains(t, w.Body.String(), "req_first_body")
				require.NotEqual(t, "req_first_header", w.Header().Get("request-id"))
			}
			require.Empty(t, w.Header().Get("Set-Cookie"))
			require.Empty(t, w.Header().Get("Authorization"))
			switch tc.name {
			case "success":
				require.Equal(t, 200, w.Code)
				require.JSONEq(t, clientCompatibilitySuccessBody, w.Body.String())
			case "new_native_error":
				require.Equal(t, 529, w.Code)
				require.Equal(t, tc.last.body, w.Body.String())
				require.Equal(t, "req_second_header", w.Header().Get("request-id"))
				require.Equal(t, "3", w.Header().Get("retry-after"))
			case "transport_mismatch_exhaustion":
				require.Equal(t, 429, w.Code)
				require.Equal(t, first.body, w.Body.String())
				require.Equal(t, "req_first_header", w.Header().Get("request-id"))
				require.Equal(t, "2", w.Header().Get("retry-after"))
			case "new_local_error":
				require.Equal(t, 400, w.Code)
				require.Contains(t, w.Body.String(), "fixture_local_error")
				require.NotContains(t, w.Body.String(), `"type":"error"`)
				require.Empty(t, w.Header().Get("request-id"))
				require.Empty(t, w.Header().Get("retry-after"))
			case "new_converted_error":
				require.Equal(t, 400, w.Code)
				require.Contains(t, w.Body.String(), "converted_fixture")
				require.NotContains(t, w.Body.String(), `"type":"error"`)
				require.Empty(t, w.Header().Get("retry-after"))
			}
		})
	}
}
