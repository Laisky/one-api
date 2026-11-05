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
)

// deliveryFailureWriter commits the status line and then fails every body
// write, like a downstream connection that broke after the response began.
type deliveryFailureWriter struct {
	gin.ResponseWriter
	cause  error
	writes *atomic.Int32
}

// Write commits headers and reports the configured delivery failure.
func (w deliveryFailureWriter) Write([]byte) (int, error) {
	w.writes.Add(1)
	w.WriteHeaderNow()
	return 0, w.cause
}

// WriteString follows the same failed delivery boundary as Write.
func (w deliveryFailureWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// deliveryFailureSurface describes one client API surface and the provider
// receipt its registered adaptor parses. Every receipt measures 11 input and
// 7 output tokens.
type deliveryFailureSurface struct {
	name, channelType, path, model, request, contentType, providerPath, providerBody string
	typeID                                                                           int
	base                                                                             string
	// retainsReservation marks a stream whose handler stops reading at the
	// failed write before the final receipt: it settles the funded reservation
	// as an explicit estimate instead of the unobserved receipt.
	retainsReservation bool
}

// deliveryFailureSurfaces lists the relay helpers that settle a provider receipt
// before a downstream delivery failure can be observed.
func deliveryFailureSurfaces() []deliveryFailureSurface {
	const chatJSON = `{"id":"chatcmpl-delivery","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"delivered answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`
	const chatStream = `data: {"id":"chatcmpl-delivery","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"delivered answer"},"finish_reason":null}]}` + "\n\n" +
		`data: {"id":"chatcmpl-delivery","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
		`data: {"id":"chatcmpl-delivery","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}` + "\n\n" +
		"data: [DONE]\n\n"
	const claudeJSON = `{"id":"msg_delivery","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"delivered answer"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":11,"output_tokens":7}}`
	const claudeStream = "event: message_start\n" + `data: {"type":"message_start","message":{"id":"msg_delivery","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":11,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"delivered answer"}}` + "\n\n" +
		"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}` + "\n\n" +
		"event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"
	const responsesJSON = `{"id":"resp-delivery","object":"response","status":"completed","model":"gpt-4o","output":[{"type":"message","id":"msg-delivery","role":"assistant","content":[{"type":"output_text","text":"delivered answer"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`
	const responsesStream = "event: response.completed\n" + `data: {"type":"response.completed","response":` + responsesJSON + "}\n\n"
	return []deliveryFailureSurface{
		// OpenAI chat requests are relayed to the provider's Responses API.
		{name: "openai_chat_json", typeID: channeltype.OpenAI, base: "https://api.openai.com", path: "/v1/chat/completions", model: "gpt-4o-mini",
			request: `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`, contentType: "application/json", providerPath: "/v1/responses", providerBody: responsesJSON},
		{name: "openai_chat_stream", typeID: channeltype.OpenAI, base: "https://api.openai.com", path: "/v1/chat/completions", model: "gpt-4o-mini",
			request: `{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}`, contentType: "text/event-stream", providerPath: "/v1/responses", providerBody: responsesStream},
		{name: "compatible_chat_json", typeID: channeltype.OpenAICompatible, base: "https://chat.delivery.invalid", path: "/v1/chat/completions", model: "gpt-4o-mini",
			request: `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`, contentType: "application/json", providerPath: "/v1/chat/completions", providerBody: chatJSON},
		{name: "compatible_chat_stream", typeID: channeltype.OpenAICompatible, base: "https://chat.delivery.invalid", path: "/v1/chat/completions", model: "gpt-4o-mini",
			request: `{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}`, contentType: "text/event-stream", providerPath: "/v1/chat/completions", providerBody: chatStream},
		// A Responses client served by a Chat-only channel uses the fallback bridge.
		{name: "compatible_responses_fallback_json", typeID: channeltype.OpenAICompatible, base: "https://chat.delivery.invalid", path: "/v1/responses", model: "gpt-4o-mini",
			request: `{"model":"gpt-4o-mini","input":"hello","max_output_tokens":16}`, contentType: "application/json", providerPath: "/v1/chat/completions", providerBody: chatJSON},
		{name: "compatible_responses_fallback_stream", typeID: channeltype.OpenAICompatible, base: "https://chat.delivery.invalid", path: "/v1/responses", model: "gpt-4o-mini",
			request: `{"model":"gpt-4o-mini","input":"hello","max_output_tokens":16,"stream":true}`, contentType: "text/event-stream", providerPath: "/v1/chat/completions", providerBody: chatStream},
		// Dedicated OpenAI-compatible adaptors share openai_compatible.Handler.
		{name: "mistral_chat_json", typeID: channeltype.Mistral, base: "https://mistral.delivery.invalid", path: "/v1/chat/completions", model: "mistral-small-latest",
			request: `{"model":"mistral-small-latest","messages":[{"role":"user","content":"hello"}]}`, contentType: "application/json", providerPath: "/v1/chat/completions", providerBody: chatJSON},
		{name: "mistral_chat_stream", typeID: channeltype.Mistral, base: "https://mistral.delivery.invalid", path: "/v1/chat/completions", model: "mistral-small-latest",
			request: `{"model":"mistral-small-latest","stream":true,"messages":[{"role":"user","content":"hello"}]}`, contentType: "text/event-stream", providerPath: "/v1/chat/completions", providerBody: chatStream},
		{name: "mistral_responses_fallback_json", typeID: channeltype.Mistral, base: "https://mistral.delivery.invalid", path: "/v1/responses", model: "mistral-small-latest",
			request: `{"model":"mistral-small-latest","input":"hello","max_output_tokens":16}`, contentType: "application/json", providerPath: "/v1/chat/completions", providerBody: chatJSON},
		{name: "mistral_responses_fallback_stream", typeID: channeltype.Mistral, base: "https://mistral.delivery.invalid", path: "/v1/responses", model: "mistral-small-latest",
			request: `{"model":"mistral-small-latest","input":"hello","max_output_tokens":16,"stream":true}`, contentType: "text/event-stream", providerPath: "/v1/chat/completions", providerBody: chatStream},
		{name: "claude_json", typeID: channeltype.Anthropic, base: "https://claude.delivery.invalid", path: "/v1/messages", model: "claude-sonnet-4-5",
			request: `{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`, contentType: "application/json", providerPath: "/v1/messages", providerBody: claudeJSON},
		{name: "claude_stream", typeID: channeltype.Anthropic, base: "https://claude.delivery.invalid", path: "/v1/messages", model: "claude-sonnet-4-5",
			request: `{"model":"claude-sonnet-4-5","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hello"}]}`, contentType: "text/event-stream", providerPath: "/v1/messages", providerBody: claudeStream,
			retainsReservation: true},
		{name: "responses_json", typeID: channeltype.OpenAI, base: "https://api.openai.com", path: "/v1/responses", model: "gpt-4o",
			request: `{"model":"gpt-4o","input":"hello","max_output_tokens":16}`, contentType: "application/json", providerPath: "/v1/responses", providerBody: responsesJSON},
		{name: "responses_stream", typeID: channeltype.OpenAI, base: "https://api.openai.com", path: "/v1/responses", model: "gpt-4o",
			request: `{"model":"gpt-4o","input":"hello","max_output_tokens":16,"stream":true}`, contentType: "text/event-stream", providerPath: "/v1/responses", providerBody: responsesStream},
	}
}

// deliveryFailureChannel builds one enabled channel with a deterministic 1:1
// tariff; its key identifies the dispatch destination.
func deliveryFailureChannel(surface deliveryFailureSurface, id int, key string, priority int64) *dbmodel.Channel {
	base := surface.base
	pricing := `{"` + surface.model + `":{"ratio":1,"completion_ratio":1}}`
	return &dbmodel.Channel{Id: id, Name: key, Type: surface.typeID, Status: dbmodel.ChannelStatusEnabled,
		Models: surface.model, Group: "default", Priority: &priority, BaseURL: &base, ModelConfigs: &pricing, Key: key}
}

// TestRelayDeliveryFailureSettlesOnce exercises the complete transaction from
// a successful provider receipt, through a downstream writer that fails after
// output is committed, to persistent settlement. It drives the real relay loop,
// the registered adaptors, every settling relay helper and SQLite billing, with
// an alternative channel available, and checks that the provider is called
// once, the receipt is settled exactly once without a refund, the failure is
// reported without a channel-health penalty, and no error body is appended.
func TestRelayDeliveryFailureSettlesOnce(t *testing.T) {
	for _, surface := range deliveryFailureSurfaces() {
		t.Run(surface.name, func(t *testing.T) {
			previousGinMode := gin.Mode()
			gin.SetMode(gin.TestMode)
			t.Cleanup(func() { gin.SetMode(previousGinMode) })
			first := deliveryFailureChannel(surface, 92203, "first-delivery-key", 10)
			second := deliveryFailureChannel(surface, 92204, "second-delivery-key", 5)
			incidentBodyResetRoutingFixture(t, []*dbmodel.Channel{first, second}, false)
			var processed []processChannelRelayErrorParams
			var processedMu sync.Mutex
			processChannelRelayErrorForTest = func(_ context.Context, params processChannelRelayErrorParams) {
				processedMu.Lock()
				defer processedMu.Unlock()
				processed = append(processed, params)
			}
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.New(glog.WithName("delivery-settlement"), glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
			require.NoError(t, err)
			var firstCalls, secondCalls atomic.Int32
			var holdBeforeDispatch atomic.Int64
			client.HTTPClient = &http.Client{Transport: incidentBodyResetRoundTripper(func(r *http.Request) (*http.Response, error) {
				_, err := io.Copy(io.Discard, r.Body)
				require.NoError(t, err)
				credential := r.Header.Get("Authorization") + r.Header.Get("x-api-key")
				switch {
				case strings.Contains(credential, "first-delivery-key"):
					firstCalls.Add(1)
					var u dbmodel.User
					require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
					holdBeforeDispatch.Store(incidentBodyResetBalance - u.Quota)
				case strings.Contains(credential, "second-delivery-key"):
					secondCalls.Add(1)
				default:
					t.Errorf("unexpected fixture credential for %s", r.URL)
				}
				require.Equal(t, surface.providerPath, r.URL.Path)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{surface.contentType}},
					Body: io.NopCloser(strings.NewReader(surface.providerBody)), Request: r}, nil
			})}

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, lg)
			c.Request = httptest.NewRequest(http.MethodPost, surface.path, strings.NewReader(surface.request))
			c.Request.Header.Set("Content-Type", "application/json")
			cause := stderrors.New("synthetic downstream delivery failure")
			var writes atomic.Int32
			c.Writer = deliveryFailureWriter{ResponseWriter: c.Writer, cause: cause, writes: &writes}
			requestID := "delivery-settlement-" + surface.name
			for key, value := range map[string]any{ctxkey.Id: 92001, ctxkey.TokenId: 92002, ctxkey.TokenName: "incident-fixture-token", ctxkey.Group: "default", ctxkey.RequestModel: surface.model, ctxkey.RequestId: requestID, ctxkey.TokenQuota: incidentBodyResetBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 92001, Quota: incidentBodyResetBalance}, ctxkey.Username: "incident-fixture-owner"} {
				c.Set(key, value)
			}
			middleware.SetupContextForSelectedChannel(c, first, surface.model)
			Relay(c)
			incidentBodyResetDrain(t)

			var u dbmodel.User
			var token dbmodel.Token
			require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
			require.NoError(t, dbmodel.DB.First(&token, 92002).Error)
			var rows []dbmodel.Log
			require.NoError(t, dbmodel.LOG_DB.Where("request_id = ?", requestID).Order("id").Find(&rows).Error)
			var costs []dbmodel.UserRequestCost
			require.NoError(t, dbmodel.DB.Where("request_id = ?", requestID).Find(&costs).Error)
			processedMu.Lock()
			reported := append([]processChannelRelayErrorParams(nil), processed...)
			processedMu.Unlock()
			final := logs.FilterMessage("relay request completed").All()

			// Exactly one provider attempt, although an alternative channel exists.
			require.EqualValues(t, 1, firstCalls.Load(), "exactly one provider dispatch")
			require.Zero(t, secondCalls.Load(), "a settled attempt must not be replayed")
			require.Positive(t, holdBeforeDispatch.Load(), "a real reservation precedes dispatch")
			require.Positive(t, writes.Load(), "the downstream write was attempted")

			// The failure is reported once, as a local failure, without a
			// channel-health penalty, and no error body follows committed output.
			require.Len(t, reported, 1, "the failed delivery must reach the relay failure funnel")
			failure := reported[0].Err
			require.ErrorIs(t, failure.RawError, cause)
			require.Equal(t, errkind.Server, relayFailureKind(&failure), "a delivery failure is local, not upstream")
			require.False(t, countsAgainstChannelHealth(&failure), "a delivery failure must not penalise the channel")
			// Run the real failure funnel: it must neither suspend nor disable the
			// channel that served the receipt.
			processChannelRelayError(context.Background(), reported[0])
			var suspended int64
			require.NoError(t, dbmodel.DB.Model(&dbmodel.Ability{}).Where("channel_id = ? AND suspend_until IS NOT NULL", first.Id).Count(&suspended).Error)
			require.Zero(t, suspended, "a delivery failure must not suspend the serving channel")
			var served dbmodel.Channel
			require.NoError(t, dbmodel.DB.First(&served, first.Id).Error)
			require.Equal(t, dbmodel.ChannelStatusEnabled, served.Status)
			require.Len(t, final, 1)
			require.Equal(t, "helper_failed", final[0].ContextMap()["relay_outcome"])
			require.Equal(t, "committed_response_preserved", final[0].ContextMap()["client_outcome"])
			require.Empty(t, recorder.Body.String(), "no error JSON may be appended to committed output")

			// The purchased work is settled exactly once and never refunded: the
			// measured receipt, or the funded reservation when the stream stopped
			// before its final receipt (never less than the observed receipt).
			const receipt = int64(18)
			charge := incidentBodyResetBalance - u.Quota
			require.Equal(t, u.Quota, token.RemainQuota, "owner and token balances move together")
			require.Len(t, costs, 1)
			require.Equal(t, charge, costs[0].Quota, "request cost equals the single debit")
			require.Len(t, rows, 1, "one final ledger row; no provisional or refund row remains")
			require.Equal(t, dbmodel.LogTypeConsume, rows[0].Type)
			require.EqualValues(t, charge, rows[0].Quota, "the consume row equals the single debit")
			require.EqualValues(t, 11, rows[0].PromptTokens)
			if surface.retainsReservation {
				require.Equal(t, holdBeforeDispatch.Load(), charge, "the funded reservation is retained, not refunded")
				require.GreaterOrEqual(t, charge, receipt)
				require.Equal(t, true, rows[0].Metadata["billing_estimated"])
			} else {
				require.Equal(t, receipt, charge, "owner pays the measured receipt once")
				require.EqualValues(t, 7, rows[0].CompletionTokens)
			}
			require.Empty(t, logs.FilterMessageSnippet("refund").All(), "purchased provider usage must not be refunded")
		})
	}
}
