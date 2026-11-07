package openai_compatible

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/common/claudeconformance"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// convertClaudeStreamForTest runs ConvertOpenAIStreamToClaudeSSE over the
// given upstream SSE lines and returns the raw Claude SSE output.
func convertClaudeStreamForTest(t *testing.T, lines ...string) string {
	t.Helper()
	c, w := newGinTestContext()
	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{"mcp__fs_read": "mcp__fs.read"})
	body := strings.Join(lines, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	return w.Body.String()
}

// TestClaudeStreamChatToolCallsKeyedByIndex reproduces the OpenAI streaming
// shape where only the first fragment of each tool call carries id and name:
// every later fragment must extend the same tool_use block instead of opening
// a new nameless block per fragment.
func TestClaudeStreamChatToolCallsKeyedByIndex(t *testing.T) {
	t.Parallel()
	out := convertClaudeStreamForTest(t,
		`data: {"choices":[{"index":0,"delta":{"content":"Checking."}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"mcp__fs_read","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	)
	msg := requireConformantClaudeStream(t, out)
	require.Equal(t, "Checking.", msg.Text())
	tools := msg.ToolUses()
	require.Len(t, tools, 2)
	require.Equal(t, "call_a", tools[0].ID)
	require.Equal(t, "get_weather", tools[0].Name)
	require.JSONEq(t, `{"city":"Paris"}`, tools[0].Input)
	require.Equal(t, "call_b", tools[1].ID)
	require.Equal(t, "mcp__fs.read", tools[1].Name, "sanitized tool names must be restored")
	require.Equal(t, "tool_use", msg.StopReason)
}

// TestClaudeStreamChatFinishReasonMapping verifies finish_reason maps to the
// Claude stop_reason enum, including when the upstream omits [DONE].
func TestClaudeStreamChatFinishReasonMapping(t *testing.T) {
	t.Parallel()
	for finish, want := range map[string]string{
		"stop":           "end_turn",
		"length":         "max_tokens",
		"content_filter": "refusal",
		"tool_calls":     "end_turn", // no tool_use block was produced
	} {
		t.Run(finish, func(t *testing.T) {
			t.Parallel()
			out := convertClaudeStreamForTest(t,
				`data: {"choices":[{"index":0,"delta":{"content":"partial"}}]}`,
				`data: {"choices":[{"index":0,"delta":{},"finish_reason":"`+finish+`"}]}`,
			)
			msg := requireConformantClaudeStream(t, out)
			require.Equal(t, want, msg.StopReason)
		})
	}
}

// TestClaudeStreamChatStopWithToolsBecomesToolUse covers providers that report
// finish_reason "stop" for a turn that requests tools.
func TestClaudeStreamChatStopWithToolsBecomesToolUse(t *testing.T) {
	t.Parallel()
	out := convertClaudeStreamForTest(t,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call.with:bad/chars","function":{"name":"lookup","arguments":"{\"q\":1}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call.with:bad/chars","function":{"name":"lookup","arguments":"{\"q\":2}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	)
	msg := requireConformantClaudeStream(t, out)
	require.Equal(t, "tool_use", msg.StopReason)
	tools := msg.ToolUses()
	require.Len(t, tools, 2)
	require.Equal(t, "call_with_bad_chars", tools[0].ID)
	require.Equal(t, "call_with_bad_chars_2", tools[1].ID)
}

// TestClaudeStreamChatReusedIndexDistinctIDs keeps two calls apart when a
// provider reuses stream index 0 for parallel calls with distinct ids, and
// buffers arguments that arrive before the function name.
func TestClaudeStreamChatReusedIndexDistinctIDs(t *testing.T) {
	t.Parallel()
	out := convertClaudeStreamForTest(t,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"arguments":"{\"a\":"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"first","arguments":"1}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_2","function":{"name":"second","arguments":"{\"b\":2}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	)
	msg := requireConformantClaudeStream(t, out)
	tools := msg.ToolUses()
	require.Len(t, tools, 2)
	require.Equal(t, "first", tools[0].Name)
	require.JSONEq(t, `{"a":1}`, tools[0].Input)
	require.Equal(t, "second", tools[1].Name)
	require.JSONEq(t, `{"b":2}`, tools[1].Input)
}

// TestClaudeStreamResponsesSnapshotOnlyItems renders items that an upstream
// delivers only as output_item.done snapshots exactly once, and closes each
// function_call block at its output_item.done.
func TestClaudeStreamResponsesSnapshotOnlyItems(t *testing.T) {
	t.Parallel()
	out := convertClaudeStreamForTest(t,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"Snapshot text"}]}}`,
		`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"fc_1","type":"function_call","call_id":"call_x","name":"mcp__fs_read","arguments":""}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[`+
			`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"Snapshot text"}]},`+
			`{"id":"fc_1","type":"function_call","call_id":"call_x","name":"mcp__fs_read","arguments":""}],"usage":{"input_tokens":3,"output_tokens":4}}}`,
	)
	msg := requireConformantClaudeStream(t, out)
	require.Equal(t, "Snapshot text", msg.Text())
	tools := msg.ToolUses()
	require.Len(t, tools, 1)
	require.Equal(t, "call_x", tools[0].ID)
	require.Equal(t, "mcp__fs.read", tools[0].Name)
	require.JSONEq(t, `{}`, tools[0].Input)
	require.Equal(t, "tool_use", msg.StopReason)
}

