package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const imageSettledReplayModel = "dall-e-3"

// imageSettledReplayUnitQuota is the explicit channel tariff (0.002 USD per
// image at 500,000 quota per USD and group ratio 1) for one rendered image.
const imageSettledReplayUnitQuota = int64(1000)

const imageSettledReplayMalformedBody = `{"created":1,"data":[`
const imageSettledReplayValidBody = `{"created":1,"data":[{"url":"https://image.fixture.invalid/render.png"}]}`

// imageSettledReplayChannel constructs one synthetic OpenAI image channel whose
// explicit per-image tariff makes every rendered image cost exactly one unit.
func imageSettledReplayChannel(id int, host string, priority int64) *dbmodel.Channel {
	base := "https://" + host
	pricing := `{"dall-e-3":{"ratio":0,"image":{"price_per_image_usd":0.002}}}`
	return &dbmodel.Channel{Id: id, Name: host, Type: channeltype.OpenAI,
		Status: dbmodel.ChannelStatusEnabled, Models: imageSettledReplayModel, Group: "default",
		Priority: &priority, BaseURL: &base, ModelConfigs: &pricing, Key: "synthetic-image-key-" + host}
}

// imageSettledReplayObservation captures every externally visible effect of
// one relayed image request: upstream dispatches, client response, and ledger.
type imageSettledReplayObservation struct {
	firstCalls, secondCalls int32
	firstHold               int64
	status                  int
	body                    string
	userQuota, tokenRemain  int64
	userUsedQuota           int64
	userRequestCount        int
	firstChannelUsed        int64
	secondChannelUsed       int64
	requestCost             int64
	requestCostRows         int
	consumeSum              int64
	provisionalRows         int
	logs                    []dbmodel.Log
	retryRefundLogs         int
	reconcileSequence       []string
}

// TestImageSettledAttemptIsNotReplayed drives the real Relay retry loop, the
// registered OpenAI image adaptor, and SQLite billing when a 2xx upstream image
// response cannot be decoded. One-api settles every 2xx dispatch as paid provider
// work, so that attempt must not be replayed on an alternative channel (the
// replay used to refund the settled charge as an abandoned hold, billing two
// generations as one), and the balance, consume logs, request cost, and usage
// counters must agree.
func TestImageSettledAttemptIsNotReplayed(t *testing.T) {
	scenarios := []string{"alternative_channel", "no_alternative", "initial_success", "alternative_initial_success", "alternative_after_http_error"}
	for _, memory := range []bool{false, true} {
		for _, scenario := range scenarios {
			name := scenario + "/database"
			if memory {
				name = scenario + "/memory"
			}
			t.Run(name, func(t *testing.T) {
				imageSettledReplayScenario(t, scenario, memory)
			})
		}
	}
}

// imageSettledReplayScenario runs one scenario under one routing mode and
// asserts the policy-neutral billing contract. Parameters: scenario selects the
// channel pool and the first upstream body; memory selects cache routing.
func imageSettledReplayScenario(t *testing.T, scenario string, memory bool) {
	t.Helper()
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	first := imageSettledReplayChannel(92103, "first.image.invalid", 10)
	channels := []*dbmodel.Channel{first}
	if strings.HasPrefix(scenario, "alternative_") {
		channels = append(channels, imageSettledReplayChannel(92104, "second.image.invalid", 5))
	}
	incidentBodyResetRoutingFixture(t, channels, memory)
	obs := imageSettledReplayRun(t, scenario, first)
	t.Logf("scenario=%s dispatch first=%d second=%d status=%d debit=%d consume_sum=%d request_cost=%d used=%d channel_used=%d+%d reconcile=%v",
		scenario, obs.firstCalls, obs.secondCalls, obs.status, incidentBodyResetBalance-obs.userQuota, obs.consumeSum,
		obs.requestCost, obs.userUsedQuota, obs.firstChannelUsed, obs.secondChannelUsed, obs.reconcileSequence)

	require.EqualValues(t, 1, obs.firstCalls)
	require.Equal(t, imageSettledReplayUnitQuota, obs.firstHold, "one image reserves exactly one unit")
	switch scenario {
	case "alternative_channel", "no_alternative":
		// The settled 2xx attempt is final: the client receives the decode
		// failure and no alternative channel is dispatched or refunded.
		require.Zero(t, obs.secondCalls, "a settled image attempt must not be replayed")
		require.Zero(t, obs.retryRefundLogs, "a settled charge must not be refunded as an abandoned hold")
		require.Equal(t, http.StatusInternalServerError, obs.status)
		require.Contains(t, obs.body, "unmarshal_response_body_failed")
		require.Equal(t, imageSettledReplayUnitQuota, incidentBodyResetBalance-obs.userQuota, "the settled generation is billed once")
	case "alternative_after_http_error":
		// An explicit provider rejection is refunded and still fails over.
		require.EqualValues(t, 1, obs.secondCalls, "a rejected attempt keeps cross-channel failover")
		require.Equal(t, http.StatusOK, obs.status)
		require.Contains(t, obs.body, "render.png")
		require.Equal(t, imageSettledReplayUnitQuota, incidentBodyResetBalance-obs.userQuota, "only the delivered generation is billed")
	case "initial_success", "alternative_initial_success":
		require.Zero(t, obs.secondCalls)
		require.Equal(t, http.StatusOK, obs.status)
		require.Zero(t, obs.retryRefundLogs)
	}

	debit := incidentBodyResetBalance - obs.userQuota
	generations := int64(obs.firstCalls + obs.secondCalls)
	if scenario == "alternative_after_http_error" {
		generations-- // The first dispatch was an HTTP 503 rejection, not a generation.
	}
	require.Equal(t, obs.userQuota, obs.tokenRemain, "owner and token balances move together")
	require.GreaterOrEqual(t, debit, generations*imageSettledReplayUnitQuota,
		"never under-bill: each 2xx upstream generation must be paid (generations=%d)", generations)
	require.Zero(t, obs.provisionalRows, "every reservation must be reconciled")
	require.Equal(t, obs.consumeSum, debit, "balance debit must equal the sum of consume logs")
	require.Equal(t, 1, obs.requestCostRows)
	require.Equal(t, debit, obs.requestCost, "request cost must equal the balance debit")
	require.Equal(t, debit, obs.userUsedQuota, "used_quota counter must equal the balance debit")
	require.Equal(t, debit, obs.firstChannelUsed+obs.secondChannelUsed, "channel used_quota must equal the balance debit")
}

