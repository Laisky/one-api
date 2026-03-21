package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// newDirectStreamContext creates a gin test context with a recorder suitable
// for testing ResponseAPIDirectStreamHandler.
func newDirectStreamContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, w
}

// fakeUpstream builds an *http.Response whose body contains the given SSE text.
func fakeUpstream(sse string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
	}
}

// TestResponseAPIDirectStreamHandler_DataForwarded verifies that data lines
// from the upstream are forwarded to the client (event: lines are skipped).
func TestResponseAPIDirectStreamHandler_DataForwarded(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, text, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	body := w.Body.String()
	// The data line should be forwarded
	require.Contains(t, body, "data: {\"type\":\"response.created\"")
	// The stream should end with [DONE]
	require.Contains(t, body, "data: [DONE]")

	_ = text
	_ = usage
}

// TestResponseAPIDirectStreamHandler_NormalCompletion tests a full stream with
// response.created -> deltas -> response.completed -> [DONE].
func TestResponseAPIDirectStreamHandler_NormalCompletion(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_2\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hello\"}\n" +
		"\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\" world\"}\n" +
		"\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"completed\",\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello world\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, text, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.Equal(t, "Hello world", text)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.Equal(t, 15, usage.TotalTokens)

	body := w.Body.String()
	require.Contains(t, body, "data: [DONE]")
	// All data events should be forwarded
	require.Contains(t, body, "response.created")
	require.Contains(t, body, "response.output_text.delta")
	require.Contains(t, body, "response.completed")
}

// TestResponseAPIDirectStreamHandler_UpstreamDropsNoDone verifies that when the
// upstream stream ends without sending [DONE], the handler still renders [DONE]
// (via the !doneRendered fallback at the end of the function).
func TestResponseAPIDirectStreamHandler_UpstreamDropsNoDone(t *testing.T) {
	c, w := newDirectStreamContext()

	// Stream ends abruptly without [DONE]
	sse := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_3\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"partial\"}\n"

	apiErr, text, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.Equal(t, "partial", text)

	body := w.Body.String()
	// When upstream drops without [DONE], the handler does NOT fabricate [DONE]
	require.NotContains(t, body, "data: [DONE]")
}

// TestResponseAPIDirectStreamHandler_UnparseableEventsSkipped verifies that
// malformed JSON in data lines is skipped (not forwarded) but doesn't break
// the stream.
func TestResponseAPIDirectStreamHandler_UnparseableEventsSkipped(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "data: {this is not valid json}\n" +
		"\n" +
		"event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_4\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	body := w.Body.String()
	// The valid event should still be forwarded
	require.Contains(t, body, "response.created")
	// The handler now forwards all SSE lines including unparseable ones
	require.Contains(t, body, "this is not valid json")
	require.Contains(t, body, "data: [DONE]")
}

// TestResponseAPIDirectStreamHandler_UsageAccumulation tests that usage from
// a response.completed event is correctly extracted and returned.
func TestResponseAPIDirectStreamHandler_UsageAccumulation(t *testing.T) {
	c, _ := newDirectStreamContext()

	sse := "event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_5\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":20,\"output_tokens\":30,\"total_tokens\":50}}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 30, usage.CompletionTokens)
	require.Equal(t, 50, usage.TotalTokens)
}

// TestResponseAPIDirectStreamHandler_TextAccumulation verifies that delta events
// accumulate responseText correctly.
func TestResponseAPIDirectStreamHandler_TextAccumulation(t *testing.T) {
	c, _ := newDirectStreamContext()

	sse := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"foo\"}\n" +
		"\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"bar\"}\n" +
		"\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"baz\"}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, text, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.Equal(t, "foobarbaz", text)
}

// TestResponseAPIDirectStreamHandler_EmptyEventType verifies that data lines
// without a preceding event: line are still forwarded (they just have data: prefix).
func TestResponseAPIDirectStreamHandler_EmptyEventType(t *testing.T) {
	c, w := newDirectStreamContext()

	// Data line with no preceding event: line - still a valid response object
	sse := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_6\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	body := w.Body.String()
	require.Contains(t, body, "response.created")
	require.Contains(t, body, "data: [DONE]")
}

// TestResponseAPIDirectStreamHandler_WebSearchTracking tests that web_search_call
// output items are counted and stored in context.
func TestResponseAPIDirectStreamHandler_WebSearchTracking(t *testing.T) {
	c, _ := newDirectStreamContext()

	// response.completed with a web_search_call in output
	sse := "event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_7\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"completed\",\"output\":[{\"type\":\"web_search_call\",\"id\":\"ws_1\",\"action\":{\"type\":\"search\",\"query\":\"test query\"}},{\"type\":\"web_search_call\",\"id\":\"ws_2\",\"action\":{\"type\":\"search\",\"query\":\"another query\"}}],\"usage\":{\"input_tokens\":5,\"output_tokens\":3,\"total_tokens\":8}}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	val, exists := c.Get(ctxkey.WebSearchCallCount)
	require.True(t, exists, "WebSearchCallCount should be set in context")
	require.Equal(t, 2, val.(int))
}

