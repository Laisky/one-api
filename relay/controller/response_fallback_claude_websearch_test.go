package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// responsesWebSearchBalance is the finite fixture balance for ledger checks.
const responsesWebSearchBalance = int64(100_000)

// responsesWebSearchCharge is 11 input + 7 output tokens at ratio 1 plus two
// receipted searches at the configured claudeToolPolicyPerSearch tariff.
const responsesWebSearchCharge = int64(18) + 2*claudeToolPolicyPerSearch

// claudeWebSearchFallbackSSE streams two searches with results, a cumulative
// intermediate receipt, the final receipt and a duplicate final receipt.
const claudeWebSearchFallbackSSE = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_ws","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"stop_reason":null,"usage":{"input_tokens":11,"output_tokens":1,"server_tool_use":{"web_search_requests":0}}}}` + "\n\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{}}}` + "\n\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"a\"}"}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
	`data: {"type":"content_block_start","index":1,"content_block":{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","url":"https://example.com/a","title":"A","encrypted_content":"opaque"}]}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":1}` + "\n\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":4,"server_tool_use":{"web_search_requests":1}}}` + "\n\n" +
	`data: {"type":"content_block_start","index":2,"content_block":{"type":"server_tool_use","id":"srvtoolu_2","name":"web_search","input":{"query":"b"}}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":2}` + "\n\n" +
	`data: {"type":"content_block_start","index":3,"content_block":{"type":"web_search_tool_result","tool_use_id":"srvtoolu_2","content":[]}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":3}` + "\n\n" +
	`data: {"type":"content_block_start","index":4,"content_block":{"type":"text","text":""}}` + "\n\n" +
	`data: {"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"ok"}}` + "\n\n" +
	`data: {"type":"content_block_stop","index":4}` + "\n\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}` + "\n\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}` + "\n\n" +
	`data: {"type":"message_stop"}` + "\n\n"

// responsesWebSearchCase drives one Responses request through the Claude fallback.
type responsesWebSearchCase struct {
	channel     int
	actual      string
	payload     string
	contentType string
	response    string
	tooling     *model.ChannelToolingConfig
}

// responsesWebSearchResult carries the observable effects of one run.
type responsesWebSearchResult struct {
	apiErr       *relaymodel.ErrorWithStatusCode
	calls        int32
	body         map[string]any
	heldDuringIO int64
	id           string
	c            *gin.Context
	w            *httptest.ResponseRecorder
}

// runResponsesWebSearchCase serves tc.response from a synthetic TLS Claude
// upstream, records the provider request body and the balance held while the
// provider runs, and drains post-billing work.
func runResponsesWebSearchCase(t *testing.T, tc responsesWebSearchCase) responsesWebSearchResult {
	t.Helper()
	xaiVideoSetup(t, responsesWebSearchBalance, false)
	var calls atomic.Int32
	var held atomic.Int64
	observed := make(chan map[string]any, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var user model.User
		if err := model.DB.Where("id = ?", fallbackUserID).First(&user).Error; err == nil {
			held.Store(responsesWebSearchBalance - user.Quota)
		}
		var request map[string]any
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		select {
		case observed <- request:
		default:
		}
		w.Header().Set("Content-Type", tc.contentType)
		_, _ = io.WriteString(w, tc.response)
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	c, w, id := protocolContext(t, tc.channel, tc.actual, "/v1/responses", tc.payload, upstream.URL,
		responsesWebSearchBalance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	// Request IDs derive from the test name; clear rows a repeated run (-count)
	// left behind so the ledger assertions see only this run.
	require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Delete(&model.Log{}).Error)
	require.NoError(t, model.DB.Where("request_id = ?", id).Delete(&model.UserRequestCost{}).Error)
	if tc.tooling != nil {
		value, _ := c.Get(ctxkey.ChannelModel)
		require.NoError(t, value.(*model.Channel).SetToolingConfig(tc.tooling))
	}
	apiErr := RelayResponseAPIHelper(c)
	drainCriticalTasks(t)
	result := responsesWebSearchResult{apiErr: apiErr, calls: calls.Load(), heldDuringIO: held.Load(), id: id, c: c, w: w}
	if len(observed) > 0 {
		result.body = <-observed
	}
	return result
}

// codexWebSearchPayload is a Codex-shaped Responses request in live mode.
func codexWebSearchPayload(stream bool) string {
	return fmt.Sprintf(`{"model":"alias","stream":%t,"max_output_tokens":64,"input":[{"type":"message","role":"user",`+
		`"content":[{"type":"input_text","text":"news?"}]}],"tools":[{"type":"web_search","external_web_access":true,`+
		`"filters":{"allowed_domains":["example.com"]},"user_location":{"type":"approximate","country":"US"}}]}`, stream)
}

// requireWebSearchToolSummary asserts the shared server-tool billing summary.
func requireWebSearchToolSummary(t *testing.T, c *gin.Context) {
	t.Helper()
	raw, ok := c.Get(ctxkey.ToolInvocationSummary)
	require.True(t, ok, "server-tool billing must record a summary")
	summary, ok := raw.(*model.ToolUsageSummary)
	require.True(t, ok)
	require.Equal(t, 2, summary.Counts["web_search"])
	require.Equal(t, 2*claudeToolPolicyPerSearch, summary.CostByTool["web_search"])
}

// TestResponseFallbackClaudeWebSearchJSON proves a non-streaming fallback maps
// the Codex web_search declaration to the dynamic Claude server tool, renders
// web_search_call items, and charges tokens plus exactly two receipted searches.
func TestResponseFallbackClaudeWebSearchJSON(t *testing.T) {
	result := runResponsesWebSearchCase(t, responsesWebSearchCase{
		channel: channeltype.Anthropic, actual: "claude-opus-4-6", payload: codexWebSearchPayload(false),
		contentType: "application/json", response: fmt.Sprintf(claudeToolPolicyNativeJSON, `,"server_tool_use":{"web_search_requests":2}`),
		tooling: claudeToolPolicyAllowSearch(),
	})
	require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
	require.EqualValues(t, 1, result.calls)
	require.Equal(t, []any{map[string]any{
		"type": "web_search_20260209", "name": "web_search", "allowed_domains": []any{"example.com"},
		"user_location": map[string]any{"type": "approximate", "country": "US"},
	}}, result.body["tools"])
	require.GreaterOrEqual(t, result.heldDuringIO, claudeServerToolDefaultReservedUses*claudeToolPolicyPerSearch,
		"the search allowance is reserved before dispatch")

	var response openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(result.w.Body.Bytes(), &response))
	require.Len(t, response.Output, 3)
	require.Equal(t, "message", response.Output[0].Type)
	for i, query := range []string{"a", "b"} {
		item := response.Output[i+1]
		require.Equal(t, "web_search_call", item.Type)
		require.Equal(t, fmt.Sprintf("ws_%d", i+1), item.Id)
		require.Equal(t, "completed", item.Status)
		require.Equal(t, &openai.WebSearchCallAction{Type: "search", Query: query}, item.Action)
	}
	requireClaudeToolPolicyLedger(t, claudeToolPolicyResult{id: result.id}, responsesWebSearchBalance, responsesWebSearchCharge)
	requireWebSearchToolSummary(t, result.c)
}

// TestResponseFallbackClaudeWebSearchSSE proves streaming renders each search
// as output_item.added/done with one stable output_index, keeps the final
// output ordered, and charges the cumulative (not summed) receipt exactly once.
func TestResponseFallbackClaudeWebSearchSSE(t *testing.T) {
	result := runResponsesWebSearchCase(t, responsesWebSearchCase{
		channel: channeltype.Anthropic, actual: "claude-sonnet-4-5-20250929", payload: codexWebSearchPayload(true),
		contentType: "text/event-stream", response: claudeWebSearchFallbackSSE, tooling: claudeToolPolicyAllowSearch(),
	})
	require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
	require.EqualValues(t, 1, result.calls)
	tools, ok := result.body["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	require.Equal(t, "web_search_20250305", tools[0].(map[string]any)["type"], "pre-4.6 models use the basic version")

	added := map[string]int{}
	done := map[string]int{}
	order := []string{}
	var completed *openai.ResponseAPIResponse
	for _, event := range parseSSEEvents(result.w.Body.String()) {
		if event.data == "[DONE]" || event.data == "" {
			continue
		}
		var decoded openai.ResponseAPIStreamEvent
		require.NoError(t, json.Unmarshal([]byte(event.data), &decoded), event.data)
		switch event.event {
		case "response.output_item.added", "response.output_item.done":
			require.NotNil(t, decoded.Item)
			require.NotEqual(t, "function_call", decoded.Item.Type, "server tools must not become caller calls")
			if decoded.Item.Type != "web_search_call" {
				continue
			}
			if event.event == "response.output_item.added" {
				require.NotContains(t, added, decoded.Item.Id)
				added[decoded.Item.Id] = decoded.OutputIndex
				order = append(order, decoded.Item.Id)
				continue
			}
			require.Equal(t, added[decoded.Item.Id], decoded.OutputIndex, "added and done share one output_index")
			require.Equal(t, "completed", decoded.Item.Status)
			done[decoded.Item.Id] = decoded.OutputIndex
		case "response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed":
			require.Contains(t, added, decoded.ItemId)
			require.Equal(t, added[decoded.ItemId], decoded.OutputIndex)
		case "response.completed":
			completed = decoded.Response
		}
	}
	require.Equal(t, []string{"ws_1", "ws_2"}, order)
	require.Equal(t, added, done)
	require.Equal(t, map[string]int{"ws_1": 1, "ws_2": 2}, added, "the message keeps index 0")
	require.NotNil(t, completed)
	require.Len(t, completed.Output, 3)
	require.Equal(t, "message", completed.Output[0].Type)
	require.Equal(t, "ok", completed.Output[0].Content[0].Text)
	require.Equal(t, "ws_1", completed.Output[1].Id)
	require.Equal(t, &openai.WebSearchCallAction{Type: "search", Query: "a"}, completed.Output[1].Action)
	require.Equal(t, "ws_2", completed.Output[2].Id)

	requireClaudeToolPolicyLedger(t, claudeToolPolicyResult{id: result.id}, responsesWebSearchBalance, responsesWebSearchCharge)
	requireWebSearchToolSummary(t, result.c)
}

// TestResponseFallbackClaudeWebSearchHistoryReplay proves Codex's verbatim
// replay of earlier web_search_call items is accepted and dropped upstream.
func TestResponseFallbackClaudeWebSearchHistoryReplay(t *testing.T) {
	payload := `{"model":"alias","max_output_tokens":64,"tools":[{"type":"web_search","external_web_access":false}],"input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"news?"}]},` +
		`{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"a"}},` +
		`{"type":"web_search_call","id":"ws_2","status":"completed","action":{"type":"open_page","url":"https://example.com"}},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"earlier answer"}]},` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"more?"}]}]}`
	result := runResponsesWebSearchCase(t, responsesWebSearchCase{
		channel: channeltype.Anthropic, actual: "claude-opus-4-6", payload: payload,
		contentType: "application/json", response: fmt.Sprintf(claudeToolPolicyNativeJSON, `,"server_tool_use":{"web_search_requests":2}`),
		tooling: claudeToolPolicyAllowSearch(),
	})
	require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
	messages, ok := result.body["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 3, "replayed searches are dropped, the turns stay intact")
	encoded, err := json.Marshal(messages)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "web_search_call")
	require.NotContains(t, string(encoded), "server_tool_use")
	// Cached mode (external_web_access:false) still searches: Claude has no
	// cached index, and the client still expects search results.
	require.Len(t, result.body["tools"], 1)
	requireClaudeToolPolicyLedger(t, claudeToolPolicyResult{id: result.id}, responsesWebSearchBalance, responsesWebSearchCharge)
}

// TestResponseFallbackClaudeWebSearchUnsupportedPlatform proves platforms
// without a metered Claude web search keep the historical drop and invent no
// tool charge or allowance.
func TestResponseFallbackClaudeWebSearchUnsupportedPlatform(t *testing.T) {
	var droppedHold, forwardedHold int64
	t.Run("claude_compatible_drops", func(t *testing.T) {
		result := runResponsesWebSearchCase(t, responsesWebSearchCase{
			channel: channeltype.ClaudeCompatible, actual: "claude-opus-4-6", payload: codexWebSearchPayload(false),
			contentType: "application/json", response: fmt.Sprintf(claudeToolPolicyNativeJSON, ""),
			tooling: claudeToolPolicyAllowSearch(),
		})
		require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
		require.EqualValues(t, 1, result.calls)
		require.NotContains(t, result.body, "tools", "the hosted declaration is dropped")
		require.Zero(t, result.c.GetInt64(ctxkey.ResponseFallbackToolAllowanceQuota), "no search allowance is reserved")
		droppedHold = result.heldDuringIO
		// The synthetic upstream still shows two unsolicited search blocks; with no
		// receipt counter the shared tally bills observed blocks, toward charging.
		requireClaudeToolPolicyLedger(t, claudeToolPolicyResult{id: result.id}, responsesWebSearchBalance, responsesWebSearchCharge)
	})
	t.Run("anthropic_forwards", func(t *testing.T) {
		result := runResponsesWebSearchCase(t, responsesWebSearchCase{
			channel: channeltype.Anthropic, actual: "claude-opus-4-6", payload: codexWebSearchPayload(false),
			contentType: "application/json", response: fmt.Sprintf(claudeToolPolicyNativeJSON, `,"server_tool_use":{"web_search_requests":2}`),
			tooling: claudeToolPolicyAllowSearch(),
		})
		require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
		forwardedHold = result.heldDuringIO
		requireClaudeToolPolicyLedger(t, claudeToolPolicyResult{id: result.id}, responsesWebSearchBalance, responsesWebSearchCharge)
	})
	// The same request holds exactly ten searches more while a first-party
	// provider runs; the complete receipt then refunds the unused allowance.
	require.Equal(t, claudeServerToolDefaultReservedUses*claudeToolPolicyPerSearch, forwardedHold-droppedHold)
}

// TestResponseFallbackClaudeWebSearchRejectsContradictoryFilters proves options
// Claude cannot represent are rejected before reservation or dispatch.
func TestResponseFallbackClaudeWebSearchRejectsContradictoryFilters(t *testing.T) {
	payload := `{"model":"alias","input":"news?","tools":[{"type":"web_search","filters":{"allowed_domains":["a.com"],"disallowed_domains":["b.com"]}}]}`
	result := runResponsesWebSearchCase(t, responsesWebSearchCase{
		channel: channeltype.Anthropic, actual: "claude-opus-4-6", payload: payload,
		contentType: "application/json", response: fmt.Sprintf(claudeToolPolicyNativeJSON, ""),
		tooling: claudeToolPolicyAllowSearch(),
	})
	requireClaudeToolPolicyUntouched(t, claudeToolPolicyResult{apiErr: result.apiErr, calls: result.calls}, responsesWebSearchBalance)
	require.Equal(t, "response_fallback_not_supported", result.apiErr.Code)
}
