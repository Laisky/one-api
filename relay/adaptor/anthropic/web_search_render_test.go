package anthropic

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// claudeWebSearchMessageJSON is a complete Claude message with a completed
// search, a failed search, a search that never returned, and cited text.
const claudeWebSearchMessageJSON = `{"id":"msg_ws","type":"message","role":"assistant","model":"claude-opus-4-6","content":[
	{"type":"text","text":"I'll search for that."},
	{"type":"server_tool_use","id":"srvtoolu_a","name":"web_search","input":{"query":"go release"}},
	{"type":"web_search_tool_result","tool_use_id":"srvtoolu_a","content":[{"type":"web_search_result","url":"https://go.dev","title":"Go","encrypted_content":"opaque"}]},
	{"type":"server_tool_use","id":"srvtoolu_b","name":"web_search","input":{"query":"go 1.27 notes"}},
	{"type":"web_search_tool_result","tool_use_id":"srvtoolu_b","content":{"type":"web_search_tool_result_error","error_code":"max_uses_exceeded"}},
	{"type":"text","text":"Based on the results, "},
	{"type":"text","text":"Go 1.27 is out.","citations":[{"type":"web_search_result_location","url":"https://go.dev","title":"Go","encrypted_index":"idx","cited_text":"Go 1.27"}]},
	{"type":"server_tool_use","id":"srvtoolu_c","name":"web_search","input":{"query":"pending"}}
],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}`

// newWebSearchRenderContext builds a request context for converted rendering.
func newWebSearchRenderContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-web-search-render-test"))
	return c, recorder
}

// TestResponseClaude2OpenAIWebSearchCalls proves non-streaming conversion
// carries every web search with its status and never exposes server tools as
// caller tool calls, while the receipt still drives billing counts.
func TestResponseClaude2OpenAIWebSearchCalls(t *testing.T) {
	c, recorder := newWebSearchRenderContext(t)
	errResp, usage := Handler(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(claudeWebSearchMessageJSON))}, 0, "claude-opus-4-6")
	require.Nil(t, errResp)
	require.Equal(t, 11, usage.PromptTokens)

	var converted openai.TextResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &converted))
	require.Len(t, converted.Choices, 1)
	message := converted.Choices[0].Message
	require.Empty(t, message.ToolCalls)
	require.Equal(t, "I'll search for that.\n\nBased on the results, Go 1.27 is out.", message.StringContent())
	require.Equal(t, []model.WebSearchCall{
		{ID: "srvtoolu_a", Query: "go release", Status: model.WebSearchCallCompleted},
		{ID: "srvtoolu_b", Query: "go 1.27 notes", Status: model.WebSearchCallFailed},
		{ID: "srvtoolu_c", Query: "pending", Status: model.WebSearchCallFailed},
	}, message.WebSearchCalls)

	counts, ok := c.Get(ctxkey.ToolInvocationCounts)
	require.True(t, ok)
	require.Equal(t, map[string]int{"web_search": 2}, counts, "the receipt, not the block count, is billed")
}

