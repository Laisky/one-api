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

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// operatorTariffRelayChannel builds an Azure channel serving the native-only
// gpt-6-cyber deployment, optionally with a complete operator tariff.
func operatorTariffRelayChannel(id int, key string, priority int64, priced bool) *dbmodel.Channel {
	base := "https://" + key + ".openai.azure.com"
	ch := &dbmodel.Channel{Id: id, Name: key, Type: channeltype.Azure, Status: dbmodel.ChannelStatusEnabled,
		Models: "gpt-6-cyber", Group: "default", Priority: &priority, BaseURL: &base, Key: key,
		Config: `{"api_version":"2025-04-01-preview"}`}
	if priced {
		pricing := `{"gpt-6-cyber":{"ratio":1,"completion_ratio":2}}`
		ch.ModelConfigs = &pricing
	}
	return ch
}

// relayOperatorTariffRequest drives one Chat Completions request through the
// real relay loop over channels and returns the recorder, the reported attempt
// failures, and per-credential upstream call counts.
func relayOperatorTariffRequest(t *testing.T, protocol string, channels []*dbmodel.Channel, requestID string, transportErr error) (*httptest.ResponseRecorder, []processChannelRelayErrorParams, map[string]int32) {
	t.Helper()
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	incidentBodyResetRoutingFixture(t, channels, false)
	var processed []processChannelRelayErrorParams
	var processedMu sync.Mutex
	processChannelRelayErrorForTest = func(_ context.Context, params processChannelRelayErrorParams) {
		processedMu.Lock()
		defer processedMu.Unlock()
		processed = append(processed, params)
	}
	var mu sync.Mutex
	calls := map[string]int32{}
	var total atomic.Int32
	client.HTTPClient = &http.Client{Transport: incidentBodyResetRoundTripper(func(r *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, r.Body)
		total.Add(1)
		mu.Lock()
		calls[r.Header.Get("api-key")]++
		mu.Unlock()
		if transportErr != nil {
			return nil, transportErr
		}
		if strings.HasPrefix(r.Header.Get("api-key"), "failing-") {
			return &http.Response{StatusCode: http.StatusInternalServerError, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"error":{"message":"upstream exploded","type":"server_error","code":"server_error"}}`)), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(`{"id":"chatcmpl-tariff","object":"chat.completion","model":"gpt-6-cyber","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`)),
			Request: r}, nil
	})}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	gmw.SetLogger(c, logger.Logger)
	path := "/v1/chat/completions"
	if protocol == "messages" {
		path = "/v1/messages"
	}
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-6-cyber","messages":[{"role":"user","content":"hello"}],"max_tokens":16}`))
	c.Request.Header.Set("Content-Type", "application/json")
	for key, value := range map[string]any{ctxkey.Id: 92001, ctxkey.TokenId: 92002, ctxkey.TokenName: "incident-fixture-token", ctxkey.Group: "default", ctxkey.RequestModel: "gpt-6-cyber", ctxkey.RequestId: requestID, ctxkey.TokenQuota: incidentBodyResetBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 92001, Quota: incidentBodyResetBalance}, ctxkey.Username: "incident-fixture-owner"} {
		c.Set(key, value)
	}
	middleware.SetupContextForSelectedChannel(c, channels[0], "gpt-6-cyber")
	Relay(c)
	incidentBodyResetDrain(t)
	processedMu.Lock()
	defer processedMu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]int32, len(calls))
	for key, value := range calls {
		out[key] = value
	}
	return recorder, append([]processChannelRelayErrorParams(nil), processed...), out
}

// TestRelayMissingOperatorTariffIsServiceUnavailable proves that a channel
// refused for a missing operator tariff never contacts its upstream, never
// counts against channel health, fails over to a priced channel, and, when no
// candidate is priced, answers the documented 503 billing-configuration error
// instead of a generic 500 dispatch failure (matching image requests'
// image_billing_not_configured contract).
func TestRelayMissingOperatorTariffIsServiceUnavailable(t *testing.T) {
	for _, protocol := range []string{"chat", "messages"} {
		t.Run(protocol, func(t *testing.T) { testRelayMissingOperatorTariff(t, protocol) })
	}
}

