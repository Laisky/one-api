package openai

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// rawReceiptPromptTokens is the gateway prompt estimate every lifecycle
// fixture admits; settlement must never fall below it on an estimate.
const rawReceiptPromptTokens = 10

// rawReceiptCompleteUsage is the authoritative Standard Responses receipt
// carried by terminal fixtures (20 input and 2 output tokens).
const rawReceiptCompleteUsage = `{"input_tokens":20,"output_tokens":2,"total_tokens":22}`

// rawReceiptMode names one client rendering of the same native Cyber stream.
type rawReceiptMode struct {
	name string
	mode int
}

// rawReceiptModes lists the native Responses passthrough and both converted
// client formats (Chat Completions and Claude Messages) served by DoResponse.
var rawReceiptModes = []rawReceiptMode{
	{name: "responses", mode: relaymode.ResponseAPI},
	{name: "chat", mode: relaymode.ChatCompletions},
	{name: "messages", mode: relaymode.ClaudeMessages},
}

// rawReceiptCompletionMarkers are client-visible completion signals that an
// interrupted stream must never fabricate in any of the three formats.
var rawReceiptCompletionMarkers = []string{
	"response.completed", "[DONE]", "message_stop", `"finish_reason":"stop"`, `"stop_reason":"end_turn"`,
}

// rawReceiptDelta renders one Responses output_text delta SSE frame.
// Parameters: text is the delta. Returns: the SSE frame.
func rawReceiptDelta(text string) string {
	payload, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "item_id": "msg_lifecycle",
		"output_index": 0, "content_index": 0, "delta": text})
	return "event: response.output_text.delta\ndata: " + string(payload) + "\n\n"
}

// rawReceiptProvisional renders the response.created and response.in_progress
// lifecycle frames whose placeholder usage is not a receipt. It takes no
// parameters and returns both SSE frames.
func rawReceiptProvisional() string {
	var out strings.Builder
	for _, kind := range []string{"response.created", "response.in_progress"} {
		out.WriteString("event: " + kind + "\ndata: {\"type\":\"" + kind + "\",\"response\":{\"id\":\"resp_lifecycle\",\"object\":\"response\"," +
			"\"model\":\"gpt-6-cyber\",\"status\":\"in_progress\",\"service_tier\":\"default\",\"output\":[]," +
			"\"usage\":{\"input_tokens\":20,\"output_tokens\":0,\"total_tokens\":20}}}\n\n")
	}
	return out.String()
}

// rawReceiptCompleted renders a terminal response.completed frame. Parameters:
// text is the final assistant text and usage the receipt JSON. Returns: the frame.
func rawReceiptCompleted(text, usage string) string {
	encoded, _ := json.Marshal(text)
	return "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lifecycle\"," +
		"\"object\":\"response\",\"model\":\"gpt-6-cyber\",\"status\":\"completed\",\"service_tier\":\"default\"," +
		"\"output\":[{\"type\":\"message\",\"id\":\"msg_lifecycle\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":" +
		string(encoded) + "}]}],\"usage\":" + usage + "}}\n\n"
}

// rawReceiptContext prepares one native gpt-6-cyber attempt rendered for mode.
// Parameters: mode is the client relay mode and stream selects SSE delivery.
// Returns: the gin context, its response recorder and the attempt metadata.
func rawReceiptContext(mode int, stream bool) (*gin.Context, *httptest.ResponseRecorder, *metalib.Meta) {
	c, w := newDirectStreamContext()
	info := &metalib.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6-cyber", OriginModelName: "gpt-6-cyber",
		Mode: mode, IsStream: stream, PromptTokens: rawReceiptPromptTokens}
	metalib.Set2Context(c, info)
	// The native Cyber route serves Chat Completions clients from Responses.
	c.Set(ctxkey.ConvertedRequest, &ResponseAPIRequest{Model: "gpt-6-cyber"})
	if mode == relaymode.ClaudeMessages {
		c.Set(ctxkey.ClaudeMessagesConversion, true)
	}
	return c, w, info
}