// TestClaudeStreamResponsesIncompleteReasons maps response.incomplete reasons.
func TestClaudeStreamResponsesIncompleteReasons(t *testing.T) {
	t.Parallel()
	for reason, want := range map[string]string{"max_output_tokens": "max_tokens", "content_filter": "refusal"} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			out := convertClaudeStreamForTest(t,
				`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"cut"}`,
				`data: {"type":"response.incomplete","response":{"id":"resp_1","object":"response","status":"incomplete","incomplete_details":{"reason":"`+reason+`"},"output":[]}}`,
			)
			msg := requireConformantClaudeStream(t, out)
			require.Equal(t, "cut", msg.Text())
			require.Equal(t, want, msg.StopReason)
		})
	}
}

// TestClaudeStreamResponsesFailedEmitsError verifies a failed upstream response
// surfaces as an Anthropic error event and is never presented as complete.
func TestClaudeStreamResponsesFailedEmitsError(t *testing.T) {
	t.Parallel()
	out := convertClaudeStreamForTest(t,
		`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"partial"}`,
		`data: {"type":"response.failed","response":{"id":"resp_1","object":"response","status":"failed","error":{"code":"server_error","message":"boom"},"output":[]}}`,
	)
	require.Contains(t, out, "event: error\n")
	require.Contains(t, out, `"message":"server_error: boom"`)
	report := claudeconformance.ValidateStream([]byte(out))
	require.NotContains(t, report.Events, "message_delta")
	require.NotContains(t, report.Events, "message_stop")
}

// TestClaudeStreamChatDropWithoutCompletion keeps the honest-proxy contract: a
// stream that ends with no finish_reason, terminal event, or [DONE] does not
// receive a fabricated stop_reason, content_block_stop, or message_stop.
func TestClaudeStreamChatDropWithoutCompletion(t *testing.T) {
	t.Parallel()
	out := convertClaudeStreamForTest(t,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"f","arguments":"{\"x\":"}}]}}]}`,
	)
	report := claudeconformance.ValidateStream([]byte(out))
	require.Equal(t, []string{"message_start", "content_block_start", "content_block_delta"}, report.Events)
}

// TestClaudeNonStreamChatNormalizesBlocks verifies non-streaming Chat
// conversion yields a conformant Message: explicit null stop_sequence, tool
// inputs that are always objects, valid unique tool ids, and tool_use when a
// provider reports "stop" for a tool turn.
func TestClaudeNonStreamChatNormalizesBlocks(t *testing.T) {
	t.Parallel()
	c, _ := newGinTestContext()
	upstream := `{"id":"","object":"chat.completion","model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"",` +
		`"reasoning_content":"why","tool_calls":[{"id":"call 1","type":"function","function":{"name":"noargs","arguments":""}},` +
		`{"id":"call 1","type":"function","function":{"name":"truncated","arguments":"{\"a\":"}}]}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(upstream))}
	converted, errResp := ConvertOpenAIResponseToClaudeResponse(c, resp)
	require.Nil(t, errResp)
	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"stop_sequence":null`)
	require.Contains(t, string(body), `"signature":""`)
	msg := requireConformantClaudeMessage(t, body)
	require.Equal(t, "tool_use", msg.StopReason)
	require.NotEmpty(t, msg.ID)
	require.Empty(t, msg.Text(), "empty text must not become an empty text block")
	tools := msg.ToolUses()
	require.Len(t, tools, 2)
	require.Equal(t, "call_1", tools[0].ID)
	require.Equal(t, "call_1_2", tools[1].ID)
	require.JSONEq(t, `{}`, tools[0].Input)
	require.JSONEq(t, `{}`, tools[1].Input)
}