// testRelayMissingOperatorTariff runs the missing-tariff relay contract for one
// client protocol. Parameters: t owns the subtests; protocol is chat or messages.
func testRelayMissingOperatorTariff(t *testing.T, protocol string) {
	t.Run("all_unpriced", func(t *testing.T) {
		channels := []*dbmodel.Channel{
			operatorTariffRelayChannel(93101, "unpriced-a", 10, false),
			operatorTariffRelayChannel(93102, "unpriced-b", 5, false),
		}
		recorder, processed, calls := relayOperatorTariffRequest(t, protocol, channels, "operator-tariff-all-unpriced-"+protocol, nil)
		require.Empty(t, calls, "no upstream may be contacted without an operator tariff")
		require.Len(t, processed, 2, "control: both candidates were attempted")
		for _, attempt := range processed {
			failure := attempt.Err
			require.False(t, countsAgainstChannelHealth(&failure), "a missing tariff is configuration, not channel health")
		}
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Contains(t, recorder.Body.String(), `"code":"model_pricing_not_configured"`)
		require.Contains(t, recorder.Body.String(), "explicit positive input and output pricing")
		var u dbmodel.User
		require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
		require.Equal(t, incidentBodyResetBalance, u.Quota, "nothing is charged")
	})

	t.Run("transport_failure_keeps_dispatch_error", func(t *testing.T) {
		// Control: a priced channel whose transport fails is a real dispatch
		// failure and keeps the generic 500 do_request_failed contract.
		channels := []*dbmodel.Channel{
			operatorTariffRelayChannel(93301, "priced-a", 10, true),
			operatorTariffRelayChannel(93302, "priced-b", 5, true),
		}
		recorder, processed, calls := relayOperatorTariffRequest(t, protocol, channels, "operator-tariff-transport-"+protocol, errors.New("dial tcp: connection refused"))
		require.Equal(t, map[string]int32{"priced-a": 1, "priced-b": 1}, calls)
		require.Len(t, processed, 2)
		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Contains(t, recorder.Body.String(), `"code":"do_request_failed"`)
	})

	t.Run("local_refusals_do_not_consume_retry_budget", func(t *testing.T) {
		// The fixture allows two retries. Three unpriced channels outrank the
		// priced one; refusals make no provider call, so like a REST transport
		// mismatch they must not spend the paid retry budget.
		channels := []*dbmodel.Channel{
			operatorTariffRelayChannel(93401, "unpriced-1", 10, false),
			operatorTariffRelayChannel(93402, "unpriced-2", 9, false),
			operatorTariffRelayChannel(93403, "unpriced-3", 8, false),
			operatorTariffRelayChannel(93404, "priced-last", 5, true),
		}
		recorder, processed, calls := relayOperatorTariffRequest(t, protocol, channels, "operator-tariff-budget-"+protocol, nil)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, map[string]int32{"priced-last": 1}, calls)
		require.Len(t, processed, 3)
	})

	t.Run("real_upstream_error_outranks_later_refusal", func(t *testing.T) {
		// A priced channel fails upstream, then the only alternative is
		// unpriced. The client must see the real provider failure, not a
		// billing-configuration error that hides it.
		channels := []*dbmodel.Channel{
			operatorTariffRelayChannel(93501, "failing-priced", 10, true),
			operatorTariffRelayChannel(93502, "unpriced-alt", 5, false),
		}
		recorder, processed, calls := relayOperatorTariffRequest(t, protocol, channels, "operator-tariff-real-error-"+protocol, nil)
		require.Equal(t, map[string]int32{"failing-priced": 1}, calls)
		require.Len(t, processed, 2)
		require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), "model_pricing_not_configured")
	})

	t.Run("fails_over_to_priced", func(t *testing.T) {
		channels := []*dbmodel.Channel{
			operatorTariffRelayChannel(93201, "unpriced-first", 10, false),
			operatorTariffRelayChannel(93202, "priced-second", 5, true),
		}
		recorder, processed, calls := relayOperatorTariffRequest(t, protocol, channels, "operator-tariff-failover-"+protocol, nil)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, map[string]int32{"priced-second": 1}, calls)
		require.Len(t, processed, 1)
		failure := processed[0].Err
		require.False(t, countsAgainstChannelHealth(&failure))
	})
}