// rawReceiptScriptedBody replays a fixed byte script in small reads and then
// fails every later read with err, modelling a provider body that is cut off.
// Reads and closes are serialized so the body itself never races.
type rawReceiptScriptedBody struct {
	mu     sync.Mutex
	data   []byte
	chunk  int
	err    error
	closes atomic.Int32
}

// newRawReceiptScriptedBody creates a scripted body. Parameters: script is the
// delivered prefix, chunk bounds each read, and err ends the body. Returns: the body.
func newRawReceiptScriptedBody(script string, chunk int, err error) *rawReceiptScriptedBody {
	return &rawReceiptScriptedBody{data: []byte(script), chunk: chunk, err: err}
}

// Read copies at most chunk script bytes into p, then returns the terminal
// error. Parameters: p is the destination. Returns: bytes copied and the error.
func (b *rawReceiptScriptedBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.data) == 0 {
		return 0, b.err
	}
	n := copy(p, b.data[:min(len(b.data), b.chunk)])
	b.data = b.data[n:]
	return n, nil
}

// Close records the release. It takes no parameters and returns nil.
func (b *rawReceiptScriptedBody) Close() error {
	b.closes.Add(1)
	return nil
}

// rawReceiptStreamResponse wraps body as a 200 SSE provider response.
// Parameters: body is the provider body. Returns: the response.
func rawReceiptStreamResponse(body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: body,
		Header: http.Header{"Content-Type": {"text/event-stream"}}}
}

// rawReceiptConnReset is the error a kernel-reset TCP connection surfaces.
func rawReceiptConnReset() error {
	return &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}
}

// requireRawReceiptEstimate asserts the settlement contract of an interrupted
// stream: a flagged estimate that counts delivered output and never falls
// below the admitted prompt or a provisional input snapshot. Parameters: t
// asserts, usage is the settlement usage, minPrompt the prompt floor, and tier
// the envelope service tier delivered before the cut (empty when none was).
func requireRawReceiptEstimate(t *testing.T, usage *model.Usage, minPrompt int, tier string) {
	t.Helper()
	require.NotNil(t, usage, "an interrupted paid stream must reach settlement")
	require.NotEmpty(t, usage.BillingEstimateReason, "an interrupted receipt must be an estimate, not a complete-looking charge")
	require.Positive(t, usage.CompletionTokens, "delivered deltas must be counted")
	require.GreaterOrEqual(t, usage.PromptTokens, minPrompt)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.Equal(t, tier, usage.ServiceTier, "the observed envelope tier must survive the interruption")
}

// requireRawReceiptDelivery asserts the client saw every delivered delta and
// no fabricated completion. Parameters: t asserts, body is the client bytes,
// and deltas the texts that were delivered before the interruption.
func requireRawReceiptDelivery(t *testing.T, body string, deltas ...string) {
	t.Helper()
	for _, delta := range deltas {
		require.Contains(t, body, delta, "a delivered delta must reach the client")
	}
	for _, marker := range rawReceiptCompletionMarkers {
		require.NotContains(t, body, marker, "an interrupted stream must not be disguised as complete")
	}
}

// requireRawReceiptTerminalError asserts the adaptor reports the transport
// interruption for formats whose handlers own the stream lifecycle. The Claude
// converter's documented contract is to end the client stream without
// message_stop and return nil; its settlement remains a flagged estimate.
// Parameters: t asserts, mode is the client mode, apiErr the adaptor error.
func requireRawReceiptTerminalError(t *testing.T, mode int, apiErr *model.ErrorWithStatusCode) {
	t.Helper()
	if mode == relaymode.ClaudeMessages {
		require.Nil(t, apiErr, "the Claude converter reports interruptions through its estimate")
		return
	}
	require.NotNil(t, apiErr, "a provider interruption must be reported, not converted to success")
	require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	require.Equal(t, "read_stream_failed", apiErr.Error.Code)
}

