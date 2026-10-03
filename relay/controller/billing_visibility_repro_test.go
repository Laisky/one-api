package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// ---------------------------------------------------------------------------
// Reproduction suite for: "quota was debited but the Logs page shows nothing".
//
// Every assertion here is stated in terms of OBSERVABLE behaviour an operator
// or a user can check — the persisted user balance, and the rows returned by
// model.GetAllLogs/GetUserLogs, which are the exact queries backing the Logs
// page (controller/log.go). None of these tests reach into the internals of the
// billing state machine, so a fix is free to restructure it.
//
// The suite is deliberately built to EXCLUDE FALSE POSITIVES:
//   - a control test (…_ControlVisibleLog) runs the identical harness against a
//     channel whose adaptor does report usage and requires the log to be
//     visible, proving the harness itself can see logs;
//   - the E2E test asserts the upstream was actually contacted and returned a
//     terminal usage block, proving the missing usage is the gateway's fault
//     and not the fixture's;
//   - the settlement tests seed the provisional row through the production
//     helper rather than hand-writing a type=6 row.
// ---------------------------------------------------------------------------

const (
	reproXAIChannelID       = 99101
	reproCompatibleChanneID = 99102
)

// xaiStreamingSSE is a faithful reproduction of what api.x.ai streams back for a
// grok-4.5 Response API call: incremental reasoning/text events, then a terminal
// response.completed event that carries the ONLY usage block in the whole stream.
const xaiStreamingSSE = "event: response.created\n" +
	"data: {\"sequence_number\":0,\"type\":\"response.created\",\"response\":{\"id\":\"resp_repro\",\"object\":\"response\",\"created_at\":1784588116,\"status\":\"in_progress\",\"model\":\"grok-4.5\",\"output\":[],\"usage\":null}}\n\n" +
	"event: response.in_progress\n" +
	"data: {\"sequence_number\":1,\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_repro\",\"object\":\"response\",\"created_at\":1784588116,\"status\":\"in_progress\",\"model\":\"grok-4.5\",\"output\":[],\"usage\":null}}\n\n" +
	"event: response.output_item.added\n" +
	"data: {\"sequence_number\":2,\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"msg_repro\",\"type\":\"message\",\"status\":\"in_progress\",\"role\":\"assistant\",\"content\":[]}}\n\n" +
	"event: response.output_text.delta\n" +
	"data: {\"sequence_number\":3,\"type\":\"response.output_text.delta\",\"item_id\":\"msg_repro\",\"output_index\":0,\"content_index\":0,\"delta\":\"halo\"}\n\n" +
	"event: response.completed\n" +
	"data: {\"sequence_number\":4,\"type\":\"response.completed\",\"response\":{\"id\":\"resp_repro\",\"object\":\"response\",\"created_at\":1784588116,\"status\":\"completed\",\"model\":\"grok-4.5\",\"output\":[{\"id\":\"msg_repro\",\"type\":\"message\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"halo\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":320,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":40,\"output_tokens_details\":{\"reasoning_tokens\":12},\"total_tokens\":360}}}\n\n" +
	"data: [DONE]\n\n"

// upstreamReproStats records what the fake provider actually observed, so a test
// can prove the request really reached it (and therefore that a missing charge or
// missing log is the gateway's doing).
type upstreamReproStats struct {
	hits int
	path string
}

// newStreamingReproUpstream installs a fake provider that answers a Response API
// call with the given SSE body and reports what it saw.
func newStreamingReproUpstream(t *testing.T, sse string) (string, *upstreamReproStats) {
	t.Helper()

	stats := &upstreamReproStats{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stats.hits++
		stats.path = r.URL.Path

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sse))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	prev := client.HTTPClient
	client.HTTPClient = srv.Client()
	t.Cleanup(func() { client.HTTPClient = prev })

	return srv.URL, stats
}