// claudeWebSearchStream streams one completed and one failed search, dynamic
// filtering code execution blocks, text on both sides, and cumulative receipts.
const claudeWebSearchStream = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_ws","type":"message","role":"assistant","model":"claude-opus-4-6","content":[],"stop_reason":null,"usage":{"input_tokens":11,"output_tokens":1}}}` + "\n\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Searching."}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
	`data: {"type":"content_block_start","index":1,"content_block":{"type":"server_tool_use","id":"srvtoolu_code","name":"code_execution","input":{}}}` + "\n\n" +
	`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"code\":\"print(1)\"}"}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":1}` + "\n\n" +
	`data: {"type":"content_block_start","index":2,"content_block":{"type":"server_tool_use","id":"srvtoolu_a","name":"web_search","input":{}}}` + "\n\n" +
	`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}` + "\n\n" +
	`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"go release\"}"}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":2}` + "\n\n" +
	`data: {"type":"content_block_start","index":3,"content_block":{"type":"web_search_tool_result","tool_use_id":"srvtoolu_a","content":[{"type":"web_search_result","url":"https://go.dev","title":"Go"}]}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":3}` + "\n\n" +
	`data: {"type":"content_block_start","index":4,"content_block":{"type":"server_tool_use","id":"srvtoolu_b","name":"web_search","input":{"query":"notes"}}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":4}` + "\n\n" +
	`data: {"type":"content_block_start","index":5,"content_block":{"type":"web_search_tool_result","tool_use_id":"srvtoolu_b","content":{"type":"web_search_tool_result_error","error_code":"unavailable"}}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":5}` + "\n\n" +
	`data: {"type":"content_block_start","index":6,"content_block":{"type":"code_execution_tool_result","tool_use_id":"srvtoolu_code","content":{"type":"code_execution_result","stdout":"1"}}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":6}` + "\n\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":3,"server_tool_use":{"web_search_requests":1}}}` + "\n\n" +
	`data: {"type":"content_block_start","index":7,"content_block":{"type":"text","text":""}}` + "\n\n" +
	`data: {"type":"content_block_delta","index":7,"delta":{"type":"text_delta","text":"Go 1.27 is out."}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":7}` + "\n\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}` + "\n\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}` + "\n\n" +
	`data: {"type":"message_stop"}` + "\n\n"

// TestStreamHandlerWebSearchCalls proves converted streams surface searches
// as WebSearchCalls chunks, hide every server block from caller tool calls,
// separate text across the search round, and bill the maximum receipt once.
func TestStreamHandlerWebSearchCalls(t *testing.T) {
	c, recorder := newWebSearchRenderContext(t)
	errResp, usage := StreamHandler(c, &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body:   io.NopCloser(strings.NewReader(claudeWebSearchStream))})
	require.Nil(t, errResp)
	require.Equal(t, 11, usage.PromptTokens)
	require.Equal(t, 7, usage.CompletionTokens)

	var text strings.Builder
	var calls []model.WebSearchCall
	scanner := bufio.NewScanner(strings.NewReader(recorder.Body.String()))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok || line == "[DONE]" {
			continue
		}
		var chunk openai.ChatCompletionsStreamResponse
		require.NoError(t, json.Unmarshal([]byte(line), &chunk))
		for _, choice := range chunk.Choices {
			require.Empty(t, choice.Delta.ToolCalls, "server tools must not leak as caller tool calls")
			if content, ok := choice.Delta.Content.(string); ok {
				text.WriteString(content)
			}
			calls = append(calls, choice.Delta.WebSearchCalls...)
		}
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, "Searching.\n\nGo 1.27 is out.", text.String())
	require.Equal(t, []model.WebSearchCall{
		{ID: "srvtoolu_a", Query: "go release", Status: model.WebSearchCallInProgress},
		{ID: "srvtoolu_a", Query: "go release", Status: model.WebSearchCallCompleted},
		{ID: "srvtoolu_b", Query: "notes", Status: model.WebSearchCallInProgress},
		{ID: "srvtoolu_b", Query: "notes", Status: model.WebSearchCallFailed},
	}, calls)

	counts, ok := c.Get(ctxkey.ToolInvocationCounts)
	require.True(t, ok)
	// Searches bill from the cumulative receipt (maximum, not sum). The dynamic
	// filtering code block has no receipt counter, so the existing tally keeps
	// its observed block; tool policy prices it (zero by default) separately.
	require.Equal(t, map[string]int{"web_search": 2, "code_execution": 1}, counts)
}

// TestStreamServerBlockInputBound proves an oversized streamed tool input is
// discarded instead of buffered without limit.
func TestStreamServerBlockInputBound(t *testing.T) {
	block := &streamServerBlock{}
	block.appendInput([]byte(`{"query":"`))
	block.appendInput(make([]byte, maxWebSearchQueryInputBytes))
	require.True(t, block.overflow)
	require.Nil(t, block.searchInput)
	block.appendInput([]byte(`more`))
	require.Nil(t, block.searchInput)
}