// TestClaudeNonStreamResponsesIncomplete maps an incomplete Responses object to
// max_tokens and keeps the conformant envelope.
func TestClaudeNonStreamResponsesIncomplete(t *testing.T) {
	t.Parallel()
	c, _ := newGinTestContext()
	upstream := `{"id":"resp_1","object":"response","model":"m","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},` +
		`"output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"cut"}]}],"usage":{"input_tokens":3,"output_tokens":4}}`
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(upstream))}
	converted, errResp := ConvertOpenAIResponseToClaudeResponse(c, resp)
	require.Nil(t, errResp)
	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	msg := requireConformantClaudeMessage(t, body)
	require.Equal(t, "max_tokens", msg.StopReason)
	require.Equal(t, "cut", msg.Text())
}

// TestClaudeToolInputFromArguments pins the tool input normalization rules.
func TestClaudeToolInputFromArguments(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		args any
		want string
	}{
		"object_string":  {`{"a":1}`, `{"a":1}`},
		"empty_string":   {"", `{}`},
		"null":           {"null", `{}`},
		"invalid":        {`{"a":`, `{}`},
		"array":          {`[1,2]`, `{}`},
		"double_encoded": {`"{\"a\":1}"`, `{"a":1}`},
		"decoded_map":    {map[string]any{"a": 1}, `{"a":1}`},
		"nil":            {nil, `{}`},
	} {
		require.JSONEq(t, tc.want, string(ClaudeToolInputFromArguments(tc.args)), name)
	}
}

// TestClaudeStreamEmitterInvariants verifies the emitter itself enforces the
// stream contract: events for stopped or mistyped blocks are dropped, Finish
// closes open blocks in ascending index order, and Fail suppresses Finish.
func TestClaudeStreamEmitterInvariants(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	em := NewClaudeStreamEmitter(&sb, nil)
	em.Start("", "m", relaymodel.ClaudeUsage{InputTokens: 1})
	text := em.StartBlock(relaymodel.ClaudeContent{Type: "text"})
	tool := em.StartBlock(relaymodel.ClaudeContent{Type: "tool_use", ID: "toolu_1", Name: "f"})
	thinking := em.StartBlock(relaymodel.ClaudeContent{Type: "thinking"})
	em.TextDelta(text, "hi")
	em.StopBlock(text)
	em.TextDelta(text, "late")         // stopped block
	em.InputJSONDelta(thinking, `{}`)  // wrong block type
	em.StopBlock(text)                 // double stop
	em.InputJSONDelta(tool, `{"a":1}`) // open tool block
	em.Finish("tool_use", relaymodel.ClaudeUsage{InputTokens: 1, OutputTokens: 2})
	require.Equal(t, 3, em.Dropped())
	require.True(t, em.Finished())
	require.NoError(t, em.Err())

	msg := requireConformantClaudeStream(t, sb.String())
	require.Equal(t, "hi", msg.Text())
	require.Equal(t, "tool_use", msg.StopReason)
	require.JSONEq(t, `{"a":1}`, msg.Content[1].Input)
	report := claudeconformance.ValidateStream([]byte(sb.String()))
	require.Equal(t, []string{"message_start", "content_block_start", "content_block_start", "content_block_start", "content_block_delta", "content_block_stop", "content_block_delta", "content_block_stop", "content_block_stop", "message_delta", "message_stop"}, report.Events)

	var failed strings.Builder
	em = NewClaudeStreamEmitter(&failed, nil)
	em.Start("msg_x", "m", relaymodel.ClaudeUsage{})
	em.Fail("", "boom")
	em.Finish("end_turn", relaymodel.ClaudeUsage{})
	require.True(t, em.Failed())
	require.False(t, em.Finished())
	require.Contains(t, failed.String(), `"type":"api_error"`)
	require.NotContains(t, failed.String(), "message_stop")
}