// ensureReproChannels adds the channel fixtures this suite needs on top of the
// shared response-fallback fixtures.
func ensureReproChannels(t *testing.T) {
	t.Helper()

	for _, ch := range []*model.Channel{
		{Id: reproXAIChannelID, Type: channeltype.XAI, Name: "xai-repro", Status: model.ChannelStatusEnabled},
		{Id: reproCompatibleChanneID, Type: channeltype.OpenAICompatible, Name: "compatible-repro", Status: model.ChannelStatusEnabled},
	} {
		require.NoError(t, model.DB.Where("id = ?", ch.Id).Delete(&model.Channel{}).Error)
		require.NoError(t, model.DB.Create(ch).Error)
	}
}

// enableConsumeLogging turns consume logging ON. Most existing tests in this
// package switch it off; this suite is specifically about log visibility, so it
// must run with the production default.
func enableConsumeLogging(t *testing.T) {
	t.Helper()

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })
}

// setupReproContext builds a streaming /v1/responses request bound to the given
// channel, mirroring what the distributor middleware sets in production.
func setupReproContext(t *testing.T, recorder *httptest.ResponseRecorder, channelID, channelType int, modelName, requestID, payload string) *gin.Context {
	t.Helper()

	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer repro-key")
	c.Request = req
	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channelType)
	c.Set(ctxkey.ChannelId, channelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, modelName)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, requestID)
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: 1_000_000})
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: channelID, Type: channelType})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	return c
}

// visibleLogsForRequest returns the consume rows the Logs page would show for a
// request, using the very query the HTTP handler calls.
func visibleLogsForRequest(t *testing.T, requestID string) []*model.Log {
	t.Helper()

	logs, err := model.GetAllLogs(model.LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "", "")
	require.NoError(t, err)

	var matched []*model.Log
	for _, l := range logs {
		if l.RequestId == requestID {
			matched = append(matched, l)
		}
	}
	return matched
}

// allLogRowsForRequest bypasses every visibility filter and returns whatever is
// physically stored, so a test can distinguish "no row was written" from "a row
// was written but is hidden".
func allLogRowsForRequest(t *testing.T, requestID string) []model.Log {
	t.Helper()

	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Find(&rows).Error)
	return rows
}

// runReproRelay executes one streaming Response API request end to end and
// returns the quota actually debited from the persisted user balance.
func runReproRelay(t *testing.T, channelID, channelType int, modelName, requestID, upstreamURL string) (charged int64, recorder *httptest.ResponseRecorder) {
	t.Helper()

	resetFallbackUserQuota(t, 1_000_000)
	before := fallbackUserQuota(t)

	recorder = httptest.NewRecorder()
	payload := fmt.Sprintf(`{"model":%q,"stream":true,"max_output_tokens":5000,"input":[{"role":"user","content":[{"type":"input_text","text":"halo"}]}]}`, modelName)
	c := setupReproContext(t, recorder, channelID, channelType, modelName, requestID, payload)
	c.Set(ctxkey.BaseURL, upstreamURL)

	apiErr := RelayResponseAPIHelper(c)
	require.Nil(t, apiErr, "a successful upstream stream must not surface an error")

	// Post-billing runs in a detached critical goroutine; wait for it so the
	// persisted balance and log rows are final.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))

	return before - fallbackUserQuota(t), recorder
}