// imageSettledReplayRun relays one POST /v1/images/generations request through
// Relay with fake upstreams: the first channel answers 200 with a malformed
// body unless the scenario ends in initial_success (or answers HTTP 503 for
// alternative_after_http_error), the second answers a valid image. Returns the observed dispatches, client response, and ledger state.
func imageSettledReplayRun(t *testing.T, scenario string, first *dbmodel.Channel) imageSettledReplayObservation {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.New(glog.WithName("image-settled-replay"), glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	var obs imageSettledReplayObservation
	var firstCalls, secondCalls atomic.Int32
	client.HTTPClient = &http.Client{Transport: incidentBodyResetRoundTripper(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		_, err := io.Copy(io.Discard, r.Body)
		require.NoError(t, err)
		body := imageSettledReplayValidBody
		status := http.StatusOK
		switch r.URL.Host {
		case "first.image.invalid":
			firstCalls.Add(1)
			var u dbmodel.User
			require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
			obs.firstHold = incidentBodyResetBalance - u.Quota
			switch {
			case scenario == "alternative_after_http_error":
				status, body = http.StatusServiceUnavailable, `{"error":{"message":"synthetic overload","type":"server_error"}}`
			case !strings.HasSuffix(scenario, "initial_success"):
				body = imageSettledReplayMalformedBody
			}
		case "second.image.invalid":
			secondCalls.Add(1)
		default:
			t.Fatalf("unexpected fixture destination %q", r.URL.Host)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	gmw.SetLogger(c, lg)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"dall-e-3","prompt":"synthetic image fixture","n":1,"size":"1024x1024"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	requestID := "image-settled-replay-" + scenario
	for key, value := range map[string]any{ctxkey.Id: 92001, ctxkey.TokenId: 92002, ctxkey.TokenName: "incident-fixture-token", ctxkey.Group: "default", ctxkey.RequestModel: imageSettledReplayModel, ctxkey.RequestId: requestID, ctxkey.TokenQuota: incidentBodyResetBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 92001, Quota: incidentBodyResetBalance}, ctxkey.Username: "incident-fixture-owner"} {
		c.Set(key, value)
	}
	middleware.SetupContextForSelectedChannel(c, first, imageSettledReplayModel)
	Relay(c)
	incidentBodyResetDrain(t)

	obs.firstCalls, obs.secondCalls = firstCalls.Load(), secondCalls.Load()
	obs.status, obs.body = recorder.Code, recorder.Body.String()
	var u dbmodel.User
	var token dbmodel.Token
	require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
	require.NoError(t, dbmodel.DB.First(&token, 92002).Error)
	obs.userQuota, obs.tokenRemain, obs.userUsedQuota, obs.userRequestCount = u.Quota, token.RemainQuota, u.UsedQuota, u.RequestCount
	var chans []dbmodel.Channel
	require.NoError(t, dbmodel.DB.Find(&chans).Error)
	for _, ch := range chans {
		switch ch.Id {
		case 92103:
			obs.firstChannelUsed = ch.UsedQuota
		case 92104:
			obs.secondChannelUsed = ch.UsedQuota
		}
	}
	var costs []dbmodel.UserRequestCost
	require.NoError(t, dbmodel.DB.Where("request_id = ?", requestID).Find(&costs).Error)
	obs.requestCostRows = len(costs)
	for _, cost := range costs {
		obs.requestCost += cost.Quota
	}
	require.NoError(t, dbmodel.LOG_DB.Where("request_id = ?", requestID).Order("id").Find(&obs.logs).Error)
	for _, row := range obs.logs {
		switch row.Type {
		case dbmodel.LogTypeConsume:
			obs.consumeSum += int64(row.Quota)
		case dbmodel.LogTypeProvisional:
			obs.provisionalRows++
		}
	}
	obs.retryRefundLogs = len(logs.FilterMessage("refunding abandoned attempt pre-consumed quota before cross-channel retry").All())
	for _, entry := range logs.FilterMessage("reconciled provisional consume log").All() {
		fields := entry.ContextMap()
		obs.reconcileSequence = append(obs.reconcileSequence, fmt.Sprintf("%v:%v", fields["log_id"], fields["final_quota"]))
	}
	return obs
}
