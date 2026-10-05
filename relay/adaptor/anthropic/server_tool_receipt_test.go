package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

// TestParseServerToolUse verifies counter mapping and strict counter validation.
func TestParseServerToolUse(t *testing.T) {
	t.Parallel()
	counters, err := parseServerToolUse(json.RawMessage(`{"web_search_requests":2,"web_fetch_requests":1,"code_execution_requests":0,"future_tool_requests":3,"note":"ignored","nested":{"a":1}}`))
	require.NoError(t, err)
	require.Equal(t, map[string]int{"web_search": 2, "web_fetch": 1, "code_execution": 0, "future_tool": 3}, counters)

	counters, err = parseServerToolUse(json.RawMessage(`null`))
	require.NoError(t, err)
	require.Nil(t, counters)

	_, err = parseServerToolUse(json.RawMessage(`{"web_search_requests":-1}`))
	require.ErrorContains(t, err, "negative")
	_, err = parseServerToolUse(json.RawMessage(`{"web_search_requests":1.5}`))
	require.Error(t, err)
	_, err = parseServerToolUse(json.RawMessage(`[]`))
	require.Error(t, err)
}

// TestHTTPUsageReceiptServerToolsCumulative verifies stream receipts merge by
// maximum: duplicates never add up and omitted counters never erase earlier ones.
func TestHTTPUsageReceiptServerToolsCumulative(t *testing.T) {
	t.Parallel()
	var receipt httpUsageReceipt
	for _, raw := range []string{
		`{"input_tokens":11,"output_tokens":1,"server_tool_use":{"web_search_requests":0}}`,
		`{"output_tokens":4,"server_tool_use":{"web_search_requests":1}}`,
		`{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}`,
		`{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}`,
		`{"output_tokens":7}`,
		`{"output_tokens":7,"server_tool_use":{"web_search_requests":1}}`,
	} {
		require.NoError(t, receipt.apply(json.RawMessage(raw)))
	}
	require.Equal(t, map[string]int{"web_search": 2}, receipt.tools.counts())

	// A rejected receipt leaves the earlier tool counters untouched.
	require.Error(t, receipt.apply(json.RawMessage(`{"output_tokens":-1,"server_tool_use":{"web_search_requests":9}}`)))
	require.Error(t, receipt.apply(json.RawMessage(`{"server_tool_use":{"web_search_requests":-9}}`)))
	require.Equal(t, map[string]int{"web_search": 2}, receipt.tools.counts())
}

// TestServerToolTallyBlockFallback verifies observed invocation blocks are
// billed only for tools whose authoritative receipt counter never appeared.
func TestServerToolTallyBlockFallback(t *testing.T) {
	t.Parallel()
	var tally serverToolTally
	tally.observeContent(json.RawMessage(`[
		{"type":"server_tool_use","name":"web_search"},
		{"type":"server_tool_use","name":"web_search"},
		{"type":"server_tool_use","name":"bash_code_execution"},
		{"type":"server_tool_use","name":"tool_search_tool_regex"},
		{"type":"mcp_tool_use","name":"remote_lookup"},
		{"type":"tool_use","name":"web_search"},
		{"type":"text","text":"ok"}]`))
	require.Equal(t, map[string]int{"web_search": 2, "code_execution": 1, "tool_search": 1, "mcp_connector": 1}, tally.counts())

	// An explicit receipt is authoritative, even when lower (failed searches are free).
	tally = tally.withReceipts(map[string]int{"web_search": 1})
	require.Equal(t, 1, tally.counts()["web_search"])
	tally = tally.withReceipts(map[string]int{"code_execution": 0})
	require.NotContains(t, tally.counts(), "code_execution")

	var malformed serverToolTally
	malformed.observeContent(json.RawMessage(`"not an array"`))
	require.Empty(t, malformed.counts())
}

// TestRecordServerToolInvocationsAddsMessages verifies separate upstream
// messages (for example MCP loop rounds) add to existing counters.
func TestRecordServerToolInvocationsAddsMessages(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.ToolInvocationCounts, map[string]any{"web_search": 1, "file_search": float64(2)})
	RecordServerToolInvocations(c, map[string]int{"web_search": 2, "web_fetch": 0})
	require.NoError(t, RecordServerToolUseFromJSON(c, []byte(`{"content":[{"type":"server_tool_use","name":"web_fetch"}],"usage":{"server_tool_use":{"web_search_requests":1}}}`)))
	raw, ok := c.Get(ctxkey.ToolInvocationCounts)
	require.True(t, ok)
	require.Equal(t, map[string]int{"web_search": 4, "file_search": 2, "web_fetch": 1}, raw)

	err := RecordServerToolUseFromJSON(c, []byte(`{"content":[{"type":"server_tool_use","name":"web_search"}],"usage":{"server_tool_use":{"web_search_requests":-1}}}`))
	require.Error(t, err, "a malformed receipt is reported")
	raw, _ = c.Get(ctxkey.ToolInvocationCounts)
	require.Equal(t, 5, raw.(map[string]int)["web_search"], "observed invocations stay billable")
}

// TestClaudeNativeHandlersRecordServerTools drives the real JSON and SSE
// handlers and checks the request-scoped invocation counters they publish.
func TestClaudeNativeHandlersRecordServerTools(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	jsonBody := `{"id":"msg","type":"message","role":"assistant","model":"claude","content":[{"type":"server_tool_use","id":"s1","name":"web_search","input":{}},{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2,"server_tool_use":{"web_search_requests":1}}}`
	sseBody := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg","type":"message","role":"assistant","model":"claude","content":[],"usage":{"input_tokens":3,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"s1","name":"web_search","input":{}}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"
	for _, tc := range []struct {
		name   string
		stream bool
		body   string
	}{
		{name: "json_receipt", body: jsonBody},
		{name: "sse_missing_receipt_counts_block", stream: true, body: sseBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}
			if tc.stream {
				apiErr, usage := ClaudeNativeStreamHandler(c, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
			} else {
				apiErr, usage := ClaudeNativeHandler(c, resp, 0, "claude")
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
			}
			raw, ok := c.Get(ctxkey.ToolInvocationCounts)
			require.True(t, ok)
			require.Equal(t, map[string]int{"web_search": 1}, raw)
		})
	}
}