// TestBillingVisibility_XAIStreamingResponseAPI reproduces the production
// incident end to end: a successful streaming grok-4.5 call on an XAI channel
// debits the user's balance but leaves nothing on the Logs page.
//
// It fails on the buggy code with "charged N quota but the Logs page shows 0
// rows", and passes only when the charge and the visible log agree.
func TestBillingVisibility_XAIStreamingResponseAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	ensureReproChannels(t)
	enableConsumeLogging(t)

	upstreamURL, stats := newStreamingReproUpstream(t, xaiStreamingSSE)
	requestID := fmt.Sprintf("repro-xai-%d", time.Now().UnixNano())

	charged, recorder := runReproRelay(t, reproXAIChannelID, channeltype.XAI, "grok-4.5", requestID, upstreamURL)

	// The fixture is sound: the provider really was called and really answered.
	require.Equal(t, 1, stats.hits, "the upstream must have been contacted exactly once")
	require.Equal(t, "/v1/responses", stats.path)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "response.completed",
		"the client must receive the terminal event that carries usage")

	// Invariant A: the user was charged for a request that really happened.
	require.Positive(t, charged, "a forwarded, successful request must debit quota")

	// Invariant B: that charge must be visible. This is the reported bug.
	visible := visibleLogsForRequest(t, requestID)
	stored := allLogRowsForRequest(t, requestID)
	require.Len(t, visible, 1,
		"charged %d quota but the Logs page shows %d row(s); %d row(s) exist in the table (types %v) — a charge the operator cannot see",
		charged, len(visible), len(stored), logTypesOf(stored))

	row := visible[0]
	require.Equal(t, model.LogTypeConsume, row.Type, "a settled charge must be a consume row")
	require.Equal(t, int64(row.Quota), charged, "the visible log must state the amount actually debited")
	require.NotContains(t, row.Content, "provisional", "a settled row must not still be labelled provisional")

	// Invariant A, sharper: the charge must reflect the usage the provider
	// reported (320 in / 40 out), not the blind pre-consume estimate derived
	// from max_output_tokens=5000.
	require.Equal(t, 320, row.PromptTokens, "the settled row must carry the upstream prompt tokens")
	require.Equal(t, 40, row.CompletionTokens, "the settled row must carry the upstream completion tokens")
}

// TestBillingVisibility_ControlVisibleLog is the false-positive guard for the
// test above. It runs the same harness, the same assertions and the same
// visibility query against an OpenAI-compatible channel, whose Response API
// stream handler does parse usage. If this control ever fails together with the
// XAI test, the harness — not the XAI adaptor — is at fault.
func TestBillingVisibility_ControlVisibleLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	ensureReproChannels(t)
	enableConsumeLogging(t)

	upstreamURL, stats := newStreamingReproUpstream(t, xaiStreamingSSE)
	requestID := fmt.Sprintf("repro-control-%d", time.Now().UnixNano())

	charged, recorder := runReproRelay(t, reproCompatibleChanneID, channeltype.OpenAICompatible, "gpt-4o-mini", requestID, upstreamURL)

	require.Equal(t, 1, stats.hits)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Positive(t, charged, "control: a forwarded request must debit quota")

	visible := visibleLogsForRequest(t, requestID)
	require.Len(t, visible, 1,
		"control harness must be able to see a settled consume log; if this fails the reproduction test proves nothing")
	require.Equal(t, model.LogTypeConsume, visible[0].Type)
}

// logTypesOf summarises the physical row types behind a request, for failure
// messages that distinguish "nothing was written" from "written but hidden".
func logTypesOf(rows []model.Log) []int {
	types := make([]int, 0, len(rows))
	for _, r := range rows {
		types = append(types, r.Type)
	}
	return types
}

// ---------------------------------------------------------------------------
// Settlement-level reproduction: whatever the adaptor reports, a pre-consumed
// charge must never be left invisible.
// ---------------------------------------------------------------------------

