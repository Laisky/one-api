package controller

import (
	"context"
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

// nativeSettledRelayChannel builds a native Responses OpenAI channel routed to
// DeepSeek's Responses endpoint with a funded web_search tariff and a
// deterministic token tariff; key distinguishes the dispatch destination.
func nativeSettledRelayChannel(t *testing.T, id int, key, modelName string, priority int64) *dbmodel.Channel {
	t.Helper()
	base := "https://api.openai.com"
	pricing := `{"` + modelName + `":{"ratio":1,"completion_ratio":1}}`
	ch := &dbmodel.Channel{Id: id, Name: key, Type: channeltype.OpenAI, Status: dbmodel.ChannelStatusEnabled,
		Models: modelName, Group: "default", Priority: &priority, BaseURL: &base, ModelConfigs: &pricing, Key: key,
		Config: `{"endpoint_urls":{"` + channeltype.RelayModeToEndpointName(relaymode.ResponseAPI) + `":"https://api.deepseek.com/v1/responses"}}`}
	require.NoError(t, ch.SetToolingConfig(&dbmodel.ChannelToolingConfig{Pricing: map[string]dbmodel.ToolPricingLocal{"web_search": {QuotaPerCall: 17}}}))
	return ch
}

// TestRelayNativeResponseMissingReceiptAnswersError drives the real relay
// loop, the registered OpenAI adaptor, the native Responses helper and SQLite
// billing. An undecodable DeepSeek reply after admitted paid tools retains the
// funded tool floor; the failure must reach the failure funnel without
// penalising the channel, must not be replayed on the alternative channel, and
// must be answered with an error instead of an empty HTTP 200.
func TestRelayNativeResponseMissingReceiptAnswersError(t *testing.T) {
	const modelName = "deepseek-v4-pro"
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	first := nativeSettledRelayChannel(t, 92103, "first-native-key", modelName, 10)
	second := nativeSettledRelayChannel(t, 92104, "second-native-key", modelName, 5)
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
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"id":"truncated","object":"response","status":"comp`)), Request: r}, nil
	})}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	gmw.SetLogger(c, lg)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-v4-pro","input":"hello","tools":[{"type":"web_search"}],"tool_choice":"auto"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	const requestID = "native-settled-deepseek-missing-receipt"
	for key, value := range map[string]any{ctxkey.Id: 92001, ctxkey.TokenId: 92002, ctxkey.TokenName: "incident-fixture-token", ctxkey.Group: "default", ctxkey.RequestModel: modelName, ctxkey.RequestId: requestID, ctxkey.TokenQuota: incidentBodyResetBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 92001, Quota: incidentBodyResetBalance}, ctxkey.Username: "incident-fixture-owner"} {
		c.Set(key, value)
	}
	middleware.SetupContextForSelectedChannel(c, first, modelName)
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
	require.Equal(t, "unmarshal_response_body_failed", failure.Code)
	require.Equal(t, errkind.Upstream, relayFailureKind(&failure))
	require.False(t, countsAgainstChannelHealth(&failure), "a gateway-side decode failure must not penalise the channel")
	final := logs.FilterMessage("relay request completed").All()
	require.Len(t, final, 1)
	require.Equal(t, "helper_failed", final[0].ContextMap()["relay_outcome"])
	require.Equal(t, "error_response_written", final[0].ContextMap()["client_outcome"])
	require.Equal(t, http.StatusInternalServerError, recorder.Code, "never an empty success")
	require.Contains(t, recorder.Body.String(), "unmarshal_response_body_failed")

	// The funded paid-tool floor is settled exactly once.
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
	require.Equal(t, "deepseek_tool_response_missing_usage", consume[0].Metadata["billing_estimate_reason"])
	require.Greater(t, cost.Quota, int64(170), "the funded paid-tool floor is retained")
}
