package controller

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// nativeSettledRelayWriter commits headers and then fails every body write,
// like a downstream connection that broke after the response began.
type nativeSettledRelayWriter struct {
	gin.ResponseWriter
	cause error
}

// Write commits the status line and reports the configured delivery failure.
func (w nativeSettledRelayWriter) Write([]byte) (int, error) {
	w.WriteHeaderNow()
	return 0, w.cause
}

// WriteString follows the same failed delivery boundary as Write.
func (w nativeSettledRelayWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// nativeSettledRelayChannel builds a native Responses OpenAI channel with a
// deterministic tariff; key distinguishes the dispatch destination.
func nativeSettledRelayChannel(t *testing.T, id int, key, modelName string, priority int64, deepSeekTool bool) *dbmodel.Channel {
	t.Helper()
	base := "https://api.openai.com"
	pricing := `{"` + modelName + `":{"ratio":1,"completion_ratio":1}}`
	ch := &dbmodel.Channel{Id: id, Name: key, Type: channeltype.OpenAI, Status: dbmodel.ChannelStatusEnabled,
		Models: modelName, Group: "default", Priority: &priority, BaseURL: &base, ModelConfigs: &pricing, Key: key}
	if deepSeekTool {
		config := `{"endpoint_urls":{"` + channeltype.RelayModeToEndpointName(relaymode.ResponseAPI) + `":"https://api.deepseek.com/v1/responses"}}`
		ch.Config = config
		require.NoError(t, ch.SetToolingConfig(&dbmodel.ChannelToolingConfig{Pricing: map[string]dbmodel.ToolPricingLocal{"web_search": {QuotaPerCall: 17}}}))
	}
	return ch
}

// TestRelayNativeResponseSettledFailure drives the real relay loop, the
// registered OpenAI adaptor, the native Responses helper and SQLite billing.
// A failure observed after the provider receipt must be settled once, reported
// to the failure funnel without penalising the channel, never replayed on the
// alternative channel, and answered with an error when nothing was delivered.
func TestRelayNativeResponseSettledFailure(t *testing.T) {
	const streamReceipt = "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp-relay","object":"response","status":"completed","model":"gpt-4o","output":[{"type":"message","id":"msg-relay","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}}` +
		"\n\n"
	const jsonReceipt = `{"id":"resp-relay","object":"response","status":"completed","model":"gpt-4o","output":[{"type":"message","id":"msg-relay","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`
	for _, tc := range []struct {
		name, model, request, contentType, providerBody, code string
		// delivered reports whether downstream output was committed before the failure.
		delivered, deepSeekTool bool
		kind                    errkind.Kind
	}{
		{name: "stream_delivery", model: "gpt-4o", request: `{"model":"gpt-4o","input":"hello","max_output_tokens":16,"stream":true}`,
			contentType: "text/event-stream", providerBody: streamReceipt, code: "write_stream_failed", delivered: true, kind: errkind.Server},
		{name: "json_delivery", model: "gpt-4o", request: `{"model":"gpt-4o","input":"hello","max_output_tokens":16}`,
			contentType: "application/json", providerBody: jsonReceipt, code: "write_response_body_failed", delivered: true, kind: errkind.Server},
		{name: "deepseek_missing_receipt", model: "deepseek-v4-pro", request: `{"model":"deepseek-v4-pro","input":"hello","tools":[{"type":"web_search"}],"tool_choice":"auto"}`,
			contentType: "application/json", providerBody: `{"id":"truncated","object":"response","status":"comp`, code: "unmarshal_response_body_failed", deepSeekTool: true, kind: errkind.Upstream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousGinMode := gin.Mode()
			gin.SetMode(gin.TestMode)
			t.Cleanup(func() { gin.SetMode(previousGinMode) })
			first := nativeSettledRelayChannel(t, 92103, "first-native-key", tc.model, 10, tc.deepSeekTool)
			second := nativeSettledRelayChannel(t, 92104, "second-native-key", tc.model, 5, tc.deepSeekTool)
			incidentBodyResetRoutingFixture(t, []*dbmodel.Channel{first, second}, false)
			var processed []processChannelRelayErrorParams
			var processedMu sync.Mutex
			processChannelRelayErrorForTest = func(_ context.Context, params processChannelRelayErrorParams) {
				processedMu.Lock()
				defer processedMu.Unlock()
				processed = append(processed, params)
			}
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.New(glog.WithName("native-settled"), glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
			require.NoError(t, err)
			var firstCalls, secondCalls atomic.Int32
			client.HTTPClient = &http.Client{Transport: incidentBodyResetRoundTripper(func(r *http.Request) (*http.Response, error) {
				_, err := io.Copy(io.Discard, r.Body)
				require.NoError(t, err)
				switch r.Header.Get("Authorization") {
				case "Bearer first-native-key":
					firstCalls.Add(1)
				case "Bearer second-native-key":
					secondCalls.Add(1)
				default:
					t.Errorf("unexpected fixture credential for %s", r.URL)
				}
				require.Equal(t, "/v1/responses", r.URL.Path)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{tc.contentType}},
					Body: io.NopCloser(strings.NewReader(tc.providerBody)), Request: r}, nil
			})}

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, lg)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tc.request))
			c.Request.Header.Set("Content-Type", "application/json")
			cause := stderrors.New("synthetic downstream delivery failure")
			if tc.delivered {
				c.Writer = nativeSettledRelayWriter{ResponseWriter: c.Writer, cause: cause}
			}
			requestID := "native-settled-" + tc.name
			for key, value := range map[string]any{ctxkey.Id: 92001, ctxkey.TokenId: 92002, ctxkey.TokenName: "incident-fixture-token", ctxkey.Group: "default", ctxkey.RequestModel: tc.model, ctxkey.RequestId: requestID, ctxkey.TokenQuota: incidentBodyResetBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 92001, Quota: incidentBodyResetBalance}, ctxkey.Username: "incident-fixture-owner"} {
				c.Set(key, value)
			}
			middleware.SetupContextForSelectedChannel(c, first, tc.model)
			Relay(c)
			incidentBodyResetDrain(t)

			require.EqualValues(t, 1, firstCalls.Load(), "exactly one provider dispatch")
			require.Zero(t, secondCalls.Load(), "a settled attempt must not be replayed on the alternative channel")

			// The failure reaches the relay failure funnel exactly once and is not
			// evidence against the channel.
			processedMu.Lock()
			require.Len(t, processed, 1, "the failed attempt must be reported, not recorded as success")
			failure := processed[0].Err
			processedMu.Unlock()
			require.Equal(t, tc.code, failure.Code)
			require.Equal(t, tc.kind, relayFailureKind(&failure))
			require.False(t, countsAgainstChannelHealth(&failure), "a gateway-side or delivery failure must not penalise the channel")
			if tc.delivered {
				require.ErrorIs(t, failure.RawError, cause)
			}
			final := logs.FilterMessage("relay request completed").All()
			require.Len(t, final, 1)
			require.Equal(t, "helper_failed", final[0].ContextMap()["relay_outcome"])

			// The client either keeps its committed response or receives an error;
			// never an empty success.
			if tc.delivered {
				require.Equal(t, "committed_response_preserved", final[0].ContextMap()["client_outcome"])
				require.NotContains(t, recorder.Body.String(), tc.code, "a committed response must not be followed by error JSON")
			} else {
				require.Equal(t, "error_response_written", final[0].ContextMap()["client_outcome"])
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
				require.Contains(t, recorder.Body.String(), tc.code)
			}

			// The provider receipt (or the funded tool floor) is settled exactly once.
			var u dbmodel.User
			var token dbmodel.Token
			require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
			require.NoError(t, dbmodel.DB.First(&token, 92002).Error)
			require.Equal(t, u.Quota, token.RemainQuota)
			var cost dbmodel.UserRequestCost
			require.NoError(t, dbmodel.DB.Where("request_id = ?", requestID).First(&cost).Error)
			require.Equal(t, incidentBodyResetBalance-cost.Quota, u.Quota, "owner balance reconciles to the single request cost")
			var consume []dbmodel.Log
			require.NoError(t, dbmodel.LOG_DB.Where("request_id = ? AND type = ?", requestID, dbmodel.LogTypeConsume).Find(&consume).Error)
			require.Len(t, consume, 1, "settlement consumes exactly once")
			require.EqualValues(t, cost.Quota, consume[0].Quota)
			if tc.deepSeekTool {
				require.Equal(t, "deepseek_tool_response_missing_usage", consume[0].Metadata["billing_estimate_reason"])
				require.Greater(t, cost.Quota, int64(170), "the funded paid-tool floor is retained")
			} else {
				require.EqualValues(t, 18, cost.Quota, "the measured receipt is billed once")
			}
		})
	}
}