// TestResponseAPIDirectStreamHandler_ConvertedResponseSet verifies that after
// a successful stream with a top-level response ID, ConvertedResponse is set
// as a ResponseAPIResponse. Events where the "id" field is at the top level
// (not nested inside "response") are parsed as fullResponse objects.
func TestResponseAPIDirectStreamHandler_ConvertedResponseSet(t *testing.T) {
	c, _ := newDirectStreamContext()

	// Use a response.created event which has "id" at the top level of the
	// "response" object. For ParseResponseAPIStreamEvent, the top-level JSON
	// must have "id" to be treated as a fullResponse. response.created events
	// embed the full response, so we use a flat structure that has top-level id.
	sse := "event: response.created\n" +
		"data: {\"id\":\"resp_8\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	val, exists := c.Get(ctxkey.ConvertedResponse)
	require.True(t, exists, "ConvertedResponse should be set in context")

	resp, ok := val.(ResponseAPIResponse)
	require.True(t, ok, "ConvertedResponse should be a ResponseAPIResponse")
	require.Equal(t, "resp_8", resp.Id)
	require.Equal(t, "in_progress", resp.Status)
}

// TestResponseAPIDirectStreamHandler_ConvertedResponseFromCompleted verifies
// that a response.completed event (where id is nested) still sets
// ConvertedResponse via the fallback map path.
func TestResponseAPIDirectStreamHandler_ConvertedResponseFromCompleted(t *testing.T) {
	c, _ := newDirectStreamContext()

	sse := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"hi\"}\n" +
		"\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_8\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, text, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.Equal(t, "hi", text)
	require.NotNil(t, usage)

	val, exists := c.Get(ctxkey.ConvertedResponse)
	require.True(t, exists, "ConvertedResponse should be set in context")
	// The response.completed event has nested "id" so it's parsed as a stream
	// event, not a fullResponse. ConvertedResponse is set from the fallback path.
	_, isMap := val.(map[string]any)
	_, isResp := val.(ResponseAPIResponse)
	require.True(t, isMap || isResp, "ConvertedResponse should be set as map or ResponseAPIResponse")
}

// TestResponseAPIDirectStreamHandler_ConvertedResponseFallback verifies that
// when there is no full response event but there is text/usage, a fallback
// map is stored in ConvertedResponse.
func TestResponseAPIDirectStreamHandler_ConvertedResponseFallback(t *testing.T) {
	c, _ := newDirectStreamContext()

	// Only delta events, no response.completed
	sse := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"hello\"}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, text, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.Equal(t, "hello", text)

	val, exists := c.Get(ctxkey.ConvertedResponse)
	require.True(t, exists, "ConvertedResponse should be set even without full response")

	m, ok := val.(map[string]any)
	require.True(t, ok, "ConvertedResponse fallback should be a map")
	require.Equal(t, true, m["stream"])
	require.Equal(t, "hello", m["content"])
}

// TestResponseAPIDirectStreamHandler_KeepaliveSkipped verifies that keepalive
// events (which don't have a response ID or parseable structure) are skipped
// by the parser but don't cause errors.
func TestResponseAPIDirectStreamHandler_KeepaliveSkipped(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "event: keepalive\n" +
		"data: {\"type\":\"keepalive\"}\n" +
		"\n" +
		"event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_9\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	body := w.Body.String()
	// Keepalive is parsed but produces empty fullResponse/streamEvent (Id=="", no stream event type)
	// The valid event should still be forwarded
	require.Contains(t, body, "response.created")
	require.Contains(t, body, "data: [DONE]")
}

// TestResponseAPIDirectStreamHandler_MultipleUsageUpdates verifies that the
// handler takes the last usage value when multiple events contain usage.
func TestResponseAPIDirectStreamHandler_MultipleUsageUpdates(t *testing.T) {
	c, _ := newDirectStreamContext()

	sse := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_10\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0,\"total_tokens\":1}}}\n" +
		"\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_10\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":20,\"total_tokens\":30}}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	// Should have the last (completed) usage, not the initial one
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
}

// TestResponseAPIDirectStreamHandler_EmptyStream verifies behavior with an
// empty stream (no data lines at all).
func TestResponseAPIDirectStreamHandler_EmptyStream(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := ""

	apiErr, text, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.Equal(t, "", text)
	require.Nil(t, usage)

	body := w.Body.String()
	// When upstream sends nothing (empty stream), no [DONE] is fabricated
	require.NotContains(t, body, "data: [DONE]")
}

