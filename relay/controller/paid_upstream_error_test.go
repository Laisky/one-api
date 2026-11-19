package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// paidErrorReply selects what the fake provider answers for one attempt.
type paidErrorReply int

const (
	// paidErrorSuccess answers HTTP 200 with a normal receipt (the control).
	paidErrorSuccess paidErrorReply = iota
	// paidErrorWithReceipt answers HTTP 500 whose body carries a usage receipt.
	paidErrorWithReceipt
	// paidErrorPlain answers HTTP 500 with a plain error and no usage.
	paidErrorPlain
)

// paidErrorPrompt and paidErrorCompletion are the receipt counters. They are
// large so the settled quota is far above the admission reservation.
const (
	paidErrorPrompt     = 20_000
	paidErrorCompletion = 4_000
)

// paidErrorUpstream returns a TLS fake provider answering each wire surface in
// its own shape: Responses envelopes on /responses, Chat Completions bodies
// elsewhere. Parameters: t owns cleanup, reply selects the outcome, model is
// echoed, and calls counts real HTTP requests. Returns: the started server.
func paidErrorUpstream(t *testing.T, reply paidErrorReply, modelName string, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		responses := strings.HasSuffix(r.URL.Path, "/responses")
		switch {
		case reply == paidErrorPlain:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":{"message":"upstream exploded","type":"server_error","code":"server_error"}}`)
		case responses:
			status, state, errField := http.StatusOK, "completed", "null"
			if reply == paidErrorWithReceipt {
				status, state, errField = http.StatusInternalServerError, "failed", `{"code":"server_error","message":"generation failed"}`
			}
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"id":"resp_paid","object":"response","status":%q,"model":%q,"error":%s,"output":[{"type":"web_search_call","id":"ws_paid","status":"completed","action":{"type":"search","query":"one-api"}},{"type":"message","id":"msg_paid","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}`,
				state, modelName, errField, paidErrorPrompt, paidErrorCompletion, paidErrorPrompt+paidErrorCompletion)
		case reply == paidErrorWithReceipt:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"generation failed","type":"server_error","code":"server_error"},"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
				paidErrorPrompt, paidErrorCompletion, paidErrorPrompt+paidErrorCompletion)
		default:
			_, _ = fmt.Fprintf(w, `{"id":"chatcmpl-paid","object":"chat.completion","model":%q,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
				modelName, paidErrorPrompt, paidErrorCompletion, paidErrorPrompt+paidErrorCompletion)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// paidErrorOutcome is what one relay attempt left in the ledger.
type paidErrorOutcome struct {
	failed   bool
	status   int
	calls    int32
	charged  int64
	held     int64
	logQuota int64
	settled  bool
	log      *model.Log
}

// relayPaidErrorAttempt drives one request through the production helper for
// protocol against a fake provider. Parameters: t owns the fixture, channel and
// modelName select the provider route, protocol is chat/responses/messages, and
// reply selects the provider outcome. Returns: the observed outcome.
func relayPaidErrorAttempt(t *testing.T, channel int, modelName, protocol string, reply paidErrorReply) paidErrorOutcome {
	t.Helper()
	const balance = int64(10_000_000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	upstream := paidErrorUpstream(t, reply, modelName, &calls)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	// responses_native is the client Responses protocol on the provider's native
	// Responses route; plain responses on a custom base URL uses the Chat fallback.
	clientProtocol := strings.TrimSuffix(protocol, "_native")
	path, body := lyriaProtocolRequest(clientProtocol, false)
	c, _, requestID := protocolContext(t, channel, modelName, path, body, upstream.URL, balance, 1, false, nil)
	if channel == channeltype.Azure {
		c.Set(ctxkey.Config, model.ChannelConfig{APIVersion: "2025-04-01-preview"})
	}
	if protocol == "responses_native" {
		c.Set(ctxkey.BaseURL, "https://api.openai.com")
		c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{
			channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): upstream.URL + "/v1/responses",
		}})
	}
	clearNativeSettledLedger(t, requestID)
	if protocol == "responses_native" {
		require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "control: the native Responses route must be selected")
	} else if clientProtocol == "responses" && channel == channeltype.OpenAI {
		require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "control: the custom base URL selects the Chat fallback")
	}

	apiErr := relayLyriaProtocol(c, clientProtocol)
	drainCriticalTasks(t)
	out := paidErrorOutcome{
		failed: apiErr != nil, calls: calls.Load(), charged: balance - reloadUserQuota(t),
		held:     c.GetInt64(ctxkey.PreConsumedQuotaAmount),
		logQuota: consumeLogQuota(t, requestID), settled: c.GetBool(responseSettlementKey),
	}
	if apiErr != nil {
		out.status = apiErr.StatusCode
	}
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	if len(logs) == 1 {
		out.log = &logs[0]
	}
	return out
}

// TestPaidUpstreamErrorReceiptIsSettled proves that an OpenAI-wire (native
// OpenAI or Azure) non-2xx response carrying an explicit usage receipt is billed
// exactly like a 2xx response with the same receipt and is marked settled, so
// the retry loop can neither refund it nor replay the paid attempt on another
// channel. Before the fix the attempt only retained its small admission hold,
// which the retry loop refunds before failing over. A plain non-2xx error
// without usage keeps that retained-hold, retryable path unchanged.
func TestPaidUpstreamErrorReceiptIsSettled(t *testing.T) {
	for _, tc := range []struct {
		channel int
		model   string
	}{
		{channeltype.OpenAI, "gpt-4o-mini"},
		{channeltype.OpenAI, "gpt-6-cyber"},
		{channeltype.Azure, "gpt-4o-mini"},
	} {
		channel, modelName := tc.channel, tc.model
		protocols := []string{"chat", "responses", "messages"}
		if channel == channeltype.OpenAI {
			protocols = append(protocols, "responses_native")
		}
		for _, protocol := range protocols {
			t.Run(channeltype.IdToName(channel)+"/"+modelName+"/"+protocol, func(t *testing.T) {
				control := relayPaidErrorAttempt(t, channel, modelName, protocol, paidErrorSuccess)
				require.False(t, control.failed, "control: the 2xx receipt must succeed")
				require.Positive(t, control.charged, "control: the 2xx receipt must be charged")
				require.Equal(t, control.charged, control.logQuota)

				paid := relayPaidErrorAttempt(t, channel, modelName, protocol, paidErrorWithReceipt)
				require.True(t, paid.failed, "the provider error must still reach the client")
				require.Equal(t, http.StatusInternalServerError, paid.status)
				require.EqualValues(t, 1, paid.calls)
				require.Equal(t, control.charged, paid.charged, "a paid error settles exactly like the same 2xx receipt")
				require.Equal(t, paid.charged, paid.logQuota, "the consume log records the settled charge")
				require.True(t, paid.settled, "a settled paid attempt must never be replayed on another channel")

				plain := relayPaidErrorAttempt(t, channel, modelName, protocol, paidErrorPlain)
				require.True(t, plain.failed)
				require.Positive(t, plain.held)
				require.Equal(t, plain.held, plain.charged, "an error without usage keeps only the retained admission hold")
				require.Less(t, plain.charged, control.charged, "control: the hold is smaller than the receipt")
				require.Zero(t, plain.logQuota)
				require.False(t, plain.settled, "an unpaid provider error stays retryable")
			})
		}
	}
}

// TestPaidUpstreamErrorReceiptInMCPRound proves one MCP tool-loop round keeps a
// paid provider error's receipt (so the loop retains rather than refunds the
// round), while a plain provider error still returns no usage.
func TestPaidUpstreamErrorReceiptInMCPRound(t *testing.T) {
	for _, reply := range []paidErrorReply{paidErrorWithReceipt, paidErrorPlain} {
		xaiVideoSetup(t, 1_000_000, false)
		var calls atomic.Int32
		upstream := paidErrorUpstream(t, reply, "gpt-4o-mini", &calls)
		previous := client.HTTPClient
		client.HTTPClient = upstream.Client()
		path, body := lyriaProtocolRequest("chat", false)
		c, _, _ := protocolContext(t, channeltype.OpenAI, "gpt-4o-mini", path, body, upstream.URL, 1_000_000, 1, false, nil)
		m := metalib.GetByContext(c)
		m.Mode = relaymode.ChatCompletions
		m.ActualModelName = "gpt-4o-mini"
		request := relaymodel.GeneralOpenAIRequest{Model: "gpt-4o-mini", Messages: []relaymodel.Message{{Role: "user", Content: "hello"}}}
		_, usage, apiErr := doChatRequestOnce(c, m, &openai.Adaptor{}, &request)
		client.HTTPClient = previous
		require.NotNil(t, apiErr)
		require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
		require.EqualValues(t, 1, calls.Load())
		if reply == paidErrorPlain {
			require.Nil(t, usage, "a plain provider error proves no paid work")
			continue
		}
		require.NotNil(t, usage, "the paid round's receipt must reach the MCP loop")
		require.Equal(t, paidErrorPrompt, usage.PromptTokens)
		require.Equal(t, paidErrorCompletion, usage.CompletionTokens)
	}
}