// requireRawReceiptClaudeStored asserts the converted Claude usage the
// controller renders and settles is the same receipt DoResponse returned.
// Parameters: t asserts, c is the request context and usage the result.
func requireRawReceiptClaudeStored(t *testing.T, c *gin.Context, usage *model.Usage) {
	t.Helper()
	stored, ok := c.Get(ctxkey.ConvertedResponseUsage)
	require.True(t, ok, "the Claude rendering must publish its settlement receipt")
	receipt, ok := stored.(*model.Usage)
	require.True(t, ok)
	require.NotNil(t, receipt)
	require.Equal(t, *usage, *receipt)
}

// TestRawUsageReceiptInterruptedStreamLifecycle drives one native Cyber SSE
// stream that the provider transport cuts off mid-event (unexpected EOF or a
// TCP reset) through DoResponse for the native Responses client and both
// converted client formats. The client must receive every delivered delta and
// no fabricated completion; settlement must receive a flagged estimate that is
// never the provisional lifecycle snapshot. A reset after the terminal receipt
// is the control: it loses no billable work and settles the exact receipt.
func TestRawUsageReceiptInterruptedStreamLifecycle(t *testing.T) {
	// The truncated terminal frame ends mid-JSON, after its usage began.
	truncatedTerminal := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lifecycle\",\"status\":\"completed\",\"usage\":{\"input_tokens\":20,"
	deltas := rawReceiptDelta("partial work ") + rawReceiptDelta("before the reset")
	for _, errCase := range []struct {
		name string
		err  func() error
	}{
		{name: "unexpected_eof", err: func() error { return io.ErrUnexpectedEOF }},
		{name: "connection_reset", err: rawReceiptConnReset},
	} {
		for _, script := range []struct {
			name, body, tier string
			minPrompt        int
			exact            bool
		}{
			{name: "deltas_only", body: deltas, minPrompt: rawReceiptPromptTokens},
			{name: "provisional_usage_then_deltas", body: rawReceiptProvisional() + deltas, minPrompt: 20, tier: "default"},
			{name: "cut_inside_terminal_event", body: rawReceiptProvisional() + deltas + truncatedTerminal, minPrompt: 20, tier: "default"},
			{name: "reset_after_terminal_receipt", exact: true,
				body: rawReceiptProvisional() + deltas + rawReceiptCompleted("partial work before the reset", rawReceiptCompleteUsage)},
		} {
			for _, mode := range rawReceiptModes {
				t.Run(errCase.name+"/"+script.name+"/"+mode.name, func(t *testing.T) {
					c, w, info := rawReceiptContext(mode.mode, true)
					body := newRawReceiptScriptedBody(script.body, 17, errCase.err())
					usage, apiErr := (&Adaptor{ChannelType: channeltype.OpenAI}).DoResponse(c, rawReceiptStreamResponse(body), info)
					require.Positive(t, body.closes.Load(), "the provider body must be released")
					delivered := w.Body.String()
					if script.exact {
						require.NotNil(t, usage)
						require.Empty(t, usage.BillingEstimateReason, "a reset after the terminal receipt loses no billable work")
						require.Equal(t, 20, usage.PromptTokens)
						require.Equal(t, 2, usage.CompletionTokens)
						require.Contains(t, delivered, "partial work ")
						if mode.mode == relaymode.ClaudeMessages {
							require.Contains(t, delivered, "message_stop", "the real terminal event completes the Claude stream")
							requireRawReceiptClaudeStored(t, c, usage)
						}
						return
					}
					requireRawReceiptTerminalError(t, mode.mode, apiErr)
					requireRawReceiptDelivery(t, delivered, "partial work ", "before the reset")
					requireRawReceiptEstimate(t, usage, script.minPrompt, script.tier)
					if mode.mode == relaymode.ClaudeMessages {
						requireRawReceiptClaudeStored(t, c, usage)
					}
				})
			}
		}
	}
}