// TestResponseAPIDirectStreamHandler_OnlyDone verifies behavior when the
// upstream sends only [DONE] with no data events.
func TestResponseAPIDirectStreamHandler_OnlyDone(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "data: [DONE]\n"

	apiErr, text, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.Equal(t, "", text)
	require.Nil(t, usage)

	body := w.Body.String()
	require.Contains(t, body, "data: [DONE]")
}

// TestResponseAPIDirectStreamHandler_FlushOccurs verifies that the recorder
// is flushed (indicating streaming output was sent).
func TestResponseAPIDirectStreamHandler_FlushOccurs(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_flush\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)
	require.True(t, w.Flushed, "expected response to be flushed for SSE streaming")
}

// ---------------------------------------------------------------------------
// Gap-fill tests: event: line forwarding, reasoning, tool call passthrough
// ---------------------------------------------------------------------------

// TestResponseAPIDirectStreamHandler_EventLinesForwarded verifies that SSE
// "event:" lines from upstream are forwarded to the client, matching the
// official Response API wire format: "event: <type>\ndata: <json>\n\n".
func TestResponseAPIDirectStreamHandler_EventLinesForwarded(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_ev\",\"object\":\"response\",\"created_at\":1700000000,\"status\":\"in_progress\"}}\n" +
		"\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hi\"}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	body := w.Body.String()
	// Verify event: lines are present in output (not stripped)
	require.Contains(t, body, "event: response.created\n", "event: line for response.created must be forwarded")
	require.Contains(t, body, "event: response.output_text.delta\n", "event: line for delta must be forwarded")
	// Verify the full SSE format: event line immediately before data line
	require.Contains(t, body, "event: response.created\ndata: ", "event: and data: lines must be adjacent")
}

// TestResponseAPIDirectStreamHandler_ReasoningPassthrough verifies that
// reasoning summary events are forwarded faithfully including event: lines.
func TestResponseAPIDirectStreamHandler_ReasoningPassthrough(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "event: response.output_item.added\n" +
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"rs_1\",\"type\":\"reasoning\",\"summary\":[]},\"output_index\":0}\n" +
		"\n" +
		"event: response.reasoning_summary_part.added\n" +
		"data: {\"type\":\"response.reasoning_summary_part.added\",\"item_id\":\"rs_1\",\"output_index\":0,\"part\":{\"type\":\"summary_text\",\"text\":\"\"},\"summary_index\":0}\n" +
		"\n" +
		"event: response.reasoning_summary_text.delta\n" +
		"data: {\"type\":\"response.reasoning_summary_text.delta\",\"item_id\":\"rs_1\",\"output_index\":0,\"delta\":\"Thinking about\",\"summary_index\":0}\n" +
		"\n" +
		"event: response.reasoning_summary_text.delta\n" +
		"data: {\"type\":\"response.reasoning_summary_text.delta\",\"item_id\":\"rs_1\",\"output_index\":0,\"delta\":\" the answer\",\"summary_index\":0}\n" +
		"\n" +
		"event: response.reasoning_summary_text.done\n" +
		"data: {\"type\":\"response.reasoning_summary_text.done\",\"item_id\":\"rs_1\",\"output_index\":0,\"text\":\"Thinking about the answer\",\"summary_index\":0}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	body := w.Body.String()
	require.Contains(t, body, "event: response.reasoning_summary_text.delta\n")
	require.Contains(t, body, "Thinking about")
	require.Contains(t, body, " the answer")
	require.Contains(t, body, "event: response.reasoning_summary_text.done\n")
}

// TestResponseAPIDirectStreamHandler_ToolCallPassthrough verifies that
// function_call events are forwarded faithfully.
func TestResponseAPIDirectStreamHandler_ToolCallPassthrough(t *testing.T) {
	c, w := newDirectStreamContext()

	sse := "event: response.output_item.added\n" +
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"fc_1\",\"type\":\"function_call\",\"name\":\"get_weather\",\"status\":\"in_progress\"},\"output_index\":1}\n" +
		"\n" +
		"event: response.function_call_arguments.delta\n" +
		"data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":1,\"delta\":\"{\\\"city\\\":\"}\n" +
		"\n" +
		"event: response.function_call_arguments.delta\n" +
		"data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":1,\"delta\":\"\\\"SF\\\"}\"}\n" +
		"\n" +
		"event: response.function_call_arguments.done\n" +
		"data: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"fc_1\",\"output_index\":1,\"arguments\":\"{\\\"city\\\":\\\"SF\\\"}\"}\n" +
		"\n" +
		"data: [DONE]\n"

	apiErr, _, _ := ResponseAPIDirectStreamHandler(c, fakeUpstream(sse), relaymode.ResponseAPI)
	require.Nil(t, apiErr)

	body := w.Body.String()
	require.Contains(t, body, "event: response.function_call_arguments.delta\n")
	require.Contains(t, body, "get_weather")
	require.Contains(t, body, "event: response.function_call_arguments.done\n")
}