// TestProvisionalLogAlwaysSettles proves that a request whose upstream reported
// no usage still ends with a visible consume row, because the money was already
// taken at pre-consume time. It drives the production settlement helper, not a
// hand-written row.
func TestProvisionalLogAlwaysSettles(t *testing.T) {
	cases := []struct {
		name  string
		usage *relaymodel.Usage
	}{
		{name: "nil_usage", usage: nil},
		{name: "zero_usage", usage: &relaymodel.Usage{}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cleanup := setupCacheBillingLogTest(t)
			defer cleanup()
			// The Logs page joins channel names onto every row, so the visibility
			// query needs the channels table present.
			require.NoError(t, model.LOG_DB.AutoMigrate(&model.Channel{}))

			prevLogConsume := config.IsLogConsumeEnabled()
			config.SetLogConsumeEnabled(true)
			defer config.SetLogConsumeEnabled(prevLogConsume)

			const (
				channelID       = 77
				modelName       = "grok-4.5"
				preConsumedQuot = int64(15293)
			)
			requestID := fmt.Sprintf("repro-settle-%s-%d", tc.name, time.Now().UnixNano())

			ctx := newCacheBillingContext(t, requestID, 0)
			provisionalLogID := model.RecordProvisionalConsumeLog(ctx, &model.Log{
				UserId:    1,
				ChannelId: channelID,
				ModelName: modelName,
				TokenName: "relay-cache-log-token",
				RequestId: requestID,
			}, preConsumedQuot)
			require.Greater(t, provisionalLogID, 0)

			// Before settlement the row is intentionally hidden.
			require.Empty(t, visibleLogsForRequest(t, requestID),
				"a pending provisional row is hidden by design")

			ctx = newCacheBillingContext(t, requestID, provisionalLogID)
			postConsumeResponseAPIQuota(
				ctx,
				tc.usage,
				newCacheBillingMeta(channelID),
				&openai.ResponseAPIRequest{Model: modelName},
				preConsumedQuot,
				1,
				nil,
				1,
				nil,
				map[string]float64{modelName: 1},
			)

			visible := visibleLogsForRequest(t, requestID)
			stored := allLogRowsForRequest(t, requestID)
			require.Len(t, visible, 1,
				"%d quota was pre-consumed and never refunded, so it must appear on the Logs page; stored row types: %v",
				preConsumedQuot, logTypesOf(stored))

			row := visible[0]
			require.Equal(t, model.LogTypeConsume, row.Type)
			require.Equal(t, int(preConsumedQuot), row.Quota,
				"an unreconcilable request must be logged at the amount actually debited")
			_, stillProvisional := row.Metadata[model.LogMetadataKeyProvisional]
			require.False(t, stillProvisional, "the provisional flag must be cleared once settled")
		})
	}
}

// TestSettledRequestCostIsNotZeroed proves the per-request cost record keeps the
// amount the user actually paid. Reporting $0.00 for a request that debited real
// quota is a silent accounting hole.
func TestSettledRequestCostIsNotZeroed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	ensureReproChannels(t)
	enableConsumeLogging(t)

	upstreamURL, _ := newStreamingReproUpstream(t, xaiStreamingSSE)
	requestID := fmt.Sprintf("repro-cost-%d", time.Now().UnixNano())

	charged, _ := runReproRelay(t, reproXAIChannelID, channeltype.XAI, "grok-4.5", requestID, upstreamURL)
	require.Positive(t, charged)

	var cost model.UserRequestCost
	err := model.DB.Where("request_id = ?", requestID).First(&cost).Error
	require.NoError(t, err, "a billed request must leave a cost record")
	require.Equal(t, charged, cost.Quota,
		"user_request_costs must report the quota actually debited, not 0")
}

// ---------------------------------------------------------------------------
// Adaptor-level reproduction.
// ---------------------------------------------------------------------------

// TestXAIStreamingResponseAPIReportsUsage pins the adaptor contract that the
// whole billing chain depends on: a streaming Response API response must yield
// the usage the provider reported in its terminal event, while still forwarding
// the stream to the client untouched.
//
// The XAI adaptor currently returns (nil, nil) here, which is the origin of the
// missing log. Note relay/adaptor/xai/adaptor_test.go asserts the opposite and
// must be updated together with the fix.
func TestXAIStreamingResponseAPIReportsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	ensureReproChannels(t)
	enableConsumeLogging(t)

	upstreamURL, _ := newStreamingReproUpstream(t, xaiStreamingSSE)
	requestID := fmt.Sprintf("repro-passthrough-%d", time.Now().UnixNano())

	_, recorder := runReproRelay(t, reproXAIChannelID, channeltype.XAI, "grok-4.5", requestID, upstreamURL)

	// The client must still receive every event, in order: extracting usage may
	// not come at the cost of mangling the stream.
	body := recorder.Body.String()
	for _, evt := range []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.output_text.delta",
		"response.completed",
	} {
		require.Contains(t, body, evt, "the client stream must preserve %s", evt)
	}

	// And the usage the provider reported must have reached billing.
	visible := visibleLogsForRequest(t, requestID)
	require.Len(t, visible, 1, "usage from response.completed must produce a settled consume row")
	require.Equal(t, 320, visible[0].PromptTokens)
	require.Equal(t, 40, visible[0].CompletionTokens)
}

// unusedMetaGuard keeps the metalib import meaningful if the harness is trimmed;
// it documents that the reproduction relies on the production Meta shape.
var _ = func() *metalib.Meta { return nil }
